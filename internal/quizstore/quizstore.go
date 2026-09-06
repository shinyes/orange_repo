// Package quizstore 封装刷题服务存储：唯一数据库 orangeoj.db（单库，与主站共用）。
//
//   - 判题 submissions/judge_jobs/progress + 空间作答 space_training_attempts/
//     space_practice_submissions/student_solved
//   - 共享账号表（users/sessions，表结构与账号/会话操作的唯一 owner 是 internal/accounts，
//     本包经 Accounts 字段访问）
//   - 题库/域/空间结构表（problems/domains/spaces/space_trainings…由 internal/store 的
//     MigrateSchema 保证；本包 Repo 指向同一连接读取，原跨文件只读 RepoReader 已随单库化移除）
package quizstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"orangeoj/internal/accounts"
	"orangeoj/internal/store"
)

// ErrNotFound 统一的未找到错误。
var ErrNotFound = errors.New("not found")

// Store 刷题服务存储：orangeoj.db（单库读写）。Accounts 为同一连接的账号库；Repo 为同一连接的题库读句柄。
type Store struct {
	DB       *sql.DB
	Repo     *RepoReader
	Accounts *accounts.Store
}

// Open 打开（必要时创建）数据目录与唯一数据库 orangeoj.db 并迁移全部表。
// 单库模式：题库/域/空间结构表与账号/判题/作答表同处一个文件；本服务读写同一库，
// Repo 指向同一连接（原只读 RepoReader 已随单库化移除）。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create quiz data dir: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "orangeoj.db")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open quiz sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Accounts: accounts.New(db), Repo: &RepoReader{DB: db}}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// 题库/域/空间结构表（与主站同库；若本服务先于主站启动也建齐全表）
	if err := (&store.Store{DB: db}).MigrateSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) migrate() error {
	// users/sessions 由 internal/accounts 负责（幂等），此处先行保证
	if err := accounts.Migrate(s.DB); err != nil {
		return err
	}
	stmts := []string{
		// ---------- OrangeOJ 判题（v1.12：submissions/judge_jobs/progress，结构照搬上游 db.go） ----------
		`CREATE TABLE IF NOT EXISTS submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			question_type TEXT NOT NULL,
			language TEXT NOT NULL DEFAULT '',
			source_code TEXT NOT NULL DEFAULT '',
			input_data TEXT NOT NULL DEFAULT '',
			submit_type TEXT NOT NULL,
			status TEXT NOT NULL,
			verdict TEXT NOT NULL DEFAULT 'PENDING',
			time_ms INTEGER NOT NULL DEFAULT 0,
			memory_kib INTEGER NOT NULL DEFAULT 0,
			score INTEGER NOT NULL DEFAULT 0,
			stdout TEXT NOT NULL DEFAULT '',
			stderr TEXT NOT NULL DEFAULT '',
			case_details_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS judge_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			finished_at DATETIME,
			worker_token TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_judge_jobs_status_priority ON judge_jobs(status, priority DESC, id ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_submissions_user_problem ON submissions(user_id, problem_id, id DESC);`,
		`CREATE TABLE IF NOT EXISTS user_problem_progress (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			best_verdict TEXT NOT NULL,
			best_score INTEGER NOT NULL DEFAULT 0,
			last_submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(user_id, problem_id)
		);`,
		// ---------- 空间训练/练习学生作答（主库仅存空间内容结构；作答与 users 同库） ----------
		// training_id/practice_id 指主库空间训练/练习 id，无跨库外键；
		// user_id 与 users 同库可级联删除。
		`CREATE TABLE IF NOT EXISTS space_training_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			training_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 1,
			solved INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(training_id, user_id, problem_id)
		);`,
		`CREATE TABLE IF NOT EXISTS space_practice_submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			practice_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			answers_json TEXT NOT NULL DEFAULT '[]',
			objective_correct INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS student_solved (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			problem_uuid TEXT NOT NULL,
			solved_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(user_id, problem_uuid)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("quiz migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

// decodeStrings 解析各 *_json 列（tags 等）；空/非法返回空列表。
// 供 problems.go/repo_oj.go/repo_space.go 解码主库标签复用。
func decodeStrings(s string) []string {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}