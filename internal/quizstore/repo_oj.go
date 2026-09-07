// OrangeOJ 判题所需的只读主库读取（problems.go 的扩展）：
// 编程题正文/答案/用例、题目类型摘要。全部 SELECT，绝不写入主库。
package quizstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// OJProblem 做题页所需题目完整正文（不携带判题密钥字段）。
type OJProblem struct {
	ID             int64           `json:"id"`
	Type           string          `json:"type"`
	Title          string          `json:"title"`
	StatementMD    string          `json:"statementMd"`
	BodyJSON       json.RawMessage `json:"bodyJson"`
	StarterCpp     string          `json:"starterCpp,omitempty"`
	StarterPy      string          `json:"starterPy,omitempty"`
	TimeLimitMS    int             `json:"timeLimitMs"`
	MemoryLimitMiB int             `json:"memoryLimitMiB"`
	Tags           []string        `json:"tags"`
}

// GetOJProblem 取题目做题正文（含编程题 inputFormat/outputFormat/samples 与起始代码模板；不含 testCases/answerJson/solutions）。
func (r *RepoReader) GetOJProblem(id int64) (*OJProblem, error) {
	p := &OJProblem{ID: id}
	var tags, body string
	err := r.DB.QueryRow(`SELECT type,title,tags_json,statement_md,body_json,starter_cpp,starter_py,time_limit_ms,memory_limit_mib
		FROM problems WHERE id=?`, id).
		Scan(&p.Type, &p.Title, &tags, &p.StatementMD, &body, &p.StarterCpp, &p.StarterPy, &p.TimeLimitMS, &p.MemoryLimitMiB)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tags = decodeStrings(tags)
	p.BodyJSON = json.RawMessage(body)
	return p, nil
}

// LoadJudgeProblemData 队列用：读题目时限/内存/body_json。
func (r *RepoReader) LoadJudgeProblemData(ctx context.Context, id int64) (timeLimitMS, memoryLimitMiB int, bodyJSON string, err error) {
	err = r.DB.QueryRowContext(ctx, `SELECT time_limit_ms,memory_limit_mib,body_json FROM problems WHERE id=?`, id).
		Scan(&timeLimitMS, &memoryLimitMiB, &bodyJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, "", ErrNotFound
	}
	if err != nil {
		return 0, 0, "", err
	}
	return timeLimitMS, memoryLimitMiB, bodyJSON, nil
}

// GetObjectiveAnswer 客观题判题答案（obj/submit 与空间训练/练习/刷题判题共用）。
func (r *RepoReader) GetObjectiveAnswer(id int64) (*AnswerEnvelope, error) {
	return r.GetAnswer(id)
}
