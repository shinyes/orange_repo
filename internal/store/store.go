// Package store 封装 SQLite 持久化：迁移、设置、题目与标签（斜杠嵌套层级）。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"orangeoj/internal/model"
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
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "orangeoj.db")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4) // WAL 多读者；写者由 busy_timeout 串行
	s := &Store{DB: db, DataDir: dataDir}
	if err := s.migrateWithRetry(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// MigrateSchema 对任意连接执行全量建表/迁移（题库/域/空间结构表）。
// 单库模式下 quizstore 等与主站共用同一 orangeoj.db 文件，可复用保证题库侧表齐全。
func (s *Store) MigrateSchema() error { return s.migrateWithRetry() }

// migrateWithRetry 迁移带 SQLITE_BUSY 重试：双进程同库并发首启时 DDL 会撞写锁，
// 重试若干次让先到者完成后再执行（每次重试重新探测，天然幂等）。
func (s *Store) migrateWithRetry() error {
	const attempts = 6
	var lastErr error
	for i := 0; i < attempts; i++ {
		lastErr = s.migrate()
		if lastErr == nil {
			return nil
		}
		if !strings.Contains(lastErr.Error(), "database is locked") && !strings.Contains(lastErr.Error(), "SQLITE_BUSY") {
			return lastErr
		}
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
	}
	return fmt.Errorf("migrate failed after %d attempts: %w", attempts, lastErr)
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
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
			starter_cpp TEXT NOT NULL DEFAULT '',
			starter_py TEXT NOT NULL DEFAULT '',
			time_limit_ms INTEGER NOT NULL DEFAULT 1000,
			memory_limit_mib INTEGER NOT NULL DEFAULT 256,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS trainings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL DEFAULT '',
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
			uuid TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS practice_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			practice_id INTEGER NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL REFERENCES problems(id),
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS booklet_directories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES booklet_directories(id) ON DELETE SET NULL,
			order_no INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	if err := s.ensureColumn("trainings", "folder_id", `folder_id INTEGER REFERENCES booklet_directories(id) ON DELETE SET NULL`); err != nil {
		return err
	}
	if err := s.ensureColumn("practices", "folder_id", `folder_id INTEGER REFERENCES booklet_directories(id) ON DELETE SET NULL`); err != nil {
		return err
	}
	// v1.7.0：OrangeOJ 练习无分值语义，退役 score 列
	if err := s.dropColumn("practice_items", "score"); err != nil {
		return err
	}
	// legacy（v1.0 directories 时代）迁移会重建 problems 表，须先执行再补 uuid
	if err := s.migrateLegacyDirectories(); err != nil {
		return err
	}
	// 题目 UUIDv7 稳定标识（跨库去重/引用）；存量行补 uuid
	if err := s.ensureColumn("problems", "uuid", `uuid TEXT`); err != nil {
		return err
	}
	// 编程题起始代码模板（v2：starter_cpp/starter_py；存量列空=前端通用模板）
	if err := s.ensureColumn("problems", "starter_cpp", `starter_cpp TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn("problems", "starter_py", `starter_py TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.backfillProblemUUIDs(); err != nil {
		return err
	}
	// 域/空间（OJ 重构：域隔离题目，空间隔离训练/练习/作答）
	if err := s.migrateDomains(); err != nil {
		return err
	}
	// 空间内容（空间训练/练习/刷题结构；学生作答见 quizstore）
	if err := s.migrateSpaceContent(); err != nil {
		return err
	}
	// 训练/练习/刷题 uuidv7（表建齐后补列/回填——空间表在 migrateSpaceContent 才建）
	for _, tbl := range []string{"trainings", "practices", "space_trainings", "space_practices", "space_quizzes"} {
		if err := s.ensureColumn(tbl, "uuid", `uuid TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if err := s.backfillUUIDs(tbl); err != nil {
			return err
		}
	}
	// 题目归属域：存量题归入默认域。
	// 注意：不挂 ON DELETE CASCADE——删域必须先显式删除域内题目（DeleteDomainProblems），
	// 防止“裸删域”静默级联删题（或题目被仓库题册引用时因 FK 报错）的行为歧义
	if err := s.ensureColumn("problems", "domain_id", `domain_id INTEGER REFERENCES domains(id)`); err != nil {
		return err
	}
	return s.backfillProblemDomain()
}

// EnsureDefaultDomain 返回默认域 id（不存在则创建；并发容错：他方先建则回读）。
func (s *Store) EnsureDefaultDomain() (int64, error) {
	var domainID int64
	err := s.DB.QueryRow(`SELECT id FROM domains WHERE name=? ORDER BY id LIMIT 1`, DefaultDomainName).Scan(&domainID)
	if err == nil {
		return domainID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT OR IGNORE INTO domains(name) VALUES(?)`, DefaultDomainName)
	if err != nil {
		return 0, err
	}
	domainID, _ = res.LastInsertId()
	if domainID == 0 {
		if err := s.DB.QueryRow(`SELECT id FROM domains WHERE name=?`, DefaultDomainName).Scan(&domainID); err != nil {
			return 0, err
		}
	}
	return domainID, nil
}

// DefaultDomainName 存量题库自动归属的默认域名。
const DefaultDomainName = "默认域"

// backfillProblemDomain 为无 domain_id 的存量题目补默认域（自动建域，幂等）。
func (s *Store) backfillProblemDomain() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE domain_id IS NULL`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	// 找/建默认域
	var domainID int64
	err := s.DB.QueryRow(`SELECT id FROM domains WHERE name=?`, DefaultDomainName).Scan(&domainID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		res, err := s.DB.Exec(`INSERT OR IGNORE INTO domains(name) VALUES(?)`, DefaultDomainName)
		if err != nil {
			return err
		}
		domainID, _ = res.LastInsertId()
		if domainID == 0 {
			// 并发迁移：他方刚插入默认域 → 回读其 id
			if err := s.DB.QueryRow(`SELECT id FROM domains WHERE name=?`, DefaultDomainName).Scan(&domainID); err != nil {
				return err
			}
		}
	}
	if _, err := s.DB.Exec(`UPDATE problems SET domain_id=? WHERE domain_id IS NULL`, domainID); err != nil {
		return err
	}
	return nil
}

// migrateDomains 建域/空间表（幂等）。
func (s *Store) migrateDomains() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS spaces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS space_members (
			space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(space_id, user_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_spaces_domain ON spaces(domain_id);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate domains failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

// backfillProblemUUIDs 为 uuid 为空的存量题目生成 UUIDv7（幂等）。
func (s *Store) backfillProblemUUIDs() error {
	rows, err := s.DB.Query(`SELECT id FROM problems WHERE uuid IS NULL OR uuid=''`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		u, err := NewUUIDv7()
		if err != nil {
			return err
		}
		if _, err := s.DB.Exec(`UPDATE problems SET uuid=? WHERE id=?`, u, id); err != nil {
			return err
		}
	}
	return nil
}

// backfillUUIDs 为指定表无 uuid 的行生成 UUIDv7（训练/练习/刷题等通用）。
func (s *Store) backfillUUIDs(table string) error {
	rows, err := s.DB.Query(`SELECT id FROM ` + table + ` WHERE uuid IS NULL OR uuid=''`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		u, err := NewUUIDv7()
		if err != nil {
			return err
		}
		if _, err := s.DB.Exec(`UPDATE `+table+` SET uuid=? WHERE id=?`, u, id); err != nil {
			return err
		}
	}
	return nil
}

// ensureColumn 幂等补列：仅当目标表缺少该列时执行 ALTER TABLE ADD COLUMN。
// 双进程同库并发迁移时，另一方可能已补列——ALTER 报 duplicate column 视为成功。
func (s *Store) ensureColumn(table, column, ddl string) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&n); err != nil {
		return fmt.Errorf("ensure column %s.%s: %w", table, column, err)
	}
	if n > 0 {
		return nil
	}
	if _, err := s.DB.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + ddl); err != nil {
		if strings.Contains(err.Error(), "duplicate column") {
			return nil // 并发迁移：他方已加列
		}
		return fmt.Errorf("ensure column %s.%s: %w", table, column, err)
	}
	return nil
}

// dropColumn 幂等删列：仅当目标表存在该列时执行 ALTER TABLE DROP COLUMN。
// 双进程同库并发迁移时，另一方可能已删列——ALTER 报 no such column 视为成功。
func (s *Store) dropColumn(table, column string) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&n); err != nil {
		return fmt.Errorf("drop column %s.%s: %w", table, column, err)
	}
	if n == 0 {
		return nil
	}
	if _, err := s.DB.Exec(`ALTER TABLE ` + table + ` DROP COLUMN ` + column); err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil // 并发迁移：他方已删列
		}
		return fmt.Errorf("drop column %s.%s: %w", table, column, err)
	}
	return nil
}

// migrateLegacyDirectories 一次性迁移 v1.0 旧库：退役目录结构（用户决策：目录数据丢弃）。
//
// 旧 problems 表带指向 directories 的外键列，DROP 前必须在同一连接上临时关闭 foreign_keys
// （training_items 等子表引用 problems，否则 DROP 会因级联检查失败）。
func (s *Store) migrateLegacyDirectories() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='directories'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	ctx := context.Background()
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=off`); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE problems_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			statement_md TEXT NOT NULL DEFAULT '',
			body_json TEXT NOT NULL DEFAULT '{}',
			answer_json TEXT NOT NULL DEFAULT '{}',
			solutions_json TEXT NOT NULL DEFAULT '[]',
			time_limit_ms INTEGER NOT NULL DEFAULT 1000,
			memory_limit_mib INTEGER NOT NULL DEFAULT 256,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO problems_new(id,type,title,tags_json,statement_md,body_json,answer_json,solutions_json,time_limit_ms,memory_limit_mib,created_at)
		 SELECT id,type,title,tags_json,statement_md,body_json,answer_json,solutions_json,time_limit_ms,memory_limit_mib,created_at FROM problems`,
		`DROP TABLE problems`,
		`ALTER TABLE problems_new RENAME TO problems`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w; stmt: %s", err, stmt)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS directories`); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=on`)
	return err
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

// NewUUIDv7 生成 UUIDv7（google/uuid 支持；失败极少，透传错误）。
func NewUUIDv7() (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// EnsureProblemUUID 若 p.UUID 为空则生成 UUIDv7 并回填（导入/创建共用）。
func (s *Store) EnsureProblemUUID(p *model.Problem) error {
	if p.UUID == "" {
		u, err := NewUUIDv7()
		if err != nil {
			return err
		}
		p.UUID = u
	}
	return nil
}

// ProblemUUIDExists 该 uuid 是否已存在（导入去重）。
func (s *Store) ProblemUUIDExists(u string) (bool, error) {
	if u == "" {
		return false, nil
	}
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE uuid=?`, u).Scan(&n)
	return n > 0, err
}

// ProblemIDByUUID 按 uuid 取题目 id（不存在返回 ErrNotFound）。
func (s *Store) ProblemIDByUUID(u string) (int64, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM problems WHERE uuid=?`, u).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ProblemIDByUUIDInDomain 按 uuid+归属域取题目 id：同 uuid 跨域视为不存在（各域独立副本），
// 避免导入去重把 A 域题目引用进 B 域空间（跨域污染）。
func (s *Store) ProblemIDByUUIDInDomain(u string, domainID *int64) (int64, error) {
	if u == "" || domainID == nil {
		return 0, ErrNotFound
	}
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM problems WHERE uuid=? AND domain_id=?`, u, *domainID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
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
	Q    string
	Tags []string
	Type string
	IDs  []int64
	// DomainID 域过滤（nil=不按域过滤——仅系统全局视图使用；普通调用必须带）
	DomainID *int64
}

const problemSummaryCols = `id,uuid,domain_id,type,title,tags_json,time_limit_ms,memory_limit_mib,created_at`

func scanProblemSummaries(rows *sql.Rows) ([]model.ProblemSummary, error) {
	defer rows.Close()
	out := []model.ProblemSummary{}
	for rows.Next() {
		var p model.ProblemSummary
		var tagsJSON string
		var domain sql.NullInt64
		if err := rows.Scan(&p.ID, &p.UUID, &domain, &p.Type, &p.Title, &tagsJSON, &p.TimeLimitMS, &p.MemoryLimitMiB, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Tags = decodeTags(tagsJSON)
		if domain.Valid {
			id := domain.Int64
			p.DomainID = &id
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// problemWhere 构造题目过滤 SQL（q/类型/ids；标签条件在 Go 侧按前缀规则过滤）。
func (s *Store) problemWhere(f ProblemFilter) (string, []any) {
	where := []string{"1=1"}
	var args []any
	if f.Q != "" {
		like := "%" + escapeLike(f.Q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR tags_json LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	if f.Type != "" {
		where = append(where, `type=?`)
		args = append(args, f.Type)
	}
	if f.DomainID != nil {
		where = append(where, `domain_id=?`)
		args = append(args, *f.DomainID)
	}
	if len(f.IDs) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(f.IDs)), ",")
		where = append(where, `id IN (`+ph+`)`)
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	return strings.Join(where, " AND "), args
}

// NoneTag 哨兵值：题目没有任何标签时匹配的虚拟标签（前端「无标签」伪节点）。
const NoneTag = "__none__"

// tagSetMatches 报告 tags 中是否存在 sel 本身或其前缀子孙（t==sel || HasPrefix(t, sel+"/")）。
func tagSetMatches(tags []string, sel string) bool {
	prefix := sel + "/"
	for _, t := range tags {
		if t == sel || strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

// TagMatchesSelected 匹配选中标签集的规则：对选中集 S 中每个 s，题目至少有一个标签命中
// （t==s 或 t 为 s 的前缀子孙 t==s+"/"+…）；特判 NoneTag：命中标签数组为空的题目。
// 前缀 AND 规则的唯一权威实现，供主站列表/分面与刷题服务只读复用。
func TagMatchesSelected(tags []string, selected []string) bool {
	for _, sel := range selected {
		if sel == NoneTag {
			if len(tags) != 0 {
				return false
			}
		} else if !tagSetMatches(tags, sel) {
			return false
		}
	}
	return true
}

// ListProblems 按过滤条件列出题目摘要（标签条件走前缀 AND 规则）。
func (s *Store) ListProblems(f ProblemFilter) ([]model.ProblemSummary, error) {
	where, args := s.problemWhere(f)
	rows, err := s.DB.Query(`SELECT `+problemSummaryCols+` FROM problems WHERE `+where+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	list, err := scanProblemSummaries(rows)
	if err != nil {
		return nil, err
	}
	if len(f.Tags) == 0 {
		return list, nil
	}
	out := make([]model.ProblemSummary, 0, len(list))
	for _, p := range list {
		if TagMatchesSelected(p.Tags, f.Tags) {
			out = append(out, p)
		}
	}
	return out, nil
}

// CreateProblem 写入题目，返回新 id。p.UUID 为空时自动生成 UUIDv7；p.DomainID 必填（新建须归域）。
func (s *Store) CreateProblem(p model.Problem) (int64, error) {
	if err := s.EnsureProblemUUID(&p); err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO problems
		(uuid,domain_id,type,title,tags_json,statement_md,body_json,answer_json,solutions_json,starter_cpp,starter_py,time_limit_ms,memory_limit_mib)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.UUID, nullInt64(p.DomainID), string(p.Type), p.Title, encodeTags(p.Tags), p.StatementMD,
		string(p.BodyJSON), string(p.AnswerJSON), string(p.Solutions),
		p.StarterCpp, p.StarterPy,
		p.TimeLimitMS, p.MemoryLimitMiB)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetProblem 读取题目完整内容。
func (s *Store) GetProblem(id int64) (*model.Problem, error) {
	p := &model.Problem{}
	var tagsJSON, body, answer, solutions string
	var domain sql.NullInt64
	err := s.DB.QueryRow(`SELECT id,uuid,domain_id,type,title,tags_json,statement_md,body_json,answer_json,solutions_json,
		starter_cpp,starter_py,time_limit_ms,memory_limit_mib,created_at FROM problems WHERE id=?`, id).
		Scan(&p.ID, &p.UUID, &domain, &p.Type, &p.Title, &tagsJSON, &p.StatementMD, &body, &answer, &solutions,
			&p.StarterCpp, &p.StarterPy,
			&p.TimeLimitMS, &p.MemoryLimitMiB, &p.CreatedAt)
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
	if domain.Valid {
		id := domain.Int64
		p.DomainID = &id
	}
	return p, nil
}

// UpdateProblem 全量更新题目（按 id）。
func (s *Store) UpdateProblem(p model.Problem) error {
	res, err := s.DB.Exec(`UPDATE problems SET type=?,title=?,tags_json=?,statement_md=?,body_json=?,
		answer_json=?,solutions_json=?,starter_cpp=?,starter_py=?,time_limit_ms=?,memory_limit_mib=? WHERE id=?`,
		string(p.Type), p.Title, encodeTags(p.Tags), p.StatementMD,
		string(p.BodyJSON), string(p.AnswerJSON), string(p.Solutions),
		p.StarterCpp, p.StarterPy,
		p.TimeLimitMS, p.MemoryLimitMiB, p.ID)
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

// DeleteProblem 删除题目并清理训练/练习条目与空间条目引用。
func (s *Store) DeleteProblem(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM training_items WHERE problem_id=?`,
		`DELETE FROM practice_items WHERE problem_id=?`,
		`DELETE FROM space_training_items WHERE problem_id=?`, // 空间条目无 FK，防悬挂
		`DELETE FROM space_practice_items WHERE problem_id=?`,
		`DELETE FROM problems WHERE id=?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteOrphanProblems 在已有事务内删除不再被任何训练/练习条目引用的题目。
// 用于删除题册时的级联：仅被该题册引用的题目一并删除，被多题册引用的保留。
func deleteOrphanProblems(tx *sql.Tx, pids []int64) error {
	for _, pid := range pids {
		var n int
		err := tx.QueryRow(`SELECT
			(SELECT COUNT(*) FROM training_items ti JOIN training_chapters tc ON ti.chapter_id=tc.id WHERE ti.problem_id=?)
			+
			(SELECT COUNT(*) FROM practice_items WHERE problem_id=?)`, pid, pid).Scan(&n)
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err := tx.Exec(`DELETE FROM problems WHERE id=?`, pid); err != nil {
				return err
			}
		}
	}
	return nil
}

// CountProblems 题目总数。
func (s *Store) CountProblems() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM problems`).Scan(&n)
	return n, err
}

// ---------- 标签 ----------

// TagCount 标签及动态命中数。
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ValidateTagPath 校验并规范化标签路径：trim 后非空、首尾不得为 /、不得有空层级。
func ValidateTagPath(s string) (string, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", errors.New("标签不能为空")
	}
	if strings.HasPrefix(t, "/") || strings.HasSuffix(t, "/") {
		return "", fmt.Errorf("标签 %q 不能以 / 开头或结尾", t)
	}
	for _, seg := range strings.Split(t, "/") {
		if strings.TrimSpace(seg) == "" {
			return "", fmt.Errorf("标签 %q 含空层级", t)
		}
	}
	return t, nil
}

// RenameTag 重命名标签：精确匹配重写为新值，from 前缀子树整体搬家；与已有标签重复时保序去重合并。
// 返回受影响的题目数。
func (s *Store) RenameTag(from, to string) (int64, error) {
	from, err := ValidateTagPath(from)
	if err != nil {
		return 0, fmt.Errorf("from: %w", err)
	}
	to, err = ValidateTagPath(to)
	if err != nil {
		return 0, fmt.Errorf("to: %w", err)
	}
	if from == to {
		return 0, nil
	}
	return s.rewriteTags(func(t string) ([]string, bool) {
		switch {
		case t == from:
			return []string{to}, true
		case strings.HasPrefix(t, from+"/"):
			return []string{to + t[len(from):]}, true
		}
		return nil, false
	})
}

// DeleteTag 删除标签及其全部前缀子孙，从所有题目上移除。返回受影响的题目数。
func (s *Store) DeleteTag(tag string) (int64, error) {
	tag, err := ValidateTagPath(tag)
	if err != nil {
		return 0, err
	}
	return s.rewriteTags(func(t string) ([]string, bool) {
		if t == tag || strings.HasPrefix(t, tag+"/") {
			return nil, true
		}
		return nil, false
	})
}

// rewriteTags 对每道题的标签列表应用 rewrite（返回替换列表 + 是否命中），
// 命中的题目在事务内保序去重后更新。返回发生变化的题目数。
func (s *Store) rewriteTags(rewrite func(string) ([]string, bool)) (int64, error) {
	rows, err := s.DB.Query(`SELECT id, tags_json FROM problems`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type update struct {
		id   int64
		tags []string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var tagsJSON string
		if err := rows.Scan(&id, &tagsJSON); err != nil {
			return 0, err
		}
		tags := decodeTags(tagsJSON)
		next := make([]string, 0, len(tags))
		mutated := false
		for _, t := range tags {
			if rep, hit := rewrite(t); hit {
				mutated = true
				next = append(next, rep...)
				continue
			}
			next = append(next, t)
		}
		if mutated {
			seen := map[string]bool{}
			dedup := make([]string, 0, len(next))
			for _, t := range next {
				if !seen[t] {
					seen[t] = true
					dedup = append(dedup, t)
				}
			}
			updates = append(updates, update{id: id, tags: dedup})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(updates) == 0 {
		return 0, nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, u := range updates {
		if _, err := tx.Exec(`UPDATE problems SET tags_json=? WHERE id=?`, encodeTags(u.tags), u.id); err != nil {
			return 0, err
		}
	}
	return int64(len(updates)), tx.Commit()
}

// ListTagFacets 标签计数（每个标签显示它自己实际命中的题目数）：
//
//   - 基底过滤 = f 去掉标签条件后的全部条件（q/类型等）；
//   - 候选节点 = 基底命中题目的字面标签 ∪ 其虚拟祖先前缀 ∪ 选中集 ∪ __none__；
//   - 对候选 T：count = 基底命中题目中、按前缀规则命中标签 T 的题目数
//     （T 已选中与否不影响其计数；T=__none__ 时计无任何标签的题目数）；
//   - total = 满足完整选中集（AND + 前缀规则）的题目数，与题目列表一致。
//
// 空过滤时退化为全局计数。
func (s *Store) ListTagFacets(f ProblemFilter) ([]TagCount, int, error) {
	where, args := s.problemWhere(f)
	rows, err := s.DB.Query(`SELECT tags_json FROM problems WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	selected := f.Tags
	candidates := map[string]bool{}
	var tagLists [][]string
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			return nil, 0, err
		}
		tags := decodeTags(tagsJSON)
		tagLists = append(tagLists, tags)
		for _, t := range tags {
			candidates[t] = true
			for i := 0; i < len(t); i++ { // 虚拟祖先：a/b/c → a, a/b
				if t[i] == '/' {
					candidates[t[:i]] = true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, t := range selected {
		candidates[t] = true
	}
	candidates[NoneTag] = true // 始终显示「无标签」伪节点

	counts := make(map[string]int, len(candidates))
	total := 0
	// 前缀命中语义：题目命中候选 C ⇔ 题目某标签 == C 或 C 是该标签的祖先。
	// 因此单题的“命中候选集合” = 其全部标签 ∪ 各标签的全部祖先——直接桶计数，
	// 无需对每个候选反查（原实现 O(题目数×候选数)，此处降为 O(题目数×标签深度)）。
	for _, tags := range tagLists {
		if TagMatchesSelected(tags, selected) {
			total++
		}
		seen := make(map[string]bool, len(tags)*2)
		for _, t := range tags {
			// 标签自身与全部祖先（含完整串，避免重复计数）
			for {
				if seen[t] || !candidates[t] {
					break
				}
				seen[t] = true
				counts[t]++
				idx := strings.LastIndexByte(t, '/')
				if idx < 0 {
					break
				}
				t = t[:idx]
			}
		}
		if len(tags) == 0 && candidates[NoneTag] {
			counts[NoneTag]++
		}
	}

	out := make([]TagCount, 0, len(candidates))
	for tag := range candidates {
		out = append(out, TagCount{Tag: tag, Count: counts[tag]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, total, nil
}
