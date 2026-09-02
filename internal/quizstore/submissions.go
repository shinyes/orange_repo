// submissions/judge_jobs/progress 数据层（quiz.db）。
// 表结构与上游 OrangeOJ backend/internal/db/db.go 一致（去 space_id）。
package quizstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"orangerepo/internal/judge"
)

// Submission 提交记录（JSON 视图，判题结果完成态）。
type Submission struct {
	ID           int64             `json:"id"`
	ProblemID    int64             `json:"problemId"`
	QuestionType string            `json:"questionType"`
	Language     string            `json:"language"`
	SourceCode   string            `json:"sourceCode,omitempty"`
	InputData    string            `json:"inputData,omitempty"`
	SubmitType   judge.SubmitType  `json:"submitType"`
	Status       string            `json:"status"`
	Verdict      judge.Verdict     `json:"verdict"`
	TimeMS       int               `json:"timeMs"`
	MemoryKiB    int               `json:"memoryKiB"`
	Score        int               `json:"score"`
	Stdout       string            `json:"stdout,omitempty"`
	Stderr       string            `json:"stderr,omitempty"`
	CaseDetails  []judge.CaseResult `json:"caseDetails,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	FinishedAt   *time.Time        `json:"finishedAt,omitempty"`
}

// CreateProgrammingSubmission 事务内写入 submissions(queued) + judge_jobs(queued)。
// 返回 submission id；runner 未配置（judge-token 空）时由调用方决定拒绝。
func (s *Store) CreateProgrammingSubmission(userID, problemID int64, qtype, language, sourceCode, inputData string, submitType judge.SubmitType) (int64, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO submissions
		(user_id, problem_id, question_type, language, source_code, input_data, submit_type, status, verdict)
		VALUES(?,?,?,?,?,?,?,'queued','PENDING')`,
		userID, problemID, qtype, language, sourceCode, inputData, string(submitType))
	if err != nil {
		return 0, err
	}
	submissionID, _ := res.LastInsertId()

	if _, err := tx.Exec(`INSERT INTO judge_jobs(submission_id, status, priority, available_at)
		VALUES(?,'queued',0,CURRENT_TIMESTAMP)`, submissionID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return submissionID, nil
}

// CreateObjectiveSubmission 客观题同步判题完成记录（status=done，score 100/0）。
func (s *Store) CreateObjectiveSubmission(userID, problemID int64, qtype, answerText string, verdict judge.Verdict, score int) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO submissions
		(user_id, problem_id, question_type, language, source_code, input_data, submit_type, status, verdict, score, finished_at)
		VALUES(?,?,?,'','',?,'objective','done',?,?,CURRENT_TIMESTAMP)`,
		userID, problemID, qtype, answerText, string(verdict), score)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpsertProgress 客观题/自测后同步学生进度（与队列 finishSubmission 同语义）。
func (s *Store) UpsertProgress(userID, problemID int64, verdict judge.Verdict, score int, submissionID int64) error {
	_, err := s.DB.Exec(`INSERT INTO user_problem_progress(user_id, problem_id, best_verdict, best_score, last_submission_id, updated_at)
		VALUES(?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, problem_id) DO UPDATE SET
			best_verdict=CASE WHEN excluded.best_score >= user_problem_progress.best_score THEN excluded.best_verdict ELSE user_problem_progress.best_verdict END,
			best_score=CASE WHEN excluded.best_score >= user_problem_progress.best_score THEN excluded.best_score ELSE user_problem_progress.best_score END,
			last_submission_id=excluded.last_submission_id,
			updated_at=CURRENT_TIMESTAMP`,
		userID, problemID, string(verdict), score, submissionID)
	return err
}

// LoadSubmission 实现 judge.SubmissionLoader：装载队列所需运行时数据
// （quiz.db submissions + 主库 problems 运行时字段）。
func (s *Store) LoadSubmission(ctx context.Context, submissionID int64) (*judge.RuntimeSubmission, error) {
	var (
		userID, problemID int64
		typ, language, source, input string
	)
	err := s.DB.QueryRowContext(ctx, `SELECT user_id,problem_id,submit_type,language,source_code,input_data
		FROM submissions WHERE id=?`, submissionID).
		Scan(&userID, &problemID, &typ, &language, &source, &input)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// 主库题目运行时数据（时限/内存/bodyJson）
	limit, mem, body, err := s.Repo.LoadJudgeProblemData(ctx, problemID)
	if err != nil {
		return nil, err
	}
	return &judge.RuntimeSubmission{
		ID:             submissionID,
		UserID:         userID,
		ProblemID:      problemID,
		SubmitType:     judge.SubmitType(typ),
		Language:       language,
		SourceCode:     source,
		InputData:      input,
		TimeLimitMS:    limit,
		MemoryLimitMiB: mem,
		BodyJSON:       body,
	}, nil
}

// ListSubmissions 某学生某题的提交历史（倒序，上限 50）。
func (s *Store) ListSubmissions(userID, problemID int64) ([]Submission, error) {
	rows, err := s.DB.Query(`SELECT id,problem_id,question_type,language,input_data,submit_type,status,verdict,
		time_ms,memory_kib,score,stdout,stderr,case_details_json,created_at,finished_at
		FROM submissions WHERE user_id=? AND problem_id=? ORDER BY id DESC LIMIT 50`, userID, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Submission{}
	for rows.Next() {
		var sub Submission
		var stderr, stdout, details string
		var finished sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.ProblemID, &sub.QuestionType, &sub.Language, &sub.InputData,
			&sub.SubmitType, &sub.Status, &sub.Verdict, &sub.TimeMS, &sub.MemoryKiB, &sub.Score,
			&stdout, &stderr, &details, &sub.CreatedAt, &finished); err != nil {
			return nil, err
		}
		sub.Stdout = stdout
		sub.Stderr = stderr
		if finished.Valid {
			sub.FinishedAt = &finished.Time
		}
		_ = json.Unmarshal([]byte(details), &sub.CaseDetails)
		out = append(out, sub)
	}
	return out, rows.Err()
}

// GetSubmission 单条提交（供 poll；不存在返回 ErrNotFound）。
func (s *Store) GetSubmission(userID, id int64) (*Submission, error) {
	var sub Submission
	var stderr, stdout, details string
	var finished sql.NullTime
	err := s.DB.QueryRow(`SELECT id,problem_id,question_type,language,input_data,submit_type,status,verdict,
		time_ms,memory_kib,score,stdout,stderr,case_details_json,created_at,finished_at
		FROM submissions WHERE id=? AND user_id=?`, id, userID).
		Scan(&sub.ID, &sub.ProblemID, &sub.QuestionType, &sub.Language, &sub.InputData,
			&sub.SubmitType, &sub.Status, &sub.Verdict, &sub.TimeMS, &sub.MemoryKiB, &sub.Score,
			&stdout, &stderr, &details, &sub.CreatedAt, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sub.Stdout = stdout
	sub.Stderr = stderr
	if finished.Valid {
		sub.FinishedAt = &finished.Time
	}
	_ = json.Unmarshal([]byte(details), &sub.CaseDetails)
	return &sub, nil
}

// QueueHealth 返回当前排队/运行中的任务数（健康检查用）。
func (s *Store) QueueHealth() (queued, running int, err error) {
	err = s.DB.QueryRow(`SELECT
		(SELECT COUNT(*) FROM judge_jobs WHERE status='queued'),
		(SELECT COUNT(*) FROM judge_jobs WHERE status='running')`).Scan(&queued, &running)
	return queued, running, err
}

// bestVerdicts 批量取学生进度（problem ids → best verdict；供列表完成态）。
func (s *Store) bestVerdicts(userID int64, problemIDs []int64) (map[int64]judge.Verdict, error) {
	out := map[int64]judge.Verdict{}
	if len(problemIDs) == 0 {
		return out, nil
	}
	placeholders := ""
	args := []any{userID}
	for i, pid := range problemIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, pid)
	}
	rows, err := s.DB.Query(`SELECT problem_id,best_verdict FROM user_problem_progress WHERE user_id=? AND problem_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var v string
		if err := rows.Scan(&pid, &v); err != nil {
			return nil, err
		}
		out[pid] = judge.Verdict(v)
	}
	return out, rows.Err()
}

// Progress 学生某题进度（最佳判定/分数/最近提交）。
func (s *Store) Progress(userID, problemID int64) (verdict judge.Verdict, score int, ok bool, err error) {
	var v string
	err = s.DB.QueryRow(`SELECT best_verdict,best_score FROM user_problem_progress WHERE user_id=? AND problem_id=?`, userID, problemID).
		Scan(&v, &score)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return judge.Verdict(v), score, true, nil
}

// AcceptedProblemIDs 某学生所有 AC 过的题目集合。
func (s *Store) AcceptedProblemIDs(userID int64) (map[int64]bool, error) {
	rows, err := s.DB.Query(`SELECT problem_id FROM user_problem_progress WHERE user_id=? AND best_verdict IN ('AC','OK')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var pid int64
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out[pid] = true
	}
	return out, rows.Err()
}
