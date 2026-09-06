// RepoReader：单库 orangeoj.db 的题库侧读取句柄（与作答同库；仅承载题库/空间结构 SELECT，
// 指向与 Store 同一连接，见 quizstore.Open）。
// 标签匹配/题目正文读取按判题场景直接以 SQL 取数（见 repo_oj.go/repo_space.go）。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// RepoReader 题库侧读取句柄（同一 orangeoj.db 连接）。
type RepoReader struct {
	DB *sql.DB
}

// AnswerEnvelope 判题所需答案形状（按题型解释）。
type AnswerEnvelope struct {
	Type        string
	AnswerIndex *int
	Answer      *bool
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
