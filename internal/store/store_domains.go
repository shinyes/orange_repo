// 域 / 空间 / 空间成员数据层（OJ 重构）。
// 数据物理位于主库 orangeoj.db；users 账号在 orangeoj.db（accounts 包），
// space_members.user_id 为逻辑引用（无跨库 FK）。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"orangeoj/internal/model"
)

// ---------- 域 ----------

// CreateDomain 新建域（名称唯一）。
func (s *Store) CreateDomain(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("域名称不能为空")
	}
	res, err := s.DB.Exec(`INSERT INTO domains(name) VALUES(?)`, name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, errors.New("域名称已存在")
		}
		return 0, err
	}
	return res.LastInsertId()
}

// ListDomains 全部域。
func (s *Store) ListDomains() ([]model.Domain, error) {
	rows, err := s.DB.Query(`SELECT id,name,leaderboard_public,created_at FROM domains ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Domain{}
	for rows.Next() {
		var d model.Domain
		var pub int
		if err := rows.Scan(&d.ID, &d.Name, &pub, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.LeaderboardPublic = pub != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDomain 取域。
func (s *Store) GetDomain(id int64) (*model.Domain, error) {
	d := &model.Domain{}
	var pub int
	err := s.DB.QueryRow(`SELECT id,name,leaderboard_public,created_at FROM domains WHERE id=?`, id).
		Scan(&d.ID, &d.Name, &pub, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.LeaderboardPublic = pub != 0
	return d, nil
}

// RenameDomain 改域名。
func (s *Store) RenameDomain(id int64, name string) error {
	return s.UpdateDomainMeta(id, &name, nil)
}

// UpdateDomainMeta 部分更新域元信息：仅更新非 nil 的字段
// （name/leaderboardPublic 均可省略；两者皆 nil 时报错）。
func (s *Store) UpdateDomainMeta(id int64, name *string, leaderboardPublic *bool) error {
	var sets []string
	var args []any
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errors.New("域名称不能为空")
		}
		sets = append(sets, "name=?")
		args = append(args, n)
	}
	if leaderboardPublic != nil {
		v := 0
		if *leaderboardPublic {
			v = 1
		}
		sets = append(sets, "leaderboard_public=?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return errors.New("无更新字段")
	}
	args = append(args, id)
	res, err := s.DB.Exec(`UPDATE domains SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errors.New("域名称已存在")
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDomain 删域（级联删空间与成员；题目不会级联删除——必须先显式删除
// 域内题目（DeleteDomainProblems）。域内仍有题目时返回错误（防裸删静默删题/歧义）。
func (s *Store) DeleteDomain(id int64) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE domain_id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("域内仍有 %d 道题目，请先删除题目（deleteProblems=true）", n)
	}
	res, err := s.DB.Exec(`DELETE FROM domains WHERE id=?`, id)
	if err != nil {
		return err
	}
	n2, _ := res.RowsAffected()
	if n2 == 0 {
		return ErrNotFound
	}
	return nil
}

// CountDomainProblems 域内题目数（删除域前提示/确认）。
func (s *Store) CountDomainProblems(domainID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM problems WHERE domain_id=?`, domainID).Scan(&n)
	return n, err
}

// ---------- 空间 ----------

// CreateSpace 域内新建空间（同域内名称唯一）。
func (s *Store) CreateSpace(domainID int64, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("空间名称不能为空")
	}
	res, err := s.DB.Exec(`INSERT INTO spaces(domain_id,name) VALUES(?,?)`, domainID, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSpaces 域内空间列表。
func (s *Store) ListSpaces(domainID int64) ([]model.Space, error) {
	rows, err := s.DB.Query(`SELECT id,domain_id,name,created_at FROM spaces WHERE domain_id=? ORDER BY id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Space
	for rows.Next() {
		var sp model.Space
		if err := rows.Scan(&sp.ID, &sp.DomainID, &sp.Name, &sp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// GetSpace 取空间（含所属域校验由上层做）。
func (s *Store) GetSpace(id int64) (*model.Space, error) {
	sp := &model.Space{}
	err := s.DB.QueryRow(`SELECT id,domain_id,name,created_at FROM spaces WHERE id=?`, id).
		Scan(&sp.ID, &sp.DomainID, &sp.Name, &sp.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sp, nil
}

// RenameSpace 空间改名。
func (s *Store) RenameSpace(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("空间名称不能为空")
	}
	res, err := s.DB.Exec(`UPDATE spaces SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSpace 删空间（级联删空间内训练/练习/成员——训练/练习表尚未建，先删成员）。
func (s *Store) DeleteSpace(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM spaces WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- 空间成员 ----------

// SpaceMemberIDs 空间成员 user_id 列表。
func (s *Store) SpaceMemberIDs(spaceID int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT user_id FROM space_members WHERE space_id=? ORDER BY user_id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetSpaceMembers 覆盖设置空间成员（全量替换；空列表=清空）。
func (s *Store) SetSpaceMembers(spaceID int64, userIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM space_members WHERE space_id=?`, spaceID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO space_members(space_id,user_id) VALUES(?,?)`, spaceID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UserSpaceIDs 用户加入的全部空间 id（门户空间切换）。
func (s *Store) UserSpaceIDs(userID int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT space_id FROM space_members WHERE user_id=? ORDER BY space_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RemoveUserFromAllSpaces 将用户从全部空间移除（删成员账号时清理）。
func (s *Store) RemoveUserFromAllSpaces(userID int64) error {
	_, err := s.DB.Exec(`DELETE FROM space_members WHERE user_id=?`, userID)
	return err
}

// UserInSpace 用户是否为空间成员。
func (s *Store) UserInSpace(spaceID, userID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM space_members WHERE space_id=? AND user_id=?`, spaceID, userID).Scan(&n)
	return n > 0, err
}

// SpaceOfDomain 校验空间属于域（域管理员操作前鉴权）。
func (s *Store) SpaceOfDomain(spaceID, domainID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM spaces WHERE id=? AND domain_id=?`, spaceID, domainID).Scan(&n)
	return n > 0, err
}

// SpaceDomain 空间所属域 id。
func (s *Store) SpaceDomain(spaceID int64) (int64, error) {
	var domainID int64
	err := s.DB.QueryRow(`SELECT domain_id FROM spaces WHERE id=?`, spaceID).Scan(&domainID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return domainID, nil
}

// ---------- 空间内容表（训练/练习/刷题/排行榜，阶段 4） ----------

// migrateSpaceContent 建空间训练/练习/刷题结构表（幂等）。
// 学生作答表（尝试/交卷/通过记录）已在 orangeoj.db（quizstore.migrate），此处不再建。
func (s *Store) migrateSpaceContent() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS space_trainings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL DEFAULT '',
			space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			max_attempts INTEGER NOT NULL DEFAULT 3, -- 训练级客观题统一选择上限
			is_public INTEGER NOT NULL DEFAULT 0, -- 1=空间全体成员可见（免可见名单）；0=仅可见名单（默认）
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS space_training_chapters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			training_id INTEGER NOT NULL REFERENCES space_trainings(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS space_training_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chapter_id INTEGER NOT NULL REFERENCES space_training_chapters(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS space_practices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL DEFAULT '',
			space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			is_public INTEGER NOT NULL DEFAULT 0, -- 1=空间全体成员可见（免可见名单）；0=仅可见名单（默认）
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS space_practice_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			practice_id INTEGER NOT NULL REFERENCES space_practices(id) ON DELETE CASCADE,
			problem_id INTEGER NOT NULL,
			order_no INTEGER NOT NULL DEFAULT 0
		);`,
		// 空间刷题项目
		`CREATE TABLE IF NOT EXISTS space_quizzes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL DEFAULT '',
			space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			source_type TEXT NOT NULL DEFAULT 'tags',
			repo_kind TEXT NOT NULL DEFAULT '',
			repo_id INTEGER NOT NULL DEFAULT 0,
			round_size INTEGER NOT NULL DEFAULT 0,
			is_public INTEGER NOT NULL DEFAULT 0, -- 1=空间全体成员可见（免可见名单）；0=仅可见名单（默认）
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		// ---------- 空间内容可见成员授权（空=默认无成员可见；管理员始终可见） ----------
		`CREATE TABLE IF NOT EXISTS space_training_visible (
			training_id INTEGER NOT NULL REFERENCES space_trainings(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			PRIMARY KEY(training_id, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS space_practice_visible (
			practice_id INTEGER NOT NULL REFERENCES space_practices(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			PRIMARY KEY(practice_id, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS space_quiz_visible (
			quiz_id INTEGER NOT NULL REFERENCES space_quizzes(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL,
			PRIMARY KEY(quiz_id, user_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_space_trainings_space ON space_trainings(space_id);`,
		`CREATE INDEX IF NOT EXISTS idx_space_practices_space ON space_practices(space_id);`,
		`CREATE INDEX IF NOT EXISTS idx_space_quizzes_space ON space_quizzes(space_id);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate space content failed: %w; stmt: %s", err, stmt)
		}
	}
	// 刷题项目每轮题数（0=不限制：整范围一轮）
	if err := s.ensureColumn("space_quizzes", "round_size", `round_size INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	// 公开开关（存量库补列；0=仅可见名单成员可见[默认]，1=空间全体成员可见）
	for _, tbl := range []string{"space_trainings", "space_practices", "space_quizzes"} {
		if err := s.ensureColumn(tbl, "is_public", `is_public INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDomainProblems 删除域内全部题目（含训练/练习条目引用清理），
// 供删域（?deleteProblems=true）级联使用。
func (s *Store) DeleteDomainProblems(domainID int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 收集域内题目 id
	rows, err := tx.Query(`SELECT id FROM problems WHERE domain_id=?`, domainID)
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
		if _, err := tx.Exec(`DELETE FROM training_items WHERE problem_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM practice_items WHERE problem_id=?`, id); err != nil {
			return err
		}
		// 空间条目引用（空间训练/练习条目无 FK，防悬挂指向已删题）
		if _, err := tx.Exec(`DELETE FROM space_training_items WHERE problem_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM space_practice_items WHERE problem_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM problems WHERE id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DomainOnlyBookletIDs 返回「含 ≥1 道题且全部题目都属于 domainID」的仓库训练/练习模板 id
// （删域前收集：删题后这些模板变空壳，应随之删除；他域/混合/空模板不受影响）。
func (s *Store) DomainOnlyBookletIDs(domainID int64) (trainingIDs, practiceIDs []int64, err error) {
	rows, err := s.DB.Query(`SELECT DISTINCT t.id FROM trainings t
		WHERE EXISTS (SELECT 1 FROM training_items i JOIN training_chapters c ON i.chapter_id=c.id
			JOIN problems p ON p.id=i.problem_id WHERE c.training_id=t.id AND p.domain_id=?)
		AND NOT EXISTS (SELECT 1 FROM training_items i JOIN training_chapters c ON i.chapter_id=c.id
			JOIN problems p ON p.id=i.problem_id WHERE c.training_id=t.id AND (p.domain_id IS NULL OR p.domain_id<>?))
		ORDER BY t.id`, domainID, domainID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		trainingIDs = append(trainingIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	prows, err := s.DB.Query(`SELECT DISTINCT p.id FROM practices p
		WHERE EXISTS (SELECT 1 FROM practice_items i JOIN problems pr ON pr.id=i.problem_id
			WHERE i.practice_id=p.id AND pr.domain_id=?)
		AND NOT EXISTS (SELECT 1 FROM practice_items i JOIN problems pr ON pr.id=i.problem_id
			WHERE i.practice_id=p.id AND (pr.domain_id IS NULL OR pr.domain_id<>?))
		ORDER BY p.id`, domainID, domainID)
	if err != nil {
		return nil, nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var id int64
		if err := prows.Scan(&id); err != nil {
			return nil, nil, err
		}
		practiceIDs = append(practiceIDs, id)
	}
	return trainingIDs, practiceIDs, prows.Err()
}

// DeleteBookletIDs 按 id 删除仓库训练/练习模板（级联其章节；练习无章节）。
func (s *Store) DeleteBookletIDs(trainingIDs, practiceIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(trainingIDs) > 0 {
		for _, id := range trainingIDs {
			if _, err := tx.Exec(`DELETE FROM trainings WHERE id=?`, id); err != nil {
				return err
			}
		}
	}
	if len(practiceIDs) > 0 {
		for _, id := range practiceIDs {
			if _, err := tx.Exec(`DELETE FROM practices WHERE id=?`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
