// 刷题会话（批式规则引擎）：每 用户×刷题项目 一行，维护当前批号、
// 批内已抽题（drawn）与会话答错待纠正题（wrong）。
// 规则：答对的做过题少抽（批内不重复、批覆盖后不再主动出）；答错的题跨批保留优先复抽；
// 批内已抽题不重复。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// QuizSession 刷题会话状态。
type QuizSession struct {
	BatchNo int     `json:"batchNo"`
	Wrong   []int64 `json:"wrong"`
	Drawn   []int64 `json:"drawn"`
	Covered []int64 `json:"covered"` // 跨批累计抽出的题目（每轮题数模式下判“范围全部刷过”）
}

// GetQuizSession 读会话（无记录返回零值）。
func (s *Store) GetQuizSession(userID, quizID int64) (*QuizSession, error) {
	var wrongJSON, drawnJSON, coveredJSON string
	var batch int
	err := s.DB.QueryRow(`SELECT batch_no,wrong_json,drawn_json,COALESCE(covered_json,'[]') FROM quiz_sessions
		WHERE user_id=? AND quiz_id=?`, userID, quizID).Scan(&batch, &wrongJSON, &drawnJSON, &coveredJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return &QuizSession{BatchNo: 1, Wrong: []int64{}, Drawn: []int64{}, Covered: []int64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	ss := &QuizSession{BatchNo: batch}
	_ = json.Unmarshal([]byte(wrongJSON), &ss.Wrong)
	_ = json.Unmarshal([]byte(drawnJSON), &ss.Drawn)
	_ = json.Unmarshal([]byte(coveredJSON), &ss.Covered)
	if ss.Wrong == nil {
		ss.Wrong = []int64{}
	}
	if ss.Drawn == nil {
		ss.Drawn = []int64{}
	}
	if ss.Covered == nil {
		ss.Covered = []int64{}
	}
	return ss, nil
}

// SaveQuizSession upsert 会话（wrong/drawn/covered 全量）。
func (s *Store) SaveQuizSession(userID, quizID int64, ss *QuizSession) error {
	wrongJSON, _ := json.Marshal(ss.Wrong)
	drawnJSON, _ := json.Marshal(ss.Drawn)
	coveredJSON, _ := json.Marshal(ss.Covered)
	_, err := s.DB.Exec(`INSERT INTO quiz_sessions(user_id,quiz_id,batch_no,wrong_json,drawn_json,covered_json,updated_at)
		VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(user_id,quiz_id) DO UPDATE SET
		batch_no=excluded.batch_no,wrong_json=excluded.wrong_json,drawn_json=excluded.drawn_json,
		covered_json=excluded.covered_json,updated_at=CURRENT_TIMESTAMP`,
		userID, quizID, ss.BatchNo, string(wrongJSON), string(drawnJSON), string(coveredJSON))
	return err
}

// ResetQuizSession 清除会话（重新开始一轮完整刷题）。
func (s *Store) ResetQuizSession(userID, quizID int64) error {
	_, err := s.DB.Exec(`DELETE FROM quiz_sessions WHERE user_id=? AND quiz_id=?`, userID, quizID)
	return err
}

// ContainsInt64 判包含。
func ContainsInt64(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// RemoveInt64 移除全部匹配元素。
func RemoveInt64(list []int64, v int64) []int64 {
	out := make([]int64, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
