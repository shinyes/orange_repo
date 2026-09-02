// 判题队列单测：fake runner 驱动，验证用例选择/状态机/进度 upsert/失败兜底。
package judge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fakeDB 内存 sqlite（表结构最小集：submissions/judge_jobs/user_problem_progress）。
func fakeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL, problem_id INTEGER NOT NULL,
			question_type TEXT NOT NULL, language TEXT NOT NULL DEFAULT '',
			source_code TEXT NOT NULL DEFAULT '', input_data TEXT NOT NULL DEFAULT '',
			submit_type TEXT NOT NULL, status TEXT NOT NULL, verdict TEXT NOT NULL DEFAULT 'PENDING',
			time_ms INTEGER NOT NULL DEFAULT 0, memory_kib INTEGER NOT NULL DEFAULT 0,
			score INTEGER NOT NULL DEFAULT 0, stdout TEXT NOT NULL DEFAULT '', stderr TEXT NOT NULL DEFAULT '',
			case_details_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, finished_at DATETIME
		);`,
		`CREATE TABLE judge_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
			status TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 0,
			available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME, finished_at DATETIME, worker_token TEXT
		);`,
		`CREATE TABLE user_problem_progress (
			user_id INTEGER NOT NULL, problem_id INTEGER NOT NULL,
			best_verdict TEXT NOT NULL, best_score INTEGER NOT NULL DEFAULT 0,
			last_submission_id INTEGER NOT NULL, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(user_id, problem_id)
		);`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

type fakeLoader struct {
	mu   sync.Mutex
	subs map[int64]RuntimeSubmission
}

func (f *fakeLoader) LoadSubmission(_ context.Context, id int64) (*RuntimeSubmission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.subs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &s, nil
}

type fakeRunner struct {
	mu       sync.Mutex
	lastTask JudgeTask
	result   RunResult
	err      error
}

func (f *fakeRunner) Judge(_ context.Context, task JudgeTask) (RunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastTask = task
	if f.err != nil {
		return RunResult{}, f.err
	}
	return f.result, nil
}

func seedSubmission(t *testing.T, db *sql.DB, userID, problemID int64, typ SubmitType, bodyJSON string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO submissions(user_id,problem_id,question_type,language,source_code,input_data,submit_type,status,verdict)
		VALUES(?,?,'programming','python','code','in',?,'queued','PENDING')`, userID, problemID, string(typ))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO judge_jobs(submission_id,status,priority) VALUES(?,'queued',0)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSelectCases(t *testing.T) {
	body := ProgrammingBody{
		Samples:   []ProgrammingCase{{Input: "s1", Output: "o1"}},
		TestCases: []ProgrammingCase{{Input: "t1", Output: "o1"}, {Input: "t2", Output: "o2"}},
	}
	runCases := SelectCases(SubmitTypeRun, body, "custom")
	if len(runCases) != 1 || runCases[0].Input != "custom" || runCases[0].Expected != "" {
		t.Fatalf("run cases = %+v", runCases)
	}
	testCases := SelectCases(SubmitTypeTest, body, "")
	if len(testCases) != 2 || testCases[0].Input != "t1" {
		t.Fatalf("test cases = %+v", testCases)
	}
	noTC := ProgrammingBody{Samples: []ProgrammingCase{{Input: "s1", Output: "o1"}}}
	testFallback := SelectCases(SubmitTypeSubmit, noTC, "")
	if len(testFallback) != 1 || testFallback[0].Input != "s1" {
		t.Fatalf("submit fallback = %+v", testFallback)
	}
	empty := SelectCases(SubmitTypeSubmit, ProgrammingBody{}, "")
	if len(empty) != 1 {
		t.Fatalf("empty fallback = %+v", empty)
	}
}

// TestQueueSubmitWritesResult 完整 submit 流程：认领 → 写回 done + progress。
func TestQueueSubmitWritesResult(t *testing.T) {
	db := fakeDB(t)
	body, _ := json.Marshal(ProgrammingBody{TestCases: []ProgrammingCase{{Input: "1 2", Output: "3"}}})
	subID := seedSubmission(t, db, 7, 99, SubmitTypeSubmit, string(body))
	loader := &fakeLoader{subs: map[int64]RuntimeSubmission{
		subID: {ID: subID, UserID: 7, ProblemID: 99, SubmitType: SubmitTypeSubmit, Language: "python",
			SourceCode: "print(1)", InputData: "", TimeLimitMS: 1000, MemoryLimitMiB: 256, BodyJSON: string(body)},
	}}
	runner := &fakeRunner{result: RunResult{
		Verdict: VerdictAC, TimeMS: 5, MemoryKiB: 256 * 1024, Stdout: "out",
		CaseResults: []CaseResult{{CaseNo: 1, Verdict: VerdictAC, Output: "3", ExpectedOutput: "3"}},
	}}
	q := NewQueueService(db, runner, loader, 2)
	q.Start(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status, verdict string
		_ = db.QueryRow(`SELECT status,verdict FROM submissions WHERE id=?`, subID).Scan(&status, &verdict)
		if status == "done" {
			if verdict != string(VerdictAC) {
				t.Fatalf("verdict = %s", verdict)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("judge 未完成: status=%s", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// progress
	var best string
	var score int
	if err := db.QueryRow(`SELECT best_verdict,best_score FROM user_problem_progress WHERE user_id=7 AND problem_id=99`).Scan(&best, &score); err != nil {
		t.Fatal(err)
	}
	if best != string(VerdictAC) || score != 100 {
		t.Fatalf("progress = %s/%d", best, score)
	}
	// 队列任务 done
	var jobStatus string
	_ = db.QueryRow(`SELECT status FROM judge_jobs WHERE submission_id=?`, subID).Scan(&jobStatus)
	if jobStatus != "done" {
		t.Fatalf("job status = %s", jobStatus)
	}
	// 用例已由 processJob 通过 body.testCases 选择
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.lastTask.Cases) != 1 || runner.lastTask.Cases[0].Expected != "3" {
		t.Fatalf("runner task cases = %+v", runner.lastTask.Cases)
	}
}

// TestQueueRunNoProgress run 不写进度。
func TestQueueRunNoProgress(t *testing.T) {
	db := fakeDB(t)
	subID := seedSubmission(t, db, 7, 99, SubmitTypeRun, `{}`)
	loader := &fakeLoader{subs: map[int64]RuntimeSubmission{
		subID: {ID: subID, UserID: 7, ProblemID: 99, SubmitType: SubmitTypeRun, Language: "python",
			SourceCode: "print(1)", InputData: "x", TimeLimitMS: 1000, MemoryLimitMiB: 256, BodyJSON: `{}`},
	}}
	runner := &fakeRunner{result: RunResult{Verdict: VerdictOK, TimeMS: 1, Stdout: "x"}}
	q := NewQueueService(db, runner, loader, 1)
	q.Start(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		_ = db.QueryRow(`SELECT status FROM submissions WHERE id=?`, subID).Scan(&status)
		if status == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("judge 未完成")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_problem_progress`).Scan(&n)
	if n != 0 {
		t.Fatalf("run 不应写进度: %d", n)
	}
}

// TestQueueFailJob runner 异常 → submissions failed + RE。
func TestQueueFailJob(t *testing.T) {
	db := fakeDB(t)
	subID := seedSubmission(t, db, 7, 99, SubmitTypeSubmit, `{}`)
	loader := &fakeLoader{subs: map[int64]RuntimeSubmission{
		subID: {ID: subID, UserID: 7, ProblemID: 99, SubmitType: SubmitTypeSubmit, Language: "python",
			SourceCode: "print(1)", TimeLimitMS: 1000, MemoryLimitMiB: 256, BodyJSON: `{}`},
	}}
	runner := &fakeRunner{err: errors.New("judge runtime down")}
	q := NewQueueService(db, runner, loader, 1)
	q.Start(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status, verdict string
		_ = db.QueryRow(`SELECT status,verdict FROM submissions WHERE id=?`, subID).Scan(&status, &verdict)
		if status == "failed" {
			if verdict != string(VerdictRE) {
				t.Fatalf("verdict = %s", verdict)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("失败兜底未生效")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNormalizeOutput(t *testing.T) {
	cases := map[string]string{
		"3\n":       "3",
		"3\r\n":     "3",
		"  3  \n":   "3", // 行尾去空白 + 整体 TrimSpace（首行行首随整体 Trim 消失）
		"a \n b \n": "a\n b",
		"":          "",
	}
	for in, want := range cases {
		if got := NormalizeOutput(in); got != want {
			t.Fatalf("NormalizeOutput(%q) = %q, want %q", in, got, want)
		}
	}
}
