package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"orangerepo/internal/model"
)

// ---------- 题册目录（可嵌套，训练/练习归属其中） ----------

// ListBookletDirectories 返回全部目录（扁平列表，按同级 order_no 排序）。
func (s *Store) ListBookletDirectories() ([]model.BookletDirectory, error) {
	rows, err := s.DB.Query(`SELECT id,name,parent_id,order_no FROM booklet_directories ORDER BY order_no,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BookletDirectory{}
	for rows.Next() {
		var d model.BookletDirectory
		var parent sql.NullInt64
		if err := rows.Scan(&d.ID, &d.Name, &parent, &d.OrderNo); err != nil {
			return nil, err
		}
		if parent.Valid {
			d.ParentID = &parent.Int64
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateBookletDirectory 新建目录；parentID 为 nil 表示根目录。
func (s *Store) CreateBookletDirectory(name string, parentID *int64) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("目录名不能为空")
	}
	if parentID != nil {
		var n int
		if err := s.DB.QueryRow(`SELECT 1 FROM booklet_directories WHERE id=?`, *parentID).Scan(&n); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, ErrNotFound
			}
			return 0, err
		}
	}
	var order int
	if parentID != nil {
		_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0)+1 FROM booklet_directories WHERE parent_id=?`, *parentID).Scan(&order)
	} else {
		_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0)+1 FROM booklet_directories WHERE parent_id IS NULL`).Scan(&order)
	}
	res, err := s.DB.Exec(`INSERT INTO booklet_directories(name,parent_id,order_no) VALUES(?,?,?)`,
		name, nullInt64(parentID), order)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameBookletDirectory 重命名目录。
func (s *Store) RenameBookletDirectory(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("目录名不能为空")
	}
	res, err := s.DB.Exec(`UPDATE booklet_directories SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBookletDirectory 删除目录：
//   - deleteBooklets=true 时，直接归属该目录的训练/练习一并删除；
//   - 否则它们移到顶层（folder_id=NULL）；
//   - 直接子目录始终上移一层（挂到被删目录的父目录，根目录为 NULL）。
func (s *Store) DeleteBookletDirectory(id int64, deleteBooklets bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parent sql.NullInt64
	err = tx.QueryRow(`SELECT parent_id FROM booklet_directories WHERE id=?`, id).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	steps := []struct {
		query string
		args  []any
	}{
		{`UPDATE booklet_directories SET parent_id=? WHERE parent_id=?`, []any{parent, id}},
		{`DELETE FROM booklet_directories WHERE id=?`, []any{id}},
	}
	if deleteBooklets {
		// 删除直接归属的训练/练习（含其章节/条目）再删目录
		rows, err := tx.Query(`SELECT id FROM trainings WHERE folder_id=?`, id)
		if err != nil {
			return err
		}
		var trainIDs []int64
		for rows.Next() {
			var tid int64
			if err := rows.Scan(&tid); err != nil {
				rows.Close()
				return err
			}
			trainIDs = append(trainIDs, tid)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		rows, err = tx.Query(`SELECT id FROM practices WHERE folder_id=?`, id)
		if err != nil {
			return err
		}
		var pracIDs []int64
		for rows.Next() {
			var pid int64
			if err := rows.Scan(&pid); err != nil {
				rows.Close()
				return err
			}
			pracIDs = append(pracIDs, pid)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, tid := range trainIDs {
			if err := deleteTrainingTx(tx, tid); err != nil {
				return err
			}
		}
		for _, pid := range pracIDs {
			if err := deletePracticeTx(tx, pid); err != nil {
				return err
			}
		}
	} else {
		// 不删题册：直接归属的题册移到顶层
		steps = append(steps,
			struct {
				query string
				args  []any
			}{`UPDATE trainings SET folder_id=NULL WHERE folder_id=?`, []any{id}},
			struct {
				query string
				args  []any
			}{`UPDATE practices SET folder_id=NULL WHERE folder_id=?`, []any{id}},
		)
	}
	for _, st := range steps {
		if _, err := tx.Exec(st.query, st.args...); err != nil {
			return fmt.Errorf("delete directory: %w", err)
		}
	}
	return tx.Commit()
}

// SetTrainingFolder 设置训练所属目录；folderID 为 nil 表示移到根目录。
func (s *Store) SetTrainingFolder(id int64, folderID *int64) error {
	if err := s.ensureFolder(folderID); err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE trainings SET folder_id=? WHERE id=?`, nullInt64(folderID), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPracticeFolder 设置练习所属目录；folderID 为 nil 表示移到根目录。
func (s *Store) SetPracticeFolder(id int64, folderID *int64) error {
	if err := s.ensureFolder(folderID); err != nil {
		return err
	}
	res, err := s.DB.Exec(`UPDATE practices SET folder_id=? WHERE id=?`, nullInt64(folderID), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetBookletDirectoryLayout 原子化重排目录树（拖拽改层级/顺序的一次性提交）：
//
//   - placements 必须恰好覆盖全部目录（无重复、无遗漏、无外来 id）；
//   - parentId 必须为 null 或现存目录，且不能指向自身；
//   - 提交的父链必须无环（不能把目录移进自己的子孙）；
//   - orderNo 决定同级顺序，写入后按「父级 + 顺序」整体生效。
//
// 任一校验失败则整体回滚。
func (s *Store) SetBookletDirectoryLayout(placements []model.BookletDirectory) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id,name,parent_id FROM booklet_directories`)
	if err != nil {
		return err
	}
	existing := map[int64]bool{}
	byID := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		var parent sql.NullInt64
		if err := rows.Scan(&id, &name, &parent); err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
		byID[id] = name
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(placements) != len(existing) {
		return errors.New("layout must cover all directories exactly once")
	}
	posted := map[int64]bool{}
	next := map[int64]*int64{}
	orderOf := map[int64]int{}
	for _, p := range placements {
		if !existing[p.ID] {
			return errors.New("layout contains a directory outside the tree")
		}
		if posted[p.ID] {
			return errors.New("duplicate directory id in layout")
		}
		posted[p.ID] = true
		if p.ParentID != nil {
			if *p.ParentID == p.ID {
				return fmt.Errorf("directory %q cannot be its own parent", byID[p.ID])
			}
			if !existing[*p.ParentID] {
				return fmt.Errorf("parent of %q does not exist", byID[p.ID])
			}
			v := *p.ParentID
			next[p.ID] = &v
		} else {
			next[p.ID] = nil
		}
		orderOf[p.ID] = p.OrderNo
	}
	// 无环校验：沿父链走，遇到已访问节点即成环
	visit := func(start int64) bool {
		seen := map[int64]bool{}
		cur := start
		for {
			if seen[cur] {
				return true
			}
			seen[cur] = true
			p := next[cur]
			if p == nil {
				return false
			}
			cur = *p
		}
	}
	for id := range existing {
		if visit(id) {
			return fmt.Errorf("directory %q move would create a cycle", byID[id])
		}
	}

	// 写入
	for id := range existing {
		if _, err := tx.Exec(`UPDATE booklet_directories SET parent_id=?,order_no=? WHERE id=?`,
			nullInt64(next[id]), orderOf[id], id); err != nil {
			return fmt.Errorf("apply layout: %w", err)
		}
	}
	return tx.Commit()
}

// ensureFolder 校验目录存在（folderID 为 nil 时跳过）。
// BookletDirectoryExists 目录是否存在。
func (s *Store) BookletDirectoryExists(id int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT 1 FROM booklet_directories WHERE id=?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ensureFolder(folderID *int64) error {
	if folderID == nil {
		return nil
	}
	var n int
	if err := s.DB.QueryRow(`SELECT 1 FROM booklet_directories WHERE id=?`, *folderID).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// nullInt64 将 *int64 转为 sql.NullInt64（nil → NULL）。
func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}