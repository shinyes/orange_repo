// 云端代码草稿：按 用户×题目×语言×上下文 存取（跨端恢复草稿；做题页/训练/练习隔离）。
// ctxKind：''=全局做题页 / training=训练 / practice=练习；ctxID 为对应项目 id。
package quizstore

import (
	"database/sql"
	"errors"
)

// GetDraft 取草稿（无记录返回空串不报错）。
func (s *Store) GetDraft(userID, problemID int64, language, ctxKind string, ctxID int64) (string, error) {
	var code string
	err := s.DB.QueryRow(`SELECT code FROM code_drafts
		WHERE user_id=? AND problem_id=? AND language=? AND ctx_kind=? AND ctx_id=?`,
		userID, problemID, language, ctxKind, ctxID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return code, nil
}

// SaveDraft 保存草稿（upsert；updated_at 刷新）。
func (s *Store) SaveDraft(userID, problemID int64, language, ctxKind string, ctxID int64, code string) error {
	_, err := s.DB.Exec(`INSERT INTO code_drafts(user_id,problem_id,language,ctx_kind,ctx_id,code,updated_at)
		VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(user_id,problem_id,language,ctx_kind,ctx_id) DO UPDATE SET
		code=excluded.code, updated_at=CURRENT_TIMESTAMP`, userID, problemID, language, ctxKind, ctxID, code)
	return err
}
