// Package accounts 是主站（OrangeRepo）与刷题服务（Orange quiz）共享的账号权威库：
//
//   - users/sessions 表物理位于 quiz.db（两个服务共享同一数据目录）；
//   - 本包是这些表迁移与全部用户/会话操作的唯一 owner；
//   - 旧版主站的 settings.password_hash 单账号在启动时迁移为 admin 账号（见 CreateAdminFromHash）。
package accounts

import (
	"context"
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
	// RoleGlobalAdmin 系统管理员：可管理全部域（建域/删域/设域管理员/进任意域仓库）。
	RoleGlobalAdmin Role = "global_admin"
	// RoleDomainAdmin 域管理员：管理其 domain_id 所属域的全部空间与仓库。
	RoleDomainAdmin Role = "domain_admin"
	// RoleMember 空间成员（学生）：加入空间后做题。
	RoleMember Role = "member"

	// RoleAdmin / RoleStudent 为旧角色名的兼容别名（读取旧库数据时二者等价映射，
	// 新代码不应再写入这两个值；写入时统一由 migrateRole 映射为新值）。
	RoleAdmin   Role = roleLegacyAdmin
	RoleStudent Role = roleLegacyStudent

	roleLegacyAdmin   Role = "admin"
	roleLegacyStudent Role = "student"
)

// migrateRole 兼容旧角色值：admin→global_admin、student→member。
func migrateRole(r Role) Role {
	switch r {
	case roleLegacyAdmin:
		return RoleGlobalAdmin
	case roleLegacyStudent:
		return RoleMember
	}
	return r
}

// ValidNew 新角色是否合法。
func (r Role) ValidNew() bool {
	switch r {
	case RoleGlobalAdmin, RoleDomainAdmin, RoleMember:
		return true
	}
	return false
}

// User 会话上下文中的用户。
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
	// DomainID 域管理员归属域（仅 RoleDomainAdmin 有意义；JSON 便于前端判断域切换）
	DomainID *int64 `json:"domainId,omitempty"`
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

// Migrate 幂等创建账号表（users/sessions）并迁移角色模型：
//   - users 加 domain_id 列（域管理员归属域）
//   - 旧角色 CHECK('admin','student') 升级为 ('global_admin','domain_admin','member')，
//     存量 admin→global_admin、student→member（重建表拷贝）
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	// users 表须先建（sessions 依赖），单独处理带迁移
	usersDDL := `CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('global_admin','domain_admin','member')),
			domain_id INTEGER,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`
	if _, err := db.Exec(usersDDL); err != nil {
		return fmt.Errorf("accounts migrate users: %w", err)
	}
	if err := migrateUsersSchema(db); err != nil {
		return err
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("accounts migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

// migrateUsersSchema 检测旧版 users（旧 CHECK 或缺 domain_id）并重建迁移（幂等）。
func migrateUsersSchema(db *sql.DB) error {
	needRebuild := false
	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&ddl); err != nil {
		return err
	}
	if !strings.Contains(ddl, "global_admin") {
		needRebuild = true // 旧 CHECK(role IN ('admin','student'))
	}
	// domain_id 列缺失也需重建
	if !needRebuild {
		var n int
		if err := db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('users') WHERE name='domain_id'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			needRebuild = true
		}
	}
	if !needRebuild {
		return nil
	}
	ctx := context.Background()
	conn, err := db.Conn(ctx)
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
	steps := []string{
		`CREATE TABLE users_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('global_admin','domain_admin','member')),
			domain_id INTEGER,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO users_new(id,username,password_hash,role,domain_id,created_at)
		 SELECT id,username,password_hash,
		        CASE role WHEN 'admin' THEN 'global_admin' WHEN 'student' THEN 'member' ELSE role END,
		        NULL, created_at FROM users`,
		`DROP TABLE users`,
		`ALTER TABLE users_new RENAME TO users`,
	}
	for _, stmt := range steps {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w; stmt: %s", err, stmt)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=on`)
	return err
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
	role = migrateRole(role)
	if !role.ValidNew() {
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

// CreateUser 创建用户（用户名大小写不敏感唯一；role ∈ global_admin|domain_admin|member，
// 旧值 admin/student 自动映射）。可选 domainID（域管理员归属域）。
func (s *Store) CreateUser(username, password string, role Role, domainID ...int64) (int64, error) {
	if password == "" {
		return 0, errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	role = migrateRole(role)
	if !role.ValidNew() {
		return 0, errors.New("非法角色")
	}
	var domain any
	if role == RoleDomainAdmin && len(domainID) > 0 {
		domain = domainID[0]
	}
	res, err := s.DB.Exec(`INSERT INTO users(username,password_hash,role,domain_id) VALUES(?,?,?,?)`,
		username, string(hash), string(role), domain)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrConflict
		}
		return 0, err
	}
	return res.LastInsertId()
}

// CreateAdminFromHash 用既有 bcrypt 哈希创建系统管理员（旧版主站 settings 密码迁移，
// 保证升级后原密码无缝可用）。
func (s *Store) CreateAdminFromHash(username, passwordHash string) (int64, error) {
	if passwordHash == "" {
		return 0, errors.New("密码哈希不能为空")
	}
	return s.insertUser(username, passwordHash, RoleGlobalAdmin)
}

// GetUserByUsername 按用户名（大小写不敏感）取用户。
func (s *Store) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(`SELECT id,username,role,domain_id FROM users WHERE username=? COLLATE NOCASE`, username)
}

// GetUserByID 取用户。
func (s *Store) GetUserByID(id int64) (*User, error) {
	return s.scanUser(`SELECT id,username,role,domain_id FROM users WHERE id=?`, id)
}

func (s *Store) scanUser(query string, args ...any) (*User, error) {
	u := &User{}
	var domain sql.NullInt64
	err := s.DB.QueryRow(query, args...).Scan(&u.ID, &u.Username, &u.Role, &domain)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Role = migrateRole(u.Role)
	if domain.Valid {
		id := domain.Int64
		u.DomainID = &id
	}
	return u, nil
}

// HasAdmin 是否存在系统管理员账号（用于首次引导）。
func (s *Store) HasAdmin() (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role IN ('global_admin','admin')`).Scan(&n)
	return n > 0, err
}

// ListStudents 空间成员账号列表（role=member；含各自错题数——错题数属刷题服务数据，
// 由调用方补充或保持 0）。
func (s *Store) ListStudents() ([]Student, error) {
	rows, err := s.DB.Query(`SELECT id,username,created_at FROM users WHERE role IN ('member','student') ORDER BY id`)
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

// ListAdmins 系统管理员 + 域管理员账号列表（含 domain_id，供系统管理页展示与重置密码）。
func (s *Store) ListAdmins() ([]User, error) {
	rows, err := s.DB.Query(`SELECT id,username,role,domain_id FROM users
		WHERE role IN ('global_admin','domain_admin','admin') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var domain sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &domain); err != nil {
			return nil, err
		}
		u.Role = migrateRole(u.Role)
		if domain.Valid {
			id := domain.Int64
			u.DomainID = &id
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteStudent 删除空间成员账号（级联清理会话与作答记录；仅允许 member 角色）。
func (s *Store) DeleteStudent(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM users WHERE id=? AND role IN ('member','student')`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStudentPassword 重置空间成员密码。
func (s *Store) SetStudentPassword(id int64, password string) error {
	if password == "" {
		return errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE users SET password_hash=? WHERE id=? AND role IN ('member','student')`, string(hash), id)
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

// SetUserRole 修改用户角色（member→domain_admin 等）与归属域；domainID 仅 domain_admin 使用。
// 返回 ErrNotFound（用户不存在）。
func (s *Store) SetUserRole(username string, role Role, domainID *int64) (int64, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil {
		return 0, err
	}
	role = migrateRole(role)
	if !role.ValidNew() {
		return 0, errors.New("非法角色")
	}
	var d any
	if role == RoleDomainAdmin && domainID != nil {
		d = *domainID
	}
	res, err := s.DB.Exec(`UPDATE users SET role=?, domain_id=? WHERE id=?`, string(role), d, u.ID)
	if err != nil {
		return 0, err
	}
	// 角色降级/迁移后清空旧会话（权限变化即时生效）
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, u.ID)
	_, _ = res.RowsAffected()
	return u.ID, nil
}

// ClearDomainAdmins 将某域的全部域管理员 domain_id 置空并降级为 member
// （删域后遗留的域管理员账号保留为普通成员）。
func (s *Store) ClearDomainAdmins(domainID int64) error {
	_, err := s.DB.Exec(`UPDATE users SET role='member', domain_id=NULL WHERE role='domain_admin' AND domain_id=?`, domainID)
	return err
}

// RemoveDomainAdmin 将某用户从域管理员降为普通成员（domain_id 清空；非域管理员返回 ErrNotFound）。
func (s *Store) RemoveDomainAdmin(userID int64) error {
	res, err := s.DB.Exec(`UPDATE users SET role='member', domain_id=NULL WHERE id=? AND role='domain_admin'`, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, userID)
	return nil
}

// ListDomainAdmins 某域的全部域管理员账号。
func (s *Store) ListDomainAdmins(domainID int64) ([]User, error) {
	rows, err := s.DB.Query(`SELECT id,username,role,domain_id FROM users
		WHERE role='domain_admin' AND domain_id=? ORDER BY id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var d sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &d); err != nil {
			return nil, err
		}
		if d.Valid {
			id := d.Int64
			u.DomainID = &id
		}
		out = append(out, u)
	}
	return out, rows.Err()
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
	u, err := s.scanUser(`SELECT u.id,u.username,u.role,u.domain_id FROM sessions se
		JOIN users u ON u.id=se.user_id WHERE se.token=?`, token)
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