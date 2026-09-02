// Package accounts 是主站（OrangeRepo）与刷题服务（Orange quiz）共享的账号权威库：
//
//   - users/sessions 表物理位于 quiz.db（两个服务共享同一数据目录）；
//   - 本包是这些表迁移与全部用户/会话操作的唯一 owner；
//   - 旧版主站的 settings.password_hash 单账号在启动时迁移为 admin 账号（见 CreateAdminFromHash）。
package accounts

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// ErrNotFound 统一的未找到错误。
var ErrNotFound = errors.New("not found")

// ErrConflict 唯一性冲突（用户名已存在等）。
var ErrConflict = errors.New("conflict")

// Role 用户角色。
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleStudent Role = "student"
)

// User 会话上下文中的用户。
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
}

// Student 学生账号管理视图（含错题数，错题数由调用方注入）。
type Student struct {
	ID         int64     `json:"id"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"createdAt"`
	WrongCount int       `json:"wrongCount"`
}

// Store 账号库句柄（线程安全：内部经 *sql.DB）。
type Store struct {
	DB *sql.DB
}

// OpenDB 打开（必要时创建）quiz.db 并执行账号表迁移；返回连接句柄。
// 同一进程内多个服务可各自持有连接（同文件多连接，WAL + busy_timeout 支持）。
func OpenDB(dataDir string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "quiz.db")) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open accounts sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate 幂等创建账号表（users/sessions）。
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('admin','student')),
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("accounts migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

// New 基于已有连接的账号库句柄。
func New(db *sql.DB) *Store { return &Store{DB: db} }

// ---------- 用户 ----------

// ValidateUsername 校验用户名：trim 后 1–32 字符、无控制字符。
func ValidateUsername(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return errors.New("用户名不能为空")
	}
	if len([]rune(u)) > 32 {
		return errors.New("用户名不能超过 32 个字符")
	}
	for _, r := range u {
		if r < 0x20 {
			return errors.New("用户名含非法字符")
		}
	}
	return nil
}

func (s *Store) insertUser(username, passwordHash string, role Role) (int64, error) {
	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return 0, err
	}
	if role != RoleAdmin && role != RoleStudent {
		return 0, errors.New("非法角色")
	}
	res, err := s.DB.Exec(`INSERT INTO users(username,password_hash,role) VALUES(?,?,?)`,
		username, passwordHash, string(role))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrConflict
		}
		return 0, err
	}
	return res.LastInsertId()
}

// CreateUser 创建用户（用户名大小写不敏感唯一；role ∈ admin|student）。
func (s *Store) CreateUser(username, password string, role Role) (int64, error) {
	if password == "" {
		return 0, errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	return s.insertUser(username, string(hash), role)
}

// CreateAdminFromHash 用既有 bcrypt 哈希创建管理员（旧版主站 settings 密码迁移，
// 保证升级后原密码无缝可用）。
func (s *Store) CreateAdminFromHash(username, passwordHash string) (int64, error) {
	if passwordHash == "" {
		return 0, errors.New("密码哈希不能为空")
	}
	return s.insertUser(username, passwordHash, RoleAdmin)
}

// GetUserByUsername 按用户名（大小写不敏感）取用户。
func (s *Store) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id,username,role FROM users WHERE username=? COLLATE NOCASE`, username).
		Scan(&u.ID, &u.Username, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByID 取用户。
func (s *Store) GetUserByID(id int64) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id,username,role FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// HasAdmin 是否存在管理员账号（用于首次引导）。
func (s *Store) HasAdmin() (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n)
	return n > 0, err
}

// ListStudents 学生账号列表（含各自错题数；错题数保持为 0 由调用方补充或直接使用）。
// 注意：wrong_answers 属于刷题服务数据，计数由 quizstore 侧 JOIN 维护；
// 本包返回原始账号信息。
func (s *Store) ListStudents() ([]Student, error) {
	rows, err := s.DB.Query(`SELECT id,username,created_at FROM users WHERE role='student' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Student
	for rows.Next() {
		var st Student
		if err := rows.Scan(&st.ID, &st.Username, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ListAdmins 管理员账号列表（供系统管理页展示与重置密码）。
func (s *Store) ListAdmins() ([]User, error) {
	rows, err := s.DB.Query(`SELECT id,username,role FROM users WHERE role='admin' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteStudent 删除学生账号（级联清理会话与错题记录；仅允许学生角色）。
func (s *Store) DeleteStudent(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM users WHERE id=? AND role='student'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStudentPassword 重置学生密码。
func (s *Store) SetStudentPassword(id int64, password string) error {
	if password == "" {
		return errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET password_hash=? WHERE id=? AND role='student'`, string(hash), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// 重置后该用户全端会话失效（统一账号的密码联动语义）
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	return nil
}

// SetUserPassword 管理员重置任意用户（含管理员）密码，并清除其全部会话。
func (s *Store) SetUserPassword(id int64, password string) error {
	if password == "" {
		return errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(hash), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	return nil
}

// CheckPassword 校验用户名密码（登录用）。
func (s *Store) CheckPassword(username, password string) (*User, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	var hash string
	if err := s.DB.QueryRow(`SELECT password_hash FROM users WHERE id=?`, u.ID).Scan(&hash); err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, errors.New("wrong password")
	}
	return u, nil
}

// SetPassword 修改当前用户密码并清除其全部会话（统一账号：两端强制重新登录）。
func (s *Store) SetPassword(userID int64, newPassword string) error {
	if newPassword == "" {
		return errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(hash), userID); err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

// ---------- 会话 ----------

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession 为用户创建会话，返回 token。
func (s *Store) CreateSession(userID int64) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := s.DB.Exec(`INSERT INTO sessions(token,user_id) VALUES(?,?)`, token, userID); err != nil {
		return "", err
	}
	return token, nil
}

// GetUserByToken 按会话 token 取用户；token 失效（用户被删）时自动清理。
func (s *Store) GetUserByToken(token string) (*User, bool) {
	if token == "" {
		return nil, false
	}
	u := &User{}
	err := s.DB.QueryRow(`SELECT u.id,u.username,u.role FROM sessions se
		JOIN users u ON u.id=se.user_id WHERE se.token=?`, token).
		Scan(&u.ID, &u.Username, &u.Role)
	if err != nil {
		_, _ = s.DB.Exec(`DELETE FROM sessions WHERE token=?`, token)
		return nil, false
	}
	return u, true
}

// DeleteSession 删除会话。
func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

// RotateSession 删除旧会话并创建新会话（改密码后轮换本端）。
func (s *Store) RotateSession(oldToken string, userID int64) (string, error) {
	_ = s.DeleteSession(oldToken)
	return s.CreateSession(userID)
}