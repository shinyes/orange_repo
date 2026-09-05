// RepoReader：以只读模式（mode=ro）访问主站题库数据库 orangeoj.db。
// 本文件只有 SELECT，绝不迁移、绝不写入主库。
// 标签匹配/题目正文读取按判题场景直接以 SQL 取数（见 repo_oj.go/repo_space.go）。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// RepoReader 主库只读句柄。
type RepoReader struct {
	DB *sql.DB
}

// AnswerEnvelope 判题所需答案形状（按题型解释）。
type AnswerEnvelope struct {
	Type        string
	AnswerIndex *int
	Answer      *bool
}

// OpenRepoReader 只读打开主库题库并探活（确认存在 problems 表）。
// 只读连接放开并发上限：主库为 WAL 模式（主站维护），多连接只读可并行，
// 避免大量并发读被单连接串行化（原 SetMaxOpenConns(1) 在刷题/判题读密集下是瓶颈）。
func OpenRepoReader(path string) (*RepoReader, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open repo sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='problems'`).Scan(&n); err != nil || n == 0 {
		db.Close()
		return nil, fmt.Errorf("未找到题库数据库 %s：请先运行主站服务初始化题库", path)
	}
	return &RepoReader{DB: db}, nil
}

// GetAnswer 读取判题所需答案。
// single_choice → AnswerIndex（题面选项下标）；true_false → Answer 布尔。
func (r *RepoReader) GetAnswer(id int64) (*AnswerEnvelope, error) {
	var typ, answerJSON string
	err := r.DB.QueryRow(`SELECT type,answer_json FROM problems WHERE id=?`, id).Scan(&typ, &answerJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	env := &AnswerEnvelope{Type: typ}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(answerJSON), &raw); err != nil {
		return nil, fmt.Errorf("题目 %d 答案格式异常: %w", id, err)
	}
	switch typ {
	case "single_choice":
		var idx int
		if err := json.Unmarshal(raw["answerIndex"], &idx); err != nil {
			return nil, fmt.Errorf("题目 %d 缺少有效的 answerIndex", id)
		}
		env.AnswerIndex = &idx
	case "true_false":
		var b bool
		if err := json.Unmarshal(raw["answer"], &b); err != nil {
			return nil, fmt.Errorf("题目 %d 缺少有效的 answer 布尔值", id)
		}
		env.Answer = &b
	default:
		return nil, fmt.Errorf("题目 %d 类型 %s 暂不支持刷题（第一阶段仅单选/判断）", id, typ)
	}
	return env, nil
}
