// 空间训练/练习学生作答数据层（orangeoj.db，与 users 同库、FK 级联、判题管线同侧）。
// 空间/训练/练习/条目的结构定义在主库 internal/store（store_space_content.go），
// 此处只存学生作答：训练尝试限次/标色、练习整卷交卷、通过记录（uuid 去重，排行榜）。
// training_id/practice_id 指主库空间训练/练习 id，无跨库外键；problem_uuid 直接由调用方传入，
// 避免跨库查题（主库只读经 Repo 访问）。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ---------- 训练客观题尝试记录（限次/标色） ----------

// AttemptState 单用户在某训练内对某题的作答状态（无记录=0 次）。
type AttemptState struct {
	Attempts int  `json:"attempts"`
	Solved   bool `json:"solved"`
}

// GetTrainingAttempt 取尝试状态（无记录=0 次，不报错）。
func (s *Store) GetTrainingAttempt(trainingID, userID, problemID int64) (*AttemptState, error) {
	st := &AttemptState{}
	err := s.DB.QueryRow(`SELECT attempts,solved FROM space_training_attempts
		WHERE training_id=? AND user_id=? AND problem_id=?`, trainingID, userID, problemID).
		Scan(&st.Attempts, &st.Solved)
	if errors.Is(err, sql.ErrNoRows) {
		return &AttemptState{}, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// RecordTrainingAttempt 记录一次客观题作答：attempts+1（原子，防并发丢更新）；答对标 solved 并写通过记录。
// problemUUID 由调用方从主库题目读出传入（避开跨库查题）；
// 已达上限或已 solved 由调用方先拦截止步，此处幂等处理已 solved 情形不重复计数。
// 返回 (attempts, solved, err)。
func (s *Store) RecordTrainingAttempt(trainingID, userID, problemID int64, problemUUID string, correct bool) (int, bool, error) {
	// 单条原子 upsert：attempts 在库内自增；答对时 solved=1（幂等：已 solved 不再 +1/不再置位变化）
	_, err := s.DB.Exec(`INSERT INTO space_training_attempts(training_id,user_id,problem_id,attempts,solved)
		VALUES(?,?,?,1,?)
		ON CONFLICT(training_id,user_id,problem_id) DO UPDATE SET
		attempts=CASE WHEN space_training_attempts.solved=1 THEN space_training_attempts.attempts
			ELSE space_training_attempts.attempts+1 END,
		solved=CASE WHEN excluded.solved=1 THEN 1 ELSE space_training_attempts.solved END,
		updated_at=CURRENT_TIMESTAMP`,
		trainingID, userID, problemID, b2i(correct))
	if err != nil {
		return 0, false, err
	}
	// 读回最新计数（原子自增后的一致性值；并发下展示值可能略超前于本请求，属可接受）
	var attempts int
	var solved bool
	if err := s.DB.QueryRow(`SELECT attempts,solved FROM space_training_attempts
		WHERE training_id=? AND user_id=? AND problem_id=?`, trainingID, userID, problemID).
		Scan(&attempts, &solved); err != nil {
		return 0, false, err
	}
	// 答对 → 写通过记录（uuid 去重幂等；uuid 空则跳过）
	if correct && problemUUID != "" {
		if err := s.RecordSolved(userID, problemUUID); err != nil {
			return 0, false, err
		}
	}
	return attempts, solved, nil
}

// b2i bool → 0/1。
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- 通过记录（uuid 去重，排行榜） ----------

// MarkTrainingProgrammingSolved 训练内编程题通过标记（提交判 AC 后由 poll 落定调用）：
// 置 space_training_attempts.solved=1（attempts 不计数——编程题不限次），并写 uuid 通过记录。
// 幂等：已 solved 直接返回。
func (s *Store) MarkTrainingProgrammingSolved(trainingID, userID, problemID int64, problemUUID string) error {
	var curSolved bool
	err := s.DB.QueryRow(`SELECT solved FROM space_training_attempts
		WHERE training_id=? AND user_id=? AND problem_id=?`, trainingID, userID, problemID).Scan(&curSolved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if curSolved {
		return nil
	}
	_, err = s.DB.Exec(`INSERT INTO space_training_attempts(training_id,user_id,problem_id,attempts,solved)
		VALUES(?,?,?,0,1)
		ON CONFLICT(training_id,user_id,problem_id) DO UPDATE SET solved=1, updated_at=CURRENT_TIMESTAMP`,
		trainingID, userID, problemID)
	if err != nil {
		return err
	}
	if problemUUID != "" {
		if err := s.RecordSolved(userID, problemUUID); err != nil {
			return err
		}
	}
	return nil
}

// RecordSolved 直接按 uuid 记录用户通过（去重；uuid 校验在调用方）。
func (s *Store) RecordSolved(userID int64, problemUUID string) error {
	if problemUUID == "" {
		return nil
	}
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO student_solved(user_id,problem_uuid) VALUES(?,?)`, userID, problemUUID)
	return err
}

// SolvedUUIDs 用户已通过的题目 uuid 集合（去重查询用）。
func (s *Store) SolvedUUIDs(userID int64) (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT problem_uuid FROM student_solved WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out[u] = true
	}
	return out, rows.Err()
}

// ---------- 练习交卷记录 ----------

// PracticeSubmission 练习交卷记录视图。
type PracticeSubmission struct {
	ID               int64     `json:"id"`
	PracticeID       int64     `json:"practiceId"`
	UserID           int64     `json:"userId"`
	ObjectiveCorrect int       `json:"objectiveCorrect"`
	CreatedAt        time.Time `json:"createdAt"` // RFC3339（与判题 submissions 同格式，供前端按时点过滤）
}

// SavePracticeSubmission 保存一次交卷（answersJSON 为快照 JSON，元素 {problemId,correct,uuid}），
// 答对且 uuid 非空的题写通过记录（INSERT OR IGNORE 去重），再插交卷记录；事务内，返回提交 id。
func (s *Store) SavePracticeSubmission(practiceID, userID int64, answersJSON string, objectiveCorrect int) (int64, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// 解析快照：答对且快照内带 uuid 的题写通过记录（uuid 缺失不查主库，跳过）
	type answerItem struct {
		ProblemID int64  `json:"problemId"`
		Correct   bool   `json:"correct"`
		UUID      string `json:"uuid,omitempty"`
	}
	var items []answerItem
	_ = json.Unmarshal([]byte(answersJSON), &items)
	for _, it := range items {
		if !it.Correct || it.UUID == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO student_solved(user_id,problem_uuid) VALUES(?,?)`, userID, it.UUID); err != nil {
			return 0, err
		}
	}
	res, err := tx.Exec(`INSERT INTO space_practice_submissions(practice_id,user_id,answers_json,objective_correct)
		VALUES(?,?,?,?)`, practiceID, userID, answersJSON, objectiveCorrect)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPracticeSubmissions 练习的提交记录（按 id 倒序；供结果回顾）。
func (s *Store) ListPracticeSubmissions(practiceID, userID int64) ([]PracticeSubmission, error) {
	rows, err := s.DB.Query(`SELECT id,practice_id,user_id,objective_correct,created_at
		FROM space_practice_submissions WHERE practice_id=? AND user_id=? ORDER BY id DESC`, practiceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PracticeSubmission
	for rows.Next() {
		var sub PracticeSubmission
		var rawCreated string
		if err := rows.Scan(&sub.ID, &sub.PracticeID, &sub.UserID, &sub.ObjectiveCorrect, &rawCreated); err != nil {
			return nil, err
		}
		// SQLite CURRENT_TIMESTAMP 存 "YYYY-MM-DD HH:MM:SS"（UTC）→ 转 time.Time（RFC3339 输出）
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", rawCreated, time.UTC); err == nil {
			sub.CreatedAt = t
		}
		out = append(out, sub)
	}
	return nonNilSlice(out), rows.Err()
}

// GetPracticeSubmission 取交卷快照（不存在返回 ErrNotFound）。
func (s *Store) GetPracticeSubmission(submissionID int64) (answersJSON string, err error) {
	err = s.DB.QueryRow(`SELECT answers_json FROM space_practice_submissions WHERE id=?`, submissionID).Scan(&answersJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return answersJSON, err
}
