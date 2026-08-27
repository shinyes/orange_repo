package store

import (
	"database/sql"
	"errors"

	"orangerepo/internal/model"
)

// ---------- 练习（平铺题目编组，含分值） ----------

func (s *Store) CreatePractice(title, description string, tags []string, folderID *int64) (int64, error) {
	if err := s.ensureFolder(folderID); err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO practices(title,description,tags_json,folder_id) VALUES(?,?,?,?)`,
		title, description, encodeTags(tags), nullInt64(folderID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPractices 列出练习（含题目数）。
func (s *Store) ListPractices() ([]model.Practice, error) {
	rows, err := s.DB.Query(`SELECT id,title,description,tags_json,created_at,folder_id,
		(SELECT COUNT(*) FROM practice_items pi WHERE pi.practice_id=practices.id)
		FROM practices ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Practice
	for rows.Next() {
		var p model.Practice
		var tagsJSON string
		var count int
		var folder sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &tagsJSON, &p.CreatedAt, &folder, &count); err != nil {
			return nil, err
		}
		p.Tags = decodeTags(tagsJSON)
		if folder.Valid {
			p.FolderID = &folder.Int64
		}
		p.ProblemCount = count
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPractice(id int64) (*model.Practice, error) {
	list, err := s.ListPractices()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) UpdatePractice(id int64, title, description string, tags []string) error {
	res, err := s.DB.Exec(`UPDATE practices SET title=?,description=?,tags_json=? WHERE id=?`,
		title, description, encodeTags(tags), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeletePractice(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM practice_items WHERE practice_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM practices WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// AddPracticeItems 追加题目条目到练习末尾；校验题目存在。
func (s *Store) AddPracticeItems(practiceID int64, problemIDs []int64) ([]int64, error) {
	if _, err := s.GetPractice(practiceID); err != nil {
		return nil, err
	}
	var maxOrder int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0) FROM practice_items WHERE practice_id=?`, practiceID).Scan(&maxOrder)
	ids := make([]int64, 0, len(problemIDs))
	for _, pid := range problemIDs {
		var exists int
		if err := s.DB.QueryRow(`SELECT 1 FROM problems WHERE id=?`, pid).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		maxOrder++
		res, err := s.DB.Exec(`INSERT INTO practice_items(practice_id,problem_id,order_no) VALUES(?,?,?)`,
			practiceID, pid, maxOrder)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ReorderPracticeItems 按给定 itemIds 顺序整表重排。
func (s *Store) ReorderPracticeItems(practiceID int64, itemIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM practice_items WHERE practice_id=?`, practiceID).Scan(&total); err != nil {
		return err
	}
	if len(itemIDs) != total {
		return errors.New("itemIds must cover all items of the practice")
	}
	for i, iid := range itemIDs {
		if _, err := tx.Exec(`UPDATE practice_items SET order_no=? WHERE id=? AND practice_id=?`, i+1, iid, practiceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePracticeItem 删除练习条目。
func (s *Store) DeletePracticeItem(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM practice_items WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPracticeItems 返回练习条目（含题目标题/类型）。
func (s *Store) ListPracticeItems(practiceID int64) ([]model.PracticeItem, error) {
	rows, err := s.DB.Query(`SELECT pi.id,pi.practice_id,pi.problem_id,pi.order_no,
		COALESCE(p.title,''),COALESCE(p.type,'')
		FROM practice_items pi LEFT JOIN problems p ON p.id=pi.problem_id
		WHERE pi.practice_id=? ORDER BY pi.order_no,pi.id`, practiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PracticeItem{}
	for rows.Next() {
		var it model.PracticeItem
		if err := rows.Scan(&it.ID, &it.PracticeID, &it.ProblemID, &it.OrderNo,
			&it.ProblemTitle, &it.ProblemType); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
