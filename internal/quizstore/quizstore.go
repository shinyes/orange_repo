// Package quizstore 封装刷题服务的两级存储：
//
//   - quiz.db：刷题服务自有数据（settings 表保留 + 判题 submissions/judge_jobs/progress
//     + 空间作答 space_training_attempts/space_practice_submissions/student_solved）
//     + 共享账号表（users/sessions，表结构与账号/会话操作的唯一 owner 是 internal/accounts，
//     本包经 Accounts 字段访问）；
//   - orangeoj.db：只读访问主站题库（见 repo_oj.go / repo_space.go 的 RepoReader）。
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
)

// ErrNotFound 统一的未找到错误。
var ErrNotFound = errors.New("not found")

// Store 刷题服务存储：quiz.db 写 + 主库只读。Accounts 是共享账号库（同一 quiz.db 连接）。
type Store struct {
	DB       *sql.DB
	Repo     *RepoReader
	Accounts *accounts.Store
}

// Open 打开（必要时创建）数据目录与 quiz.db 并迁移，同时以只读方式打开主库题库。
func Open(dataDir, repoPath string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create quiz data dir: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "quiz.db")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open quiz sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Accounts: accounts.New(db)}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	repo, err := OpenRepoReader(repoPath)
	if err != nil {
		db.Close()
		return nil, err
	}
	s.Repo = repo
	return s, nil
}

func (s *Store) Close() error {
	if s.Repo != nil {
		_ = s.Repo.DB.Close()
	}
	return s.DB.Close()
}

func (s *Store) migrate() error {
	// users/sessions 由 internal/accounts 负责（幂等），此处先行保证
	if err := accounts.Migrate(s.DB); err != nil {
		return err
	}
	stmts := []string{
		// settings 保留（无害小表；旧 round_size 键不再使用，后续无写入方）。
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
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