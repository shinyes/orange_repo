// Package quizstore 封装刷题服务的两级存储：
//
//   - quiz.db：刷题服务自有数据（用户、会话、科目、分类、错题、全局设置），本服务唯一写路径；
//   - orangerepo.db：只读访问主站题库（见 problems.go 的 RepoReader）。
//
// 标签匹配语义复用 internal/store.TagMatchesSelected（唯一权威实现，不重复发明）。
package quizstore

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"orangerepo/internal/store"
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

// Student 学生账号管理视图（含错题数）。
type Student struct {
	ID         int64     `json:"id"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"createdAt"`
	WrongCount int       `json:"wrongCount"`
}

// Subject 科目（含有序分类列表）。
type Subject struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	OrderNo    int        `json:"orderNo"`
	Categories []Category `json:"categories"`
}

// Category 刷题分类：映射主库标签列表与题型列表（第一阶段仅单选/判断）。
type Category struct {
	ID        int64    `json:"id"`
	SubjectID int64    `json:"subjectId"`
	Name      string   `json:"name"`
	OrderNo   int      `json:"orderNo"`
	Tags      []string `json:"tags"`
	Types     []string `json:"types"`
}

// WrongGroup 错题集按分类统计。
type WrongGroup struct {
	CategoryID   int64  `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	SubjectName  string `json:"subjectName"`
	Count        int    `json:"count"`
}

// WrongProblem 错题练习条目：problemID + 记录所属分类（答错时归集到原分类）。
type WrongProblem struct {
	ProblemID  int64
	CategoryID int64
}

// Store 刷题服务存储：quiz.db 写 + 主库只读。
type Store struct {
	DB   *sql.DB
	Repo *RepoReader
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
	s := &Store{DB: db}
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
		`CREATE TABLE IF NOT EXISTS subjects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0,
			tags_json TEXT NOT NULL DEFAULT '[]',
			types_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS wrong_answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, problem_id)
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`INSERT INTO settings(key,value) VALUES('round_size','10')
			ON CONFLICT(key) DO NOTHING;`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("quiz migrate failed: %w; stmt: %s", err, stmt)
		}
	}
	return nil
}

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

// CreateUser 创建用户（用户名大小写不敏感唯一；role ∈ admin|student）。
func (s *Store) CreateUser(username, password string, role Role) (int64, error) {
	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return 0, err
	}
	if password == "" {
		return 0, errors.New("密码不能为空")
	}
	if role != RoleAdmin && role != RoleStudent {
		return 0, errors.New("非法角色")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO users(username,password_hash,role) VALUES(?,?,?)`,
		username, string(hash), string(role))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrConflict
		}
		return 0, err
	}
	return res.LastInsertId()
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

// ListStudents 学生账号列表（含各自错题数）。
func (s *Store) ListStudents() ([]Student, error) {
	rows, err := s.DB.Query(`SELECT u.id,u.username,u.created_at,COUNT(w.id)
		FROM users u LEFT JOIN wrong_answers w ON w.user_id=u.id
		WHERE u.role='student' GROUP BY u.id ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Student
	for rows.Next() {
		var st Student
		if err := rows.Scan(&st.ID, &st.Username, &st.CreatedAt, &st.WrongCount); err != nil {
			return nil, err
		}
		out = append(out, st)
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

// SetPassword 修改当前用户密码（管理员或学生均可）。
func (s *Store) SetPassword(userID int64, newPassword string) error {
	if newPassword == "" {
		return errors.New("密码不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(hash), userID)
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

// RotateSession 删除旧会话并创建新会话（改密码后轮换）。
func (s *Store) RotateSession(oldToken string, userID int64) (string, error) {
	_ = s.DeleteSession(oldToken)
	return s.CreateSession(userID)
}

// ---------- 全局设置 ----------

const settingRoundSize = "round_size"

// GetRoundSize 每轮题数（默认 10）。
func (s *Store) GetRoundSize() int {
	v, ok := s.getSetting(settingRoundSize)
	if !ok {
		return 10
	}
	n := 0
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
		return 10
	}
	return n
}

// SetRoundSize 设置每轮题数（1–100）。
func (s *Store) SetRoundSize(n int) error {
	if n < 1 || n > 100 {
		return errors.New("每轮题数需在 1–100 之间")
	}
	return s.setSetting(settingRoundSize, fmt.Sprintf("%d", n))
}

func (s *Store) getSetting(key string) (string, bool) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) setSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// ---------- 科目 ----------

// ListSubjects 科目列表（含各科目下有序分类）。
func (s *Store) ListSubjects() ([]Subject, error) {
	rows, err := s.DB.Query(`SELECT id,name,order_no FROM subjects ORDER BY order_no,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subjects []Subject
	for rows.Next() {
		var sub Subject
		if err := rows.Scan(&sub.ID, &sub.Name, &sub.OrderNo); err != nil {
			return nil, err
		}
		sub.Categories = []Category{}
		subjects = append(subjects, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range subjects {
		sub := &subjects[i]
		cats, err := s.ListCategories(sub.ID)
		if err != nil {
			return nil, err
		}
		sub.Categories = cats
	}
	return subjects, nil
}

// CreateSubject 创建科目，order_no 追加到末尾。
func (s *Store) CreateSubject(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("科目名称不能为空")
	}
	var max int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0) FROM subjects`).Scan(&max)
	res, err := s.DB.Exec(`INSERT INTO subjects(name,order_no) VALUES(?,?)`, name, max+1)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameSubject 重命名科目。
func (s *Store) RenameSubject(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("科目名称不能为空")
	}
	res, err := s.DB.Exec(`UPDATE subjects SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSubject 删除科目（级联分类与错题记录）。
func (s *Store) DeleteSubject(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM subjects WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSubjectOrder 按 id 数组设置科目显示顺序（数组必须完整覆盖全部科目，事务内幂等）。
func (s *Store) SetSubjectOrder(ids []int64) error {
	existing, err := s.SubjectIDs()
	if err != nil {
		return err
	}
	if len(ids) != len(existing) {
		return errors.New("科目顺序列表不完整")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if !existing[id] || seen[id] {
			return errors.New("科目顺序列表含缺失或重复项")
		}
		seen[id] = true
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE subjects SET order_no=? WHERE id=?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SubjectIDs 全部科目 id 集合。
func (s *Store) SubjectIDs() (map[int64]bool, error) {
	rows, err := s.DB.Query(`SELECT id FROM subjects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ---------- 分类 ----------

// ValidQuizTypes 第一阶段允许的题型。
var ValidQuizTypes = map[string]bool{"single_choice": true, "true_false": true}

// NormalizeTypes 校验并规范化题型列表：仅允许单选/判断；空列表表示两种都算。
func NormalizeTypes(types []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range types {
		if !ValidQuizTypes[t] {
			return nil, fmt.Errorf("第一阶段仅支持单选题与判断题（%q 非法）", t)
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out, nil
}

// NormalizeTags 逐条校验标签路径。
func NormalizeTags(tags []string) error {
	for _, t := range tags {
		if _, err := store.ValidateTagPath(t); err != nil {
			return err
		}
	}
	return nil
}

func encodeStrings(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeStrings(s string) []string {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// ListCategories 科目下的分类（有序）。
func (s *Store) ListCategories(subjectID int64) ([]Category, error) {
	rows, err := s.DB.Query(`SELECT id,subject_id,name,order_no,tags_json,types_json
		FROM categories WHERE subject_id=? ORDER BY order_no,id`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		var tj, tyj string
		if err := rows.Scan(&c.ID, &c.SubjectID, &c.Name, &c.OrderNo, &tj, &tyj); err != nil {
			return nil, err
		}
		c.Tags = decodeStrings(tj)
		c.Types = decodeStrings(tyj)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCategory 取单个分类。
func (s *Store) GetCategory(id int64) (*Category, error) {
	c := &Category{}
	var tj, tyj string
	err := s.DB.QueryRow(`SELECT id,subject_id,name,order_no,tags_json,types_json
		FROM categories WHERE id=?`, id).
		Scan(&c.ID, &c.SubjectID, &c.Name, &c.OrderNo, &tj, &tyj)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Tags = decodeStrings(tj)
	c.Types = decodeStrings(tyj)
	return c, nil
}

// CreateCategory 创建分类（orderNo<=0 时追加到科目末尾）。
func (s *Store) CreateCategory(subjectID int64, name string, orderNo int, tags, types []string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("分类名称不能为空")
	}
	types, err := NormalizeTypes(types)
	if err != nil {
		return 0, err
	}
	if err := NormalizeTags(tags); err != nil {
		return 0, err
	}
	if orderNo <= 0 {
		_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0) FROM categories WHERE subject_id=?`, subjectID).Scan(&orderNo)
		orderNo++
	}
	res, err := s.DB.Exec(`INSERT INTO categories(subject_id,name,order_no,tags_json,types_json)
		VALUES(?,?,?,?,?)`, subjectID, name, orderNo, encodeStrings(tags), encodeStrings(types))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCategory 全量更新分类（name 为空表示不修改名称；orderNo<=0 表示不修改顺序）。
func (s *Store) UpdateCategory(c Category) error {
	types, err := NormalizeTypes(c.Types)
	if err != nil {
		return err
	}
	if err := NormalizeTags(c.Tags); err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE categories SET
		name=CASE WHEN ?='' THEN name ELSE ? END,
		order_no=CASE WHEN ?<=0 THEN order_no ELSE ? END,
		tags_json=?, types_json=? WHERE id=?`,
		c.Name, c.Name, c.OrderNo, c.OrderNo, encodeStrings(c.Tags), encodeStrings(types), c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCategory 删除分类（级联错题记录）。
func (s *Store) DeleteCategory(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM categories WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCategoryOrder 按 id 数组设置科目内分类顺序。
func (s *Store) SetCategoryOrder(subjectID int64, ids []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		res, err := tx.Exec(`UPDATE categories SET order_no=? WHERE id=? AND subject_id=?`, i+1, id, subjectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("分类 %d 不属于科目 %d", id, subjectID)
		}
	}
	return tx.Commit()
}

// ---------- 错题集 ----------

// AddWrong 记录错题（同一学生对同一题只记录一次，保留首次分类）。
func (s *Store) AddWrong(userID, problemID, categoryID int64) error {
	if categoryID <= 0 {
		return errors.New("缺少分类")
	}
	_, err := s.DB.Exec(`INSERT INTO wrong_answers(user_id,problem_id,category_id) VALUES(?,?,?)
		ON CONFLICT(user_id,problem_id) DO NOTHING`, userID, problemID, categoryID)
	return err
}

// RemoveWrong 答对后从错题集移除该题。
func (s *Store) RemoveWrong(userID, problemID int64) error {
	_, err := s.DB.Exec(`DELETE FROM wrong_answers WHERE user_id=? AND problem_id=?`, userID, problemID)
	return err
}

// WrongGroups 错题按分类统计（按科目/分类顺序排列）。
func (s *Store) WrongGroups(userID int64) ([]WrongGroup, error) {
	rows, err := s.DB.Query(`SELECT c.id,c.name,sub.name,COUNT(w.id)
		FROM wrong_answers w
		JOIN categories c ON c.id=w.category_id
		JOIN subjects sub ON sub.id=c.subject_id
		WHERE w.user_id=?
		GROUP BY c.id ORDER BY sub.order_no,c.order_no,c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WrongGroup
	for rows.Next() {
		var g WrongGroup
		if err := rows.Scan(&g.CategoryID, &g.CategoryName, &g.SubjectName, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// WrongTotal 错题总数。
func (s *Store) WrongTotal(userID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM wrong_answers WHERE user_id=?`, userID).Scan(&n)
	return n, err
}

// ListWrongProblems 错题条目（categoryID 为空 = 全部）。
func (s *Store) ListWrongProblems(userID int64, categoryID *int64) ([]WrongProblem, error) {
	q := `SELECT problem_id,category_id FROM wrong_answers WHERE user_id=?`
	var args []any
	args = append(args, userID)
	if categoryID != nil {
		q += ` AND category_id=?`
		args = append(args, *categoryID)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WrongProblem
	for rows.Next() {
		var wp WrongProblem
		if err := rows.Scan(&wp.ProblemID, &wp.CategoryID); err != nil {
			return nil, err
		}
		out = append(out, wp)
	}
	return out, rows.Err()
}