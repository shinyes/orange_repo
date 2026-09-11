// 云端代码草稿：按 用户×题目×语言×上下文 存取（跨端恢复草稿；做题页/训练/练习隔离）。
// ctxKind：''=全局做题页 / training=训练 / practice=练习；ctxID 为对应项目 id。
package quizstore

import (
	"database/sql"
	"errors"
	"time"
)

// Draft 云端代码草稿（Code 为空串表示无草稿；UpdatedAt 零值表示无记录）。
type Draft struct {
	Code string
	// UpdatedAt 最后保存时间（RFC3339，带时区）——多设备场景下由前端比较新旧
	UpdatedAt time.Time
}

// GetDraft 取草稿（无记录返回零值，不报错）。
func (s *Store) GetDraft(userID, problemID int64, language, ctxKind string, ctxID int64) (Draft, error) {
	var code, rawUpdated string
	err := s.DB.QueryRow(`SELECT code,COALESCE(updated_at,'') FROM code_drafts
		WHERE user_id=? AND problem_id=? AND language=? AND ctx_kind=? AND ctx_id=?`,
		userID, problemID, language, ctxKind, ctxID).Scan(&code, &rawUpdated)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, nil
	}
	if err != nil {
		return Draft{}, err
	}
	return Draft{Code: code, UpdatedAt: parseDraftTime(rawUpdated)}, nil
}

// parseDraftTime 兼容两种存量格式：SQLite CURRENT_TIMESTAMP("YYYY-MM-DD HH:MM:SS" UTC)
// 与 RFC3339——统一为 UTC time.Time（输出 RFC3339，前端可直接 Date.parse）。
func parseDraftTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.UTC); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// SaveDraft 保存草稿（upsert；updated_at 刷新）。
func (s *Store) SaveDraft(userID, problemID int64, language, ctxKind string, ctxID int64, code string) error {
	_, err := s.DB.Exec(`INSERT INTO code_drafts(user_id,problem_id,language,ctx_kind,ctx_id,code,updated_at)
		VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(user_id,problem_id,language,ctx_kind,ctx_id) DO UPDATE SET
		code=excluded.code, updated_at=CURRENT_TIMESTAMP`, userID, problemID, language, ctxKind, ctxID, code)
	return err
}
