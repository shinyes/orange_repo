// 全局错题集：刷题（quiz）答错入集（按题目去重，保留首个来源刷题项目），
// 任何一处答对即删除；支持按 来源项目 分组重刷与全部重刷。
package quizstore

import (
	"database/sql"
	"errors"
)

// AddWrong 记错题（已有则保留原来源，不覆盖）。
func (s *Store) AddWrong(userID, problemID, quizID int64) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO wrong_book(user_id,problem_id,quiz_id,wrong_at)
		VALUES(?,?,?,CURRENT_TIMESTAMP)`, userID, problemID, quizID)
	return err
}

// RemoveWrongByProblem 答对后从错题集移除（任意来源）。
func (s *Store) RemoveWrongByProblem(userID, problemID int64) error {
	_, err := s.DB.Exec(`DELETE FROM wrong_book WHERE user_id=? AND problem_id=?`, userID, problemID)
	return err
}

// WrongProblem 错题条目。
type WrongProblem struct {
	ProblemID int64 `json:"problemId"`
	QuizID    int64 `json:"quizId"` // 来源刷题项目
	WrongAt   string `json:"wrongAt"`
}

// ListWrongProblems 用户全部错题。
func (s *Store) ListWrongProblems(userID int64) ([]WrongProblem, error) {
	rows, err := s.DB.Query(`SELECT problem_id,quiz_id,COALESCE(wrong_at,'') FROM wrong_book
		WHERE user_id=? ORDER BY rowid DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWrongProblems(rows)
}

// ListWrongProblemsOfQuiz 某刷题项目的错题（用于该项目错题重刷）。
func (s *Store) ListWrongProblemsOfQuiz(userID, quizID int64) ([]WrongProblem, error) {
	rows, err := s.DB.Query(`SELECT problem_id,quiz_id,COALESCE(wrong_at,'') FROM wrong_book
		WHERE user_id=? AND quiz_id=? ORDER BY rowid DESC`, userID, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWrongProblems(rows)
}

// WrongCount 错题总数。
func (s *Store) WrongCount(userID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM wrong_book WHERE user_id=?`, userID).Scan(&n)
	return n, err
}

// IsWrongProblem 是否在错题集。
func (s *Store) IsWrongProblem(userID, problemID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM wrong_book WHERE user_id=? AND problem_id=?`, userID, problemID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return n > 0, err
}

func scanWrongProblems(rows *sql.Rows) ([]WrongProblem, error) {
	out := []WrongProblem{}
	for rows.Next() {
		var w WrongProblem
		if err := rows.Scan(&w.ProblemID, &w.QuizID, &w.WrongAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
