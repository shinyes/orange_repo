package store

import (
	"database/sql"
	"errors"

	"orangerepo/internal/model"
)

// ---------- 训练计划（章节化题目编组） ----------

func (s *Store) CreateTraining(title, description string, tags []string) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO trainings(title,description,tags_json) VALUES(?,?,?)`,
		title, description, encodeTags(tags))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanTrainingRows(rows *sql.Rows) ([]model.Training, error) {
	defer rows.Close()
	var out []model.Training
	for rows.Next() {
		var t model.Training
		var tagsJSON string
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &tagsJSON, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Tags = decodeTags(tagsJSON)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTrainings 列出训练（含题目数）。
func (s *Store) ListTrainings() ([]model.Training, error) {
	rows, err := s.DB.Query(`SELECT id,title,description,tags_json,created_at,
		(SELECT COUNT(*) FROM training_items ti JOIN training_chapters tc ON ti.chapter_id=tc.id WHERE tc.training_id=trainings.id)
		FROM trainings ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Training
	for rows.Next() {
		var t model.Training
		var tagsJSON string
		var count int
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &tagsJSON, &t.CreatedAt, &count); err != nil {
			return nil, err
		}
		t.Tags = decodeTags(tagsJSON)
		t.ProblemCount = count
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTraining(id int64) (*model.Training, error) {
	list, err := s.ListTrainings()
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

func (s *Store) UpdateTraining(id int64, title, description string, tags []string) error {
	res, err := s.DB.Exec(`UPDATE trainings SET title=?,description=?,tags_json=? WHERE id=?`,
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

func (s *Store) DeleteTraining(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM training_items WHERE chapter_id IN (SELECT id FROM training_chapters WHERE training_id=?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM training_chapters WHERE training_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM trainings WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateChapter 新建章节，排到末尾。
func (s *Store) CreateChapter(trainingID int64, title string) (int64, error) {
	if _, err := s.GetTraining(trainingID); err != nil {
		return 0, err
	}
	var order int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0)+1 FROM training_chapters WHERE training_id=?`,
		trainingID).Scan(&order)
	res, err := s.DB.Exec(`INSERT INTO training_chapters(training_id,title,order_no) VALUES(?,?,?)`,
		trainingID, title, order)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateChapter 更新章节标题/排序。
func (s *Store) UpdateChapter(id int64, title string, orderNo int) error {
	res, err := s.DB.Exec(`UPDATE training_chapters SET title=?,order_no=? WHERE id=?`, title, orderNo, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteChapter(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM training_items WHERE chapter_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM training_chapters WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// AddChapterItems 追加题目条目到章节末尾；校验题目存在。
func (s *Store) AddChapterItems(chapterID int64, problemIDs []int64) ([]int64, error) {
	var trainingID int64
	err := s.DB.QueryRow(`SELECT training_id FROM training_chapters WHERE id=?`, chapterID).Scan(&trainingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var maxOrder int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0) FROM training_items WHERE chapter_id=?`, chapterID).Scan(&maxOrder)
	ids := make([]int64, 0, len(problemIDs))
	for _, pid := range problemIDs {
		var exists int
		if err := s.DB.QueryRow(`SELECT 1 FROM problems WHERE id=?`, pid).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue // 跳过不存在的题目
			}
			return nil, err
		}
		maxOrder++
		res, err := s.DB.Exec(`INSERT INTO training_items(chapter_id,problem_id,order_no) VALUES(?,?,?)`,
			chapterID, pid, maxOrder)
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

// ReorderChapterItems 按给定 itemIds 顺序整表重排（必须覆盖该章全部条目）。
func (s *Store) ReorderChapterItems(chapterID int64, itemIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM training_items WHERE chapter_id=?`, chapterID).Scan(&total); err != nil {
		return err
	}
	if len(itemIDs) != total {
		return errors.New("itemIds must cover all items of the chapter")
	}
	for i, iid := range itemIDs {
		if _, err := tx.Exec(`UPDATE training_items SET order_no=? WHERE id=? AND chapter_id=?`, i+1, iid, chapterID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ChapterLayout 单章的条目顺序与归属（用于训练整体布局重排）。
type ChapterLayout struct {
	ChapterID int64
	ItemIDs   []int64
}

// SetTrainingLayout 原子化重排训练布局：
//
//   - chapterIds 必须是该训练全部章节的一个排列，决定章节顺序；
//   - layout 必须覆盖全部章节，且所有 ItemIDs 的并集必须恰好等于该训练全部条目
//     （无重复、无遗漏、无外来 id），决定每章条目顺序并支持跨章节移动。
//
// 任一校验失败则整 体回滚。
func (s *Store) SetTrainingLayout(trainingID int64, chapterIDs []int64, layout []ChapterLayout) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 章节完整性：chapterIds 与 layout 都必须恰好覆盖该训练的全部章节
	rows, err := tx.Query(`SELECT id FROM training_chapters WHERE training_id=?`, trainingID)
	if err != nil {
		return err
	}
	existingChapters := map[int64]bool{}
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return err
		}
		existingChapters[cid] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(chapterIDs) != len(existingChapters) || len(layout) != len(existingChapters) {
		return errors.New("layout must cover all chapters of the training exactly once")
	}
	for _, cid := range chapterIDs {
		if !existingChapters[cid] {
			return errors.New("chapterIds contains a chapter outside this training")
		}
	}

	// 条目完整性：并集必须等于该训练全部条目
	rows, err = tx.Query(`SELECT ti.id FROM training_items ti
		JOIN training_chapters tc ON ti.chapter_id=tc.id
		WHERE tc.training_id=?`, trainingID)
	if err != nil {
		return err
	}
	existingItems := map[int64]bool{}
	for rows.Next() {
		var iid int64
		if err := rows.Scan(&iid); err != nil {
			rows.Close()
			return err
		}
		existingItems[iid] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	posted := map[int64]bool{}
	for _, g := range layout {
		if !existingChapters[g.ChapterID] {
			return errors.New("layout contains a chapter outside this training")
		}
		for _, iid := range g.ItemIDs {
			if posted[iid] {
				return errors.New("duplicate itemId in layout")
			}
			posted[iid] = true
		}
	}
	if len(posted) != len(existingItems) {
		return errors.New("itemIds must cover all items of the training exactly once")
	}
	for iid := range posted {
		if !existingItems[iid] {
			return errors.New("itemIds contains an item outside this training")
		}
	}

	// 写入：章节顺序 + 条目归属与顺序
	for i, cid := range chapterIDs {
		if _, err := tx.Exec(`UPDATE training_chapters SET order_no=? WHERE id=? AND training_id=?`, i+1, cid, trainingID); err != nil {
			return err
		}
	}
	for _, g := range layout {
		for i, iid := range g.ItemIDs {
			if _, err := tx.Exec(`UPDATE training_items SET chapter_id=?, order_no=? WHERE id=?`, g.ChapterID, i+1, iid); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteItem(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM training_items WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListChapters 返回训练的章节与条目（条目含题目标题/类型）。
func (s *Store) ListChapters(trainingID int64) ([]model.Chapter, error) {
	rows, err := s.DB.Query(`SELECT id,training_id,title,order_no FROM training_chapters
		WHERE training_id=? ORDER BY order_no,id`, trainingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chapters []model.Chapter
	for rows.Next() {
		var c model.Chapter
		if err := rows.Scan(&c.ID, &c.TrainingID, &c.Title, &c.OrderNo); err != nil {
			return nil, err
		}
		c.Items = []model.Item{}
		chapters = append(chapters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range chapters {
		items, err := s.chapterItems(chapters[i].ID)
		if err != nil {
			return nil, err
		}
		chapters[i].Items = items
	}
	return chapters, nil
}

func (s *Store) chapterItems(chapterID int64) ([]model.Item, error) {
	rows, err := s.DB.Query(`SELECT ti.id,ti.chapter_id,ti.problem_id,ti.order_no,
		COALESCE(p.title,''),COALESCE(p.type,'')
		FROM training_items ti LEFT JOIN problems p ON p.id=ti.problem_id
		WHERE ti.chapter_id=? ORDER BY ti.order_no,ti.id`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Item
	for rows.Next() {
		var it model.Item
		if err := rows.Scan(&it.ID, &it.ChapterID, &it.ProblemID, &it.OrderNo, &it.ProblemTitle, &it.ProblemType); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
