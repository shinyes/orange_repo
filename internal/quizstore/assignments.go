// assignments 布置数据层（quiz.db）。
//
// 模仿上游 OrangeOJ 的训练/练习可见性语义：
//   - 训练参与者 training_participants / 练习目标 practice_targets 的统一模型：
//     assignments.assigned_all=1 → 全体学生；=0 → 仅 assigned_students 内学生；
//   - published=1 才对学生可见（发布/撤回）；
//   - 内容（章节/题目结构）实时读主库（RepoReader），本层只登记引用与可见性。
package quizstore

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Assignment 布置（管理员/学生通用视图）。
type Assignment struct {
	ID            int64     `json:"id"`
	Kind          string    `json:"kind"` // training | practice
	RepoID        int64     `json:"repoId"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	Published     bool      `json:"published"`
	AssignedAll   bool      `json:"assignedAll"`
	ProblemCount  int       `json:"problemCount"` // 实时主库题数（读取时填充）
	StudentCount  int       `json:"studentCount"` // 定向学生数（assignedAll 时 = 全体学生数）
	CreatedAt     time.Time `json:"createdAt"`
}

// AssignmentFilter 管理端列表条件。
type AssignmentFilter struct {
	Kind string // 空 = 全部
}

// CreateAssignment 创建布置（kind+repoId 唯一）。
func (s *Store) CreateAssignment(kind string, repoID int64, title, description string, tags []string, assignedAll bool, studentIDs []int64) (int64, error) {
	if kind != "training" && kind != "practice" {
		return 0, errors.New("kind 必须为 training 或 practice")
	}
	if repoID <= 0 {
		return 0, errors.New("repoId 非法")
	}
	if title == "" {
		return 0, errors.New("布置标题不能为空")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	all := 0
	if assignedAll {
		all = 1
	}
	res, err := tx.Exec(`INSERT INTO assignments(kind,repo_id,title,description,tags_json,published,assigned_all)
		VALUES(?,?,?,?,?,1,?)`, kind, repoID, title, description, encodeStrings(tags), all)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrConflict
		}
		return 0, err
	}
	id, _ := res.LastInsertId()

	if !assignedAll {
		if err := upsertAssignedStudents(tx, id, studentIDs); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// upsertAssignedStudents 事务内全量替换定向学生。
func upsertAssignedStudents(tx *sql.Tx, assignmentID int64, studentIDs []int64) error {
	if _, err := tx.Exec(`DELETE FROM assigned_students WHERE assignment_id=?`, assignmentID); err != nil {
		return err
	}
	for _, uid := range studentIDs {
		if uid <= 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO assigned_students(assignment_id,user_id) VALUES(?,?)`, assignmentID, uid); err != nil {
			return err
		}
	}
	return nil
}

// GetAssignment 单条布置（不含学生列表/统计）。
func (s *Store) GetAssignment(id int64) (*Assignment, error) {
	a := &Assignment{}
	var tags string
	var all, pub int
	err := s.DB.QueryRow(`SELECT id,kind,repo_id,title,description,tags_json,published,assigned_all,created_at
		FROM assignments WHERE id=?`, id).
		Scan(&a.ID, &a.Kind, &a.RepoID, &a.Title, &a.Description, &tags, &pub, &all, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Tags = decodeStrings(tags)
	a.Published = pub == 1
	a.AssignedAll = all == 1
	return a, nil
}

// UpdateAssignmentMeta 更新标题/描述/发布态。
func (s *Store) UpdateAssignmentMeta(id int64, title, description string, published bool) error {
	pub := 0
	if published {
		pub = 1
	}
	if title == "" {
		return errors.New("布置标题不能为空")
	}
	res, err := s.DB.Exec(`UPDATE assignments SET title=?,description=?,published=? WHERE id=?`, title, description, pub, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAssignedStudents 定向模式：assignedAll=true → 全体（清空定向）；false → 存列表。
func (s *Store) SetAssignedStudents(id int64, assignedAll bool, studentIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	all := 0
	if assignedAll {
		all = 1
	}
	res, err := tx.Exec(`UPDATE assignments SET assigned_all=? WHERE id=?`, all, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := upsertAssignedStudents(tx, id, studentIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAssignment 删除布置（级联 assigned_students）。
func (s *Store) DeleteAssignment(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM assignments WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAssignments 管理端全量布置列表（含定向学生数；problemCount 由调用方填主库）。
func (s *Store) ListAssignments(f AssignmentFilter) ([]Assignment, error) {
	q := `SELECT a.id,a.kind,a.repo_id,a.title,a.description,a.tags_json,a.published,a.assigned_all,a.created_at,
		(SELECT COUNT(*) FROM assigned_students s WHERE s.assignment_id=a.id)
		FROM assignments a`
	var args []any
	if f.Kind != "" {
		q += ` WHERE a.kind=?`
		args = append(args, f.Kind)
	}
	q += ` ORDER BY a.id DESC`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		var tags string
		var pub, all int
		if err := rows.Scan(&a.ID, &a.Kind, &a.RepoID, &a.Title, &a.Description, &tags, &pub, &all, &a.CreatedAt, &a.StudentCount); err != nil {
			return nil, err
		}
		a.Tags = decodeStrings(tags)
		a.Published = pub == 1
		a.AssignedAll = all == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssignedStudent 定向学生视图。
type AssignedStudent struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

// ListAssignedStudents 布置的定向学生（assignedAll=1 时返回空列表——按全体语义）。
func (s *Store) ListAssignedStudents(assignmentID int64) ([]AssignedStudent, error) {
	rows, err := s.DB.Query(`SELECT s.user_id,u.username FROM assigned_students s
		JOIN users u ON u.id=s.user_id WHERE s.assignment_id=? ORDER BY u.id`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssignedStudent{}
	for rows.Next() {
		var a AssignedStudent
		if err := rows.Scan(&a.UserID, &a.Username); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CanAccess 学生是否可见该布置（published + 全体/定向内）。管理员由调用方绕过。
func (s *Store) StudentCanAccess(assignmentID, userID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM assignments a
		WHERE a.id=? AND a.published=1
		AND (a.assigned_all=1 OR EXISTS(SELECT 1 FROM assigned_students s WHERE s.assignment_id=a.id AND s.user_id=?))`,
		assignmentID, userID).Scan(&n)
	return n > 0, err
}

// ListStudentAssignments 学生可见的布置（published + 全体/定向内）。
func (s *Store) ListStudentAssignments(userID int64, kind string) ([]Assignment, error) {
	q := `SELECT a.id,a.kind,a.repo_id,a.title,a.description,a.tags_json,a.published,a.assigned_all,a.created_at
		FROM assignments a WHERE a.published=1 AND a.kind=?
		AND (a.assigned_all=1 OR EXISTS(SELECT 1 FROM assigned_students s WHERE s.assignment_id=a.id AND s.user_id=?))
		ORDER BY a.id DESC`
	rows, err := s.DB.Query(q, kind, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		var tags string
		var pub, all int
		if err := rows.Scan(&a.ID, &a.Kind, &a.RepoID, &a.Title, &a.Description, &tags, &pub, &all, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Tags = decodeStrings(tags)
		a.Published = pub == 1
		a.AssignedAll = all == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// AllStudentIDs 全体学生 id 列表。
func (s *Store) AllStudentIDs() ([]int64, error) {
	rows, err := s.DB.Query(`SELECT id FROM users WHERE role='student' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AssignmentStudentSet 任务学生集合（统计用）：全体 → role=student 全部；否则定向列表。
func (s *Store) AssignmentStudentSet(assignmentID int64) ([]int64, error) {
	a, err := s.GetAssignment(assignmentID)
	if err != nil {
		return nil, err
	}
	if a.AssignedAll {
		rows, err := s.DB.Query(`SELECT id FROM users WHERE role='student' ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []int64{}
		for rows.Next() {
			var uid int64
			if err := rows.Scan(&uid); err != nil {
				return nil, err
			}
			out = append(out, uid)
		}
		return out, rows.Err()
	}
	rows, err := s.DB.Query(`SELECT user_id FROM assigned_students WHERE assignment_id=? ORDER BY user_id`, assignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// ProblemStats 每题统计（管理端）：通过人数（学生集内 best_verdict=AC）+ 提交次数（submit/objective）。
type ProblemStats struct {
	ProblemID      int64  `json:"problemId"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	AcceptedUsers  int    `json:"accepted"`
	SubmissionCount int   `json:"submissions"`
}

// ProblemStatsOf 任务内各题统计：
//   - accepted = 学生集内 user_problem_progress.best_verdict ∈ (AC,OK) 的去重学生数；
//   - submissions = 学生集内 submissions（submit_type IN ('submit','objective')）次数。
//
// 题目 id 集合由调用方（读主库实时结构）提供；结果 map 仅含集合内出现过统计的题目。
func (s *Store) ProblemStatsOf(studentIDs []int64, problemIDs []int64) (map[int64]ProblemStats, error) {
	out := map[int64]ProblemStats{}
	if len(studentIDs) == 0 || len(problemIDs) == 0 {
		return out, nil
	}
	userPh := buildPlaceholders(len(studentIDs))
	probPh := buildPlaceholders(len(problemIDs))
	userArgs := make([]any, 0, len(studentIDs))
	for _, id := range studentIDs {
		userArgs = append(userArgs, id)
	}
	probArgs := make([]any, 0, len(problemIDs))
	for _, id := range problemIDs {
		probArgs = append(probArgs, id)
	}

	// 通过人数（每人每题去重）
	rows, err := s.DB.Query(`SELECT problem_id, COUNT(DISTINCT user_id)
		FROM user_problem_progress
		WHERE user_id IN (`+userPh+`) AND problem_id IN (`+probPh+`) AND best_verdict IN ('AC','OK')
		GROUP BY problem_id`, append(append([]any{}, userArgs...), probArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var n int
		if err := rows.Scan(&pid, &n); err != nil {
			rows.Close()
			return nil, err
		}
		st := out[pid]
		st.ProblemID = pid
		st.AcceptedUsers = n
		out[pid] = st
	}
	rows.Close()

	// 提交次数
	rows, err = s.DB.Query(`SELECT problem_id, COUNT(*)
		FROM submissions
		WHERE user_id IN (`+userPh+`) AND problem_id IN (`+probPh+`)
		  AND submit_type IN ('submit','objective') AND status='done'
		GROUP BY problem_id`, append(append([]any{}, userArgs...), probArgs...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pid int64
		var n int
		if err := rows.Scan(&pid, &n); err != nil {
			rows.Close()
			return nil, err
		}
		st := out[pid]
		st.ProblemID = pid
		st.SubmissionCount = n
		out[pid] = st
	}
	rows.Close()
	return out, rows.Err()
}

func buildPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

// isUniqueViolation sqlite 唯一约束判断。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
