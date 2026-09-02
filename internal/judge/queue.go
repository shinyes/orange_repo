// QueueService 判题队列（迁移自上游 OrangeOJ backend/internal/judge/queue.go）。
//
// 状态机（表结构见 internal/quizstore 迁移，SQL 与上游一致）：
//
//	judge_jobs: queued → running(worker_token) → done | failed
//	submissions: queued → running → done(verdict/score/耗时/用例明细) | failed(RE)
//	user_problem_progress: submit/test 完成后 upsert（best_score 优先）
package judge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// jobItem 认领到的队列任务。
type jobItem struct {
	ID           int64
	SubmissionID int64
}

// QueueService 判题队列：worker 轮询认领并驱动评测。
type QueueService struct {
	db      *sql.DB
	runner  Runner
	loader  SubmissionLoader
	workers int
}

// NewQueueService 构造队列服务（workers < 1 视为 1）。
func NewQueueService(db *sql.DB, runner Runner, loader SubmissionLoader, workers int) *QueueService {
	if workers < 1 {
		workers = 1
	}
	return &QueueService{db: db, runner: runner, loader: loader, workers: workers}
}

// Start 启动全部 worker goroutine（随 ctx 取消退出）。
func (q *QueueService) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		go q.workerLoop(ctx, i+1)
	}
}

func (q *QueueService) workerLoop(ctx context.Context, idx int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := q.claimJob(ctx)
		if err != nil {
			log.Printf("[judge-worker-%d] claim error: %v", idx, err)
			time.Sleep(800 * time.Millisecond)
			continue
		}
		if job == nil {
			time.Sleep(400 * time.Millisecond)
			continue
		}

		if err := q.processJob(ctx, *job); err != nil {
			log.Printf("[judge-worker-%d] process job %d failed: %v", idx, job.ID, err)
			_ = q.failJob(context.Background(), job.ID, err)
		}
	}
}

// claimJob 原子认领一个 queued 任务（RETURNING，事务内完成）。
func (q *QueueService) claimJob(ctx context.Context) (*jobItem, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
WITH cte AS (
	SELECT id
	FROM judge_jobs
	WHERE status='queued' AND datetime(available_at) <= datetime('now')
	ORDER BY priority DESC, id ASC
	LIMIT 1
)
UPDATE judge_jobs
SET status='running', started_at=CURRENT_TIMESTAMP, worker_token=?
WHERE id=(SELECT id FROM cte)
RETURNING id, submission_id`, uuid.NewString())

	item := &jobItem{}
	if err := row.Scan(&item.ID, &item.SubmissionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tx.Commit()
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

// processJob 完整处理一次评测：置 running → 装载 → 选用例 → runner → 写回 → 完成。
func (q *QueueService) processJob(ctx context.Context, job jobItem) error {
	if q.runner == nil || q.loader == nil {
		return fmt.Errorf("judge runner/loader is nil")
	}

	if _, err := q.db.ExecContext(ctx, `UPDATE submissions SET status='running' WHERE id=?`, job.SubmissionID); err != nil {
		return err
	}

	sub, err := q.loader.LoadSubmission(ctx, job.SubmissionID)
	if err != nil {
		return err
	}

	body := ProgrammingBody{}
	if err := json.Unmarshal([]byte(sub.BodyJSON), &body); err != nil {
		body = ProgrammingBody{}
	}

	checkAnswer := sub.SubmitType == SubmitTypeTest || sub.SubmitType == SubmitTypeSubmit
	cases := SelectCases(sub.SubmitType, body, sub.InputData)

	result, err := q.runner.Judge(ctx, JudgeTask{
		SubmissionID:    sub.ID,
		Language:        sub.Language,
		SourceCode:      sub.SourceCode,
		TimeLimitMS:     sub.TimeLimitMS,
		MemoryLimitMiB:  sub.MemoryLimitMiB,
		CheckAnswer:     checkAnswer,
		CompileTimeoutS: 10,
		Cases:           cases,
	})
	if err != nil {
		return err
	}

	score := 0
	if sub.SubmitType == SubmitTypeSubmit || sub.SubmitType == SubmitTypeTest {
		if result.Verdict == VerdictAC {
			score = 100
		}
	}

	if err := q.finishSubmission(ctx, sub, result.Verdict, result.TimeMS, result.MemoryKiB, score, result.Stdout, result.Stderr, result.CaseResults); err != nil {
		return err
	}
	if err := q.completeJob(ctx, job.ID); err != nil {
		return err
	}
	return nil
}

// finishSubmission 写回 submissions 结果；submit/test 同步 upsert 学生进度。
func (q *QueueService) finishSubmission(ctx context.Context, sub *RuntimeSubmission, verdict Verdict, timeMS, memoryKiB, score int, stdout, stderr string, caseDetails []CaseResult) error {
	caseDetailsJSON := ""
	if len(caseDetails) > 0 {
		raw, err := json.Marshal(caseDetails)
		if err != nil {
			return err
		}
		caseDetailsJSON = string(raw)
	}
	_, err := q.db.ExecContext(ctx, `
UPDATE submissions
SET status='done', verdict=?, time_ms=?, memory_kib=?, score=?, stdout=?, stderr=?, case_details_json=?, finished_at=CURRENT_TIMESTAMP
WHERE id=?`, string(verdict), timeMS, memoryKiB, score, stdout, stderr, caseDetailsJSON, sub.ID)
	if err != nil {
		return err
	}

	if sub.SubmitType == SubmitTypeSubmit || sub.SubmitType == SubmitTypeTest {
		if _, err := q.db.ExecContext(ctx, `
INSERT INTO user_problem_progress(user_id, problem_id, best_verdict, best_score, last_submission_id, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id, problem_id)
DO UPDATE SET
	best_verdict=CASE WHEN excluded.best_score >= user_problem_progress.best_score THEN excluded.best_verdict ELSE user_problem_progress.best_verdict END,
	best_score=CASE WHEN excluded.best_score >= user_problem_progress.best_score THEN excluded.best_score ELSE user_problem_progress.best_score END,
	last_submission_id=excluded.last_submission_id,
	updated_at=CURRENT_TIMESTAMP`, sub.UserID, sub.ProblemID, string(verdict), score, sub.ID); err != nil {
			return err
		}
	}
	return nil
}

// completeJob 标记队列任务完成。
func (q *QueueService) completeJob(ctx context.Context, jobID int64) error {
	_, err := q.db.ExecContext(ctx, `UPDATE judge_jobs SET status='done', finished_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	return err
}

// failJob 处理异常失败：judge_jobs failed + submissions failed(RE, stderr 截断)。
func (q *QueueService) failJob(ctx context.Context, jobID int64, jobErr error) error {
	_, err := q.db.ExecContext(ctx, `UPDATE judge_jobs SET status='failed', finished_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	if err != nil {
		return err
	}
	_, _ = q.db.ExecContext(ctx, `
UPDATE submissions
SET status='failed', verdict='RE', stderr=?, finished_at=CURRENT_TIMESTAMP
WHERE id=(SELECT submission_id FROM judge_jobs WHERE id=?)`, TrimTo(jobErr.Error(), MaxFailStderr), jobID)
	return nil
}

// EnqueueSubmission 入队（调用方须在事务内连同 submission 一起写）。
func EnqueueSubmission(ctx context.Context, db *sql.DB, submissionID int64, priority int) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO judge_jobs(submission_id, status, priority, available_at)
VALUES(?, 'queued', ?, CURRENT_TIMESTAMP)`, submissionID, priority)
	return err
}
