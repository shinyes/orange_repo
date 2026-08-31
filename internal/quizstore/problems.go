// RepoReader：以只读模式（mode=ro）访问主站题库数据库 orangerepo.db。
// 本文件只有 SELECT，绝不迁移、绝不写入主库；标签匹配复用 store.TagMatchesSelected。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"orangerepo/internal/store"
)

// RepoReader 主库只读句柄。
type RepoReader struct {
	DB *sql.DB
}

// QuizProblem 刷题场景的题目内容：不含答案与题解正文，仅携带 hasExplanation 标记。
type QuizProblem struct {
	ID             int64           `json:"id"`
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	HasExplanation bool            `json:"hasExplanation"`
}

// AnswerEnvelope 判题所需答案形状（按题型解释）。
type AnswerEnvelope struct {
	Type        string
	AnswerIndex *int
	Answer      *bool
}

// defaultQuizTypes 第一阶段允许的题型（分类未配置时默认两种都算）。
var defaultQuizTypes = []string{"single_choice", "true_false"}

// OpenRepoReader 只读打开主库题库并探活（确认存在 problems 表）。
func OpenRepoReader(path string) (*RepoReader, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open repo sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='problems'`).Scan(&n); err != nil || n == 0 {
		db.Close()
		return nil, fmt.Errorf("未找到题库数据库 %s：请先运行主站服务初始化题库", path)
	}
	return &RepoReader{DB: db}, nil
}

// MatchingProblems 返回符合筛选（标签前缀 AND + 题型 IN）的题目，按 id 升序。
// 标签为空 = 不限标签；题型为空 = 默认单选+判断。
func (r *RepoReader) MatchingProblems(tags, types []string) ([]QuizProblem, error) {
	qtypes := types
	if len(qtypes) == 0 {
		qtypes = defaultQuizTypes
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(qtypes)), ",")
	args := make([]any, 0, len(qtypes))
	for _, t := range qtypes {
		args = append(args, t)
	}
	rows, err := r.DB.Query(`SELECT id,type,title,statement_md,body_json,solutions_json,tags_json
		FROM problems WHERE type IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuizProblem
	for rows.Next() {
		var p QuizProblem
		var title, stmt, body, sols, tagsJSON string
		if err := rows.Scan(&p.ID, &p.Type, &title, &stmt, &body, &sols, &tagsJSON); err != nil {
			return nil, err
		}
		p.Title = title
		p.StatementMD = stmt
		p.BodyJSON = json.RawMessage(body)
		p.HasExplanation = hasExplanationMarkdown(sols)
		if store.TagMatchesSelected(decodeStrings(tagsJSON), tags) {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// CountProblems 符合筛选的题目数。
func (r *RepoReader) CountProblems(tags, types []string) (int, error) {
	list, err := r.MatchingProblems(tags, types)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

// GetQuizProblem 取单题刷题内容。
func (r *RepoReader) GetQuizProblem(id int64) (*QuizProblem, error) {
	p := &QuizProblem{ID: id}
	var title, stmt, body, sols string
	err := r.DB.QueryRow(`SELECT type,title,statement_md,body_json,solutions_json
		FROM problems WHERE id=?`, id).
		Scan(&p.Type, &title, &stmt, &body, &sols)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Title = title
	p.StatementMD = stmt
	p.BodyJSON = json.RawMessage(body)
	p.HasExplanation = hasExplanationMarkdown(sols)
	return p, nil
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

// GetExplanation 取题解中的解析 markdown（第一条非空 markdown；无则返回 false）。
func (r *RepoReader) GetExplanation(id int64) (string, bool) {
	var sols string
	if err := r.DB.QueryRow(`SELECT solutions_json FROM problems WHERE id=?`, id).Scan(&sols); err != nil {
		return "", false
	}
	var entries []struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(sols), &entries); err != nil {
		return "", false
	}
	for _, e := range entries {
		if strings.TrimSpace(e.Markdown) != "" {
			return e.Markdown, true
		}
	}
	return "", false
}

// hasExplanationMarkdown solutions 是否存在非空 markdown 条目。
func hasExplanationMarkdown(solutionsJSON string) bool {
	var entries []struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(solutionsJSON), &entries); err != nil {
		return false
	}
	for _, e := range entries {
		if strings.TrimSpace(e.Markdown) != "" {
			return true
		}
	}
	return false
}