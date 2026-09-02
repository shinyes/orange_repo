// OrangeOJ 判题所需的只读主库读取（problems.go 的扩展）：
// 编程题正文/答案/用例、训练/练习结构（实时跟随）、判断题目是否属于某训练/练习。
// 全部 SELECT，绝不写入主库。
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
	TimeLimitMS    int             `json:"timeLimitMs"`
	MemoryLimitMiB int             `json:"memoryLimitMiB"`
	Tags           []string        `json:"tags"`
}

// GetOJProblem 取题目做题正文（含编程题 inputFormat/outputFormat/samples；不含 testCases/answerJson/solutions）。
func (r *RepoReader) GetOJProblem(id int64) (*OJProblem, error) {
	p := &OJProblem{ID: id}
	var tags, body string
	err := r.DB.QueryRow(`SELECT type,title,tags_json,statement_md,body_json,time_limit_ms,memory_limit_mib
		FROM problems WHERE id=?`, id).
		Scan(&p.Type, &p.Title, &tags, &p.StatementMD, &body, &p.TimeLimitMS, &p.MemoryLimitMiB)
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

// GetObjectiveAnswer 客观题判题（与一期 GetAnswer 等价但供 objective-submit 复用）。
func (r *RepoReader) GetObjectiveAnswer(id int64) (*AnswerEnvelope, error) {
	return r.GetAnswer(id)
}

// RepoTrainings 主库训练目录（管理员布置来源列表）。
type RepoTrainings struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ProblemCount int      `json:"problemCount"`
	ChapterCount int      `json:"chapterCount"`
}

// ListRepoTrainings 全部训练（含题数/章数）。
func (r *RepoReader) ListRepoTrainings() ([]RepoTrainings, error) {
	rows, err := r.DB.Query(`SELECT id,title,description,tags_json FROM trainings ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RepoTrainings{}
	for rows.Next() {
		var t RepoTrainings
		var tags string
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &tags); err != nil {
			return nil, err
		}
		t.Tags = decodeStrings(tags)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		chapters, err := r.TrainingProblemIDs(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].ChapterCount = len(chapters)
		n := 0
		for _, ids := range chapters {
			n += len(ids)
		}
		out[i].ProblemCount = n
	}
	return out, nil
}

// TrainingChapter 训练章节结构（含题目 id 列表）。
type TrainingChapter struct {
	ID      int64   `json:"id"`
	Title   string  `json:"title"`
	OrderNo int     `json:"orderNo"`
	Items   []int64 `json:"items"`
}

// GetRepoTraining 单训练结构：{title, description, tags, chapters}。
func (r *RepoReader) GetRepoTraining(id int64) (*RepoTraining, error) {
	t := &RepoTraining{ID: id}
	var tags string
	err := r.DB.QueryRow(`SELECT title,description,tags_json FROM trainings WHERE id=?`, id).
		Scan(&t.Title, &t.Description, &tags)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Tags = decodeStrings(tags)

	rows, err := r.DB.Query(`SELECT c.id,c.title,c.order_no,i.problem_id
		FROM training_chapters c
		LEFT JOIN training_items i ON i.chapter_id=c.id
		WHERE c.training_id=?
		ORDER BY c.order_no,c.id,i.order_no,i.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]*TrainingChapter{}
	var order []int64
	for rows.Next() {
		var cid int64
		var title string
		var orderNo int
		var pid sql.NullInt64
		if err := rows.Scan(&cid, &title, &orderNo, &pid); err != nil {
			return nil, err
		}
		ch, ok := byID[cid]
		if !ok {
			ch = &TrainingChapter{ID: cid, Title: title, OrderNo: orderNo, Items: []int64{}}
			byID[cid] = ch
			order = append(order, cid)
		}
		if pid.Valid {
			ch.Items = append(ch.Items, pid.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	t.Chapters = make([]TrainingChapter, 0, len(order))
	for _, cid := range order {
		t.Chapters = append(t.Chapters, *byID[cid])
	}
	return t, nil
}

// RepoTraining 训练结构。
type RepoTraining struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	Chapters    []TrainingChapter `json:"chapters"`
}

// TrainingProblemIDs 训练各章节题目 id（按顺序）。
func (r *RepoReader) TrainingProblemIDs(id int64) ([][]int64, error) {
	rows, err := r.DB.Query(`SELECT c.id,i.problem_id
		FROM training_chapters c
		LEFT JOIN training_items i ON i.chapter_id=c.id
		WHERE c.training_id=?
		ORDER BY c.order_no,c.id,i.order_no,i.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := [][]int64{}
	seen := map[int64]int{} // chapter id → index
	for rows.Next() {
		var cid int64
		var pid sql.NullInt64
		if err := rows.Scan(&cid, &pid); err != nil {
			return nil, err
		}
		idx, ok := seen[cid]
		if !ok {
			idx = len(out)
			seen[cid] = idx
			out = append(out, []int64{})
		}
		if pid.Valid {
			out[idx] = append(out[idx], pid.Int64)
		}
	}
	return out, rows.Err()
}

// RepoPractice 练习结构。
type RepoPractice struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	Items        []int64  `json:"items"`
	ProblemCount int      `json:"problemCount"`
}

// ListRepoPractices 主库练习目录。
func (r *RepoReader) ListRepoPractices() ([]RepoPractice, error) {
	rows, err := r.DB.Query(`SELECT id,title,description,tags_json FROM practices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RepoPractice{}
	for rows.Next() {
		var p RepoPractice
		var tags string
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &tags); err != nil {
			return nil, err
		}
		p.Tags = decodeStrings(tags)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ids, err := r.PracticeProblemIDs(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = ids
		out[i].ProblemCount = len(ids)
	}
	return out, nil
}

// GetRepoPractice 单练习。
func (r *RepoReader) GetRepoPractice(id int64) (*RepoPractice, error) {
	p := &RepoPractice{ID: id}
	var tags string
	err := r.DB.QueryRow(`SELECT title,description,tags_json FROM practices WHERE id=?`, id).
		Scan(&p.Title, &p.Description, &tags)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tags = decodeStrings(tags)
	ids, err := r.PracticeProblemIDs(id)
	if err != nil {
		return nil, err
	}
	p.Items = ids
	p.ProblemCount = len(ids)
	return p, nil
}

// PracticeProblemIDs 练习题目 id（按顺序）。
func (r *RepoReader) PracticeProblemIDs(id int64) ([]int64, error) {
	rows, err := r.DB.Query(`SELECT problem_id FROM practice_items WHERE practice_id=? ORDER BY order_no,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var pid int64
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out = append(out, pid)
	}
	return out, rows.Err()
}

// ProblemExists 题目是否存在。
func (r *RepoReader) ProblemExists(id int64) (bool, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

// ProblemsBrief 批量取题目摘要（用于列表标题/题型/AC 态展示）。
type ProblemBrief struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Type  string   `json:"type"`
	Tags  []string `json:"tags"`
}

// GetProblemsBrief 批量按 id 读摘要（保持传入顺序；缺失项跳过）。
func (r *RepoReader) GetProblemsBrief(ids []int64) (map[int64]ProblemBrief, error) {
	out := map[int64]ProblemBrief{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := ""
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			ph += ","
		}
		ph += "?"
		args = append(args, id)
	}
	rows, err := r.DB.Query(`SELECT id,title,type,tags_json FROM problems WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b ProblemBrief
		var tags string
		if err := rows.Scan(&b.ID, &b.Title, &b.Type, &tags); err != nil {
			return nil, err
		}
		b.Tags = decodeStrings(tags)
		out[b.ID] = b
	}
	return out, rows.Err()
}

// TrainingProblemCount 训练总题数（缺失章节题目容错）。
func (r *RepoReader) TrainingProblemCount(id int64) (int, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(i.id) FROM training_chapters c
		JOIN training_items i ON i.chapter_id=c.id WHERE c.training_id=?`, id).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// PracticeProblemCount 练习总题数。
func (r *RepoReader) PracticeProblemCount(id int64) (int, error) {
	var n int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM practice_items WHERE practice_id=?`, id).Scan(&n)
	return n, err
}
