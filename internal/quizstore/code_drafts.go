// 云端代码草稿：按 用户×题目×语言 存取（跨端恢复草稿；做题页/训练页共用）。
package quizstore

import (
	"database/sql"
	"errors"
)

// GetDraft 取草稿（无记录返回空串不报错）。
func (s *Store) GetDraft(userID, problemID int64, language string) (string, error) {
	var code string
	err := s.DB.QueryRow(`SELECT code FROM code_drafts
		WHERE user_id=? AND problem_id=? AND language=?`, userID, problemID, language).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

// SaveDraft 保存草稿（upsert；updated_at 刷新）。
func (s *Store) SaveDraft(userID, problemID int64, language, code string) error {
	_, err := s.DB.Exec(`INSERT INTO code_drafts(user_id,problem_id,language,code,updated_at)
		VALUES(?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(user_id,problem_id,language) DO UPDATE SET
		code=excluded.code, updated_at=CURRENT_TIMESTAMP`, userID, problemID, language, code)
	return err
}
