// Package store 封装 SQLite 持久化：迁移、设置、目录树、题目。
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"orangerepo/internal/model"
)

// ErrNotFound 统一的未找到错误。
var ErrNotFound = errors.New("not found")

// Store 数据库句柄与数据目录。
type Store struct {
	DB      *sql.DB
	DataDir string
}

// Open 打开（必要时创建）数据目录与数据库，并执行迁移。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "uploads"), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "orangerepo.db")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写者，避免锁竞争
	s := &Store{DB: db, DataDir: dataDir}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS directories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES directories(id) ON DELETE SET NULL,
			order_no INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS problems (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			statement_md TEXT NOT NULL DEFAULT '',
			body_json TEXT NOT NULL DEFAULT '{}',
			answer_json TEXT NOT NULL DEFAULT '{}',
			solutions_json TEXT NOT NULL DEFAULT '[]',
			time_limit_ms INTEGER NOT NULL DEFAULT 1000,
			memory_limit_mib INTEGER NOT NULL DEFAULT 256,
			directory_id INTEGER REFERENCES directories(id) ON DELETE SET NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS trainings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS training_chapters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			training_id INTEGER NOT NULL REFERENCES trainings(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS training_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chapter_id INTEGER NOT NULL REFERENCES training_chapters(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL REFERENCES problems(id),
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS practices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS practice_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			practice_id INTEGER NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL REFERENCES problems(id),
			order_no INTEGER NOT NULL DEFAULT 0,
			score INTEGER NOT NULL DEFAULT 100
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

// ---------- 设置 ----------

func (s *Store) GetSetting(key string) (string, bool) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---------- 目录 ----------

type flatDir struct {
	ID       int64
	Name     string
	ParentID *int64
	OrderNo  int
}

func (s *Store) CreateDirectory(name string, parentID *int64) (int64, error) {
	if parentID != nil {
		if _, err := s.GetDirectory(*parentID); err != nil {
			return 0, err
		}
	}
	var order int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0)+1 FROM directories WHERE parent_id IS ?`,
		parentID).Scan(&order)
	res, err := s.DB.Exec(`INSERT INTO directories(name,parent_id,order_no) VALUES(?,?,?)`,
		name, parentID, order)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetDirectory(id int64) (*flatDir, error) {
	d := &flatDir{}
	err := s.DB.QueryRow(`SELECT id,name,parent_id,order_no FROM directories WHERE id=?`, id).
		Scan(&d.ID, &d.Name, &d.ParentID, &d.OrderNo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// UpdateDirectory 更新名称/父级/排序。禁止把目录挂到自己的后代上。
func (s *Store) UpdateDirectory(id int64, name string, parentID *int64, orderNo int) error {
	if _, err := s.GetDirectory(id); err != nil {
		return err
	}
	if parentID != nil && *parentID == id {
		return errors.New("directory cannot be its own parent")
	}
	if parentID != nil {
		descendants, err := s.descendantIDs(id)
		if err != nil {
			return err
		}
		for _, d := range descendants {
			if d == *parentID {
				return errors.New("cannot move directory under its own descendant")
			}
		}
		if _, err := s.GetDirectory(*parentID); err != nil {
			return err
		}
	}
	_, err := s.DB.Exec(`UPDATE directories SET name=?,parent_id=?,order_no=? WHERE id=?`,
		name, parentID, orderNo, id)
	return err
}

// DeleteDirectory 删除目录；其子目录与题目上移到被删目录的父级。
func (s *Store) DeleteDirectory(id int64) error {
	d, err := s.GetDirectory(id)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE directories SET parent_id=? WHERE parent_id=?`, d.ParentID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE problems SET directory_id=? WHERE directory_id=?`, d.ParentID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM directories WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) allDirectories() ([]flatDir, error) {
	rows, err := s.DB.Query(`SELECT id,name,parent_id,order_no FROM directories ORDER BY order_no,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []flatDir
	for rows.Next() {
		var d flatDir
		if err := rows.Scan(&d.ID, &d.Name, &d.ParentID, &d.OrderNo); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// descendantIDs 返回 id 的全部后代目录 id（不含自身）。
func (s *Store) descendantIDs(id int64) ([]int64, error) {
	all, err := s.allDirectories()
	if err != nil {
		return nil, err
	}
	children := map[int64][]int64{}
	for _, d := range all {
		if d.ParentID != nil {
			children[*d.ParentID] = append(children[*d.ParentID], d.ID)
		}
	}
	var out []int64
	var walk func(int64)
	walk = func(pid int64) {
		for _, c := range children[pid] {
			out = append(out, c)
			walk(c)
		}
	}
	walk(id)
	return out, nil
}

// DirectoryTree 装配完整目录树（含每目录直接题目数）。
func (s *Store) DirectoryTree() ([]model.DirectoryNode, error) {
	all, err := s.allDirectories()
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	rows, err := s.DB.Query(`SELECT directory_id, COUNT(*) FROM problems WHERE directory_id IS NOT NULL GROUP BY directory_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var c int
		if err := rows.Scan(&id, &c); err != nil {
			return nil, err
		}
		counts[id] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	nodes := map[int64]*model.DirectoryNode{}
	for _, d := range all {
		nodes[d.ID] = &model.DirectoryNode{ID: d.ID, Name: d.Name, ParentID: d.ParentID, OrderNo: d.OrderNo, ProblemCount: counts[d.ID]}
	}
	var roots []*model.DirectoryNode
	for _, d := range all {
		n := nodes[d.ID]
		if d.ParentID != nil {
			if p, ok := nodes[*d.ParentID]; ok {
				p.Children = append(p.Children, *n)
				continue
			}
		}
		roots = append(roots, n)
	}
	var toVals func(list []*model.DirectoryNode) []model.DirectoryNode
	toVals = func(list []*model.DirectoryNode) []model.DirectoryNode {
		out := make([]model.DirectoryNode, 0, len(list))
		for _, n := range list {
			n.Children = toVals(ptrSlice(n.Children))
			sort.SliceStable(n.Children, func(i, j int) bool {
				if n.Children[i].OrderNo != n.Children[j].OrderNo {
					return n.Children[i].OrderNo < n.Children[j].OrderNo
				}
				return n.Children[i].ID < n.Children[j].ID
			})
			out = append(out, *n)
		}
		return out
	}
	vals := toVals(roots)
	sort.SliceStable(vals, func(i, j int) bool {
		if vals[i].OrderNo != vals[j].OrderNo {
			return vals[i].OrderNo < vals[j].OrderNo
		}
		return vals[i].ID < vals[j].ID
	})
	return vals, nil
}

func ptrSlice(in []model.DirectoryNode) []*model.DirectoryNode {
	out := make([]*model.DirectoryNode, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
}

// ---------- 题目 ----------

func encodeTags(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func decodeTags(s string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil || tags == nil {
		return []string{}
	}
	return tags
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ProblemFilter 题目列表过滤条件。
type ProblemFilter struct {
	Q         string
	Tags      []string
	Type      string
	DirID     *int64
	Recursive bool
	IDs       []int64
}

const problemSummaryCols = `id,type,title,tags_json,time_limit_ms,memory_limit_mib,directory_id,created_at`

func scanProblemSummaries(rows *sql.Rows) ([]model.ProblemSummary, error) {
	defer rows.Close()
	var out []model.ProblemSummary
	for rows.Next() {
		var p model.ProblemSummary
		var tagsJSON string
		if err := rows.Scan(&p.ID, &p.Type, &p.Title, &tagsJSON, &p.TimeLimitMS, &p.MemoryLimitMiB, &p.DirectoryID, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Tags = decodeTags(tagsJSON)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListProblems 按过滤条件列出题目摘要。
func (s *Store) ListProblems(f ProblemFilter) ([]model.ProblemSummary, error) {
	where := []string{"1=1"}
	var args []any
	if f.Q != "" {
		like := "%" + escapeLike(f.Q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR tags_json LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	for _, t := range f.Tags {
		where = append(where, `tags_json LIKE ? ESCAPE '\'`)
		args = append(args, `%"`+escapeLike(t)+`"%`)
	}
	if f.Type != "" {
		where = append(where, `type=?`)
		args = append(args, f.Type)
	}
	if f.DirID != nil {
		ids := []int64{*f.DirID}
		if f.Recursive {
			desc, err := s.descendantIDs(*f.DirID)
			if err != nil {
				return nil, err
			}
			ids = append(ids, desc...)
		}
		ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		where = append(where, `directory_id IN (`+ph+`)`)
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(f.IDs) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.IDs)), ",")
		where = append(where, `id IN (`+ph+`)`)
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	rows, err := s.DB.Query(`SELECT `+problemSummaryCols+` FROM problems WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	return scanProblemSummaries(rows)
}

// CreateProblem 写入题目，返回新 id。
func (s *Store) CreateProblem(p model.Problem) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO problems
		(type,title,tags_json,statement_md,body_json,answer_json,solutions_json,time_limit_ms,memory_limit_mib,directory_id)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		string(p.Type), p.Title, encodeTags(p.Tags), p.StatementMD,
		string(p.BodyJSON), string(p.AnswerJSON), string(p.Solutions),
		p.TimeLimitMS, p.MemoryLimitMiB, p.DirectoryID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetProblem 读取题目完整内容。
func (s *Store) GetProblem(id int64) (*model.Problem, error) {
	p := &model.Problem{}
	var tagsJSON, body, answer, solutions string
	err := s.DB.QueryRow(`SELECT id,type,title,tags_json,statement_md,body_json,answer_json,solutions_json,
		time_limit_ms,memory_limit_mib,directory_id,created_at FROM problems WHERE id=?`, id).
		Scan(&p.ID, &p.Type, &p.Title, &tagsJSON, &p.StatementMD, &body, &answer, &solutions,
			&p.TimeLimitMS, &p.MemoryLimitMiB, &p.DirectoryID, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tags = decodeTags(tagsJSON)
	p.BodyJSON = json.RawMessage(body)
	p.AnswerJSON = json.RawMessage(answer)
	p.Solutions = json.RawMessage(solutions)
	return p, nil
}

// UpdateProblem 全量更新题目（按 id）。
func (s *Store) UpdateProblem(p model.Problem) error {
	res, err := s.DB.Exec(`UPDATE problems SET type=?,title=?,tags_json=?,statement_md=?,body_json=?,
		answer_json=?,solutions_json=?,time_limit_ms=?,memory_limit_mib=?,directory_id=? WHERE id=?`,
		string(p.Type), p.Title, encodeTags(p.Tags), p.StatementMD,
		string(p.BodyJSON), string(p.AnswerJSON), string(p.Solutions),
		p.TimeLimitMS, p.MemoryLimitMiB, p.DirectoryID, p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProblemSolutions 只更新题解数组。
func (s *Store) UpdateProblemSolutions(id int64, solutions json.RawMessage) error {
	res, err := s.DB.Exec(`UPDATE problems SET solutions_json=? WHERE id=?`, string(solutions), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProblem 删除题目并清理训练/练习条目引用。
func (s *Store) DeleteProblem(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM training_items WHERE problem_id=?`,
		`DELETE FROM practice_items WHERE problem_id=?`,
		`DELETE FROM problems WHERE id=?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountProblems 题目总数。
func (s *Store) CountProblems() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM problems`).Scan(&n)
	return n, err
}

// ---------- 标签 ----------

// TagCount 标签及使用数。
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ListTags 汇总全部题目标签，按数量降序、名称升序。
func (s *Store) ListTags() ([]TagCount, error) {
	rows, err := s.DB.Query(`SELECT tags_json FROM problems`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counter := map[string]int{}
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			return nil, err
		}
		for _, t := range decodeTags(tagsJSON) {
			counter[t]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]TagCount, 0, len(counter))
	for tag, c := range counter {
		out = append(out, TagCount{Tag: tag, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}
