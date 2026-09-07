// 空间内容数据层（结构部分）：空间训练（章节+条目）、空间练习（整卷）、
// 空间刷题项目。学生作答数据（尝试/交卷/通过记录）在 quizstore（orangeoj.db，
// 与 users 同库），此处仅保留空间内容结构 CRUD。
package store

import (
	"database/sql"
	"errors"
)

// ---------- 空间训练 ----------

// SpaceTraining 空间训练视图（含题量）。
type SpaceTraining struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	MaxAttempts  int      `json:"maxAttempts"`
	ProblemCount int      `json:"problemCount"`
}

// SpaceChapter 空间训练章节（含条目）。
type SpaceChapter struct {
	ID         int64              `json:"id"`
	TrainingID int64              `json:"trainingId"`
	Title      string             `json:"title"`
	OrderNo    int                `json:"orderNo"`
	Items      []SpaceChapterItem `json:"items"`
}

// SpaceChapterItem 章节条目。
type SpaceChapterItem struct {
	ID           int64  `json:"id"`
	ChapterID    int64  `json:"chapterId"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
	ProblemUUID  string `json:"problemUuid,omitempty"`
}

// CreateSpaceTraining 建空间训练（默认 max_attempts=3；0=不限）。
func (s *Store) CreateSpaceTraining(spaceID int64, title, description string, tags []string, maxAttempts int) (int64, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	u, err := NewUUIDv7()
	if err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO space_trainings(uuid,space_id,title,description,tags_json,max_attempts)
		VALUES(?,?,?,?,?,?)`, u, spaceID, title, description, encodeTags(tags), maxAttempts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSpaceTrainings 空间内训练列表。
func (s *Store) ListSpaceTrainings(spaceID int64) ([]SpaceTraining, error) {
	rows, err := s.DB.Query(`SELECT t.id,t.uuid,t.space_id,t.title,t.description,t.tags_json,t.max_attempts,
		(SELECT COUNT(*) FROM space_training_items i JOIN space_training_chapters c ON i.chapter_id=c.id WHERE c.training_id=t.id)
		FROM space_trainings t WHERE t.space_id=? ORDER BY t.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceTraining
	for rows.Next() {
		var t SpaceTraining
		var tags string
		if err := rows.Scan(&t.ID, &t.UUID, &t.SpaceID, &t.Title, &t.Description, &tags, &t.MaxAttempts, &t.ProblemCount); err != nil {
			return nil, err
		}
		t.Tags = decodeTags(tags)
		out = append(out, t)
	}
	return nonNilSlice(out), rows.Err()
}

// GetSpaceTraining 取训练（含章节/条目题目信息）。
func (s *Store) GetSpaceTraining(id int64) (*SpaceTraining, []SpaceChapter, error) {
	var t SpaceTraining
	var tags string
	err := s.DB.QueryRow(`SELECT t.id,t.uuid,t.space_id,t.title,t.description,t.tags_json,t.max_attempts,
		(SELECT COUNT(*) FROM space_training_items i JOIN space_training_chapters c ON i.chapter_id=c.id WHERE c.training_id=t.id)
		FROM space_trainings t WHERE t.id=?`, id).
		Scan(&t.ID, &t.UUID, &t.SpaceID, &t.Title, &t.Description, &tags, &t.MaxAttempts, &t.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	t.Tags = decodeTags(tags)
	chapters, err := s.ListSpaceChapters(id)
	if err != nil {
		return nil, nil, err
	}
	return &t, chapters, nil
}

// UpdateSpaceTrainingMeta 部分更新训练元信息（nil 指针 = 该字段不变）。
// maxAttempts 语义：0 = 不限次（显式）；>0 = 限 N 次；nil = 不变。
func (s *Store) UpdateSpaceTrainingMeta(id int64, title, description *string, tags []string, maxAttempts *int) error {
	// 读当前值组装 UPDATE（保持部分更新；tags nil = 不变）
	var curTitle, curDesc, curTags string
	var curMax int
	if err := s.DB.QueryRow(`SELECT title,description,tags_json,max_attempts FROM space_trainings WHERE id=?`, id).
		Scan(&curTitle, &curDesc, &curTags, &curMax); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if title != nil {
		curTitle = *title
	}
	if description != nil {
		curDesc = *description
	}
	if tags != nil {
		curTags = encodeTags(tags)
	}
	if maxAttempts != nil {
		curMax = *maxAttempts
	}
	res, err := s.DB.Exec(`UPDATE space_trainings SET title=?,description=?,tags_json=?,max_attempts=? WHERE id=?`,
		curTitle, curDesc, curTags, curMax, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSpaceTraining 删训练（级联章节/条目；学生尝试记录在 orangeoj.db，由上层清理）。
func (s *Store) DeleteSpaceTraining(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_trainings WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSpaceChapter 训练加章节（自动排末尾）。
func (s *Store) CreateSpaceChapter(trainingID int64, title string) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO space_training_chapters(training_id,title,order_no)
		SELECT ?,?,COALESCE(MAX(order_no),0)+1 FROM space_training_chapters WHERE training_id=?`,
		trainingID, title, trainingID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameSpaceChapter 章节改名。
func (s *Store) RenameSpaceChapter(id int64, title string) error {
	res, err := s.DB.Exec(`UPDATE space_training_chapters SET title=? WHERE id=?`, title, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSpaceChapter 删章节（级联条目）。
func (s *Store) DeleteSpaceChapter(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_training_chapters WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderSpaceChapters 按给定章节 id 顺序重写训练内章节 order_no（快编/管理端章节排序）。
func (s *Store) ReorderSpaceChapters(trainingID int64, chapterIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 校验全部章节归属该训练（防跨训练重排）
	for _, cid := range chapterIDs {
		var tid int64
		err := tx.QueryRow(`SELECT training_id FROM space_training_chapters WHERE id=?`, cid).Scan(&tid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if tid != trainingID {
			return ErrNotFound
		}
	}
	for i, cid := range chapterIDs {
		if _, err := tx.Exec(`UPDATE space_training_chapters SET order_no=? WHERE id=?`, i+1, cid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReorderSpaceChapterItems 按给定条目 id 顺序重写章节内条目 order_no（题目排序）。
func (s *Store) ReorderSpaceChapterItems(chapterID int64, itemIDs []int64) error {	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 校验全部条目归属该章节
	for _, iid := range itemIDs {
		var chID int64
		err := tx.QueryRow(`SELECT chapter_id FROM space_training_items WHERE id=?`, iid).Scan(&chID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if chID != chapterID {
			return ErrNotFound
		}
	}
	for i, iid := range itemIDs {
		if _, err := tx.Exec(`UPDATE space_training_items SET order_no=? WHERE id=?`, i+1, iid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReorderSpacePracticeItems 空间练习条目全量排序（itemIDs 须覆盖该练习全部条目且归属正确）。
func (s *Store) ReorderSpacePracticeItems(practiceID int64, itemIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, iid := range itemIDs {
		var pid int64
		err := tx.QueryRow(`SELECT practice_id FROM space_practice_items WHERE id=?`, iid).Scan(&pid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if pid != practiceID {
			return ErrNotFound
		}
	}
	// 必须覆盖全部条目（防止遗漏导致部分条目 order 丢失）
	var total int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM space_practice_items WHERE practice_id=?`, practiceID).Scan(&total); err != nil {
		return err
	}
	if len(itemIDs) != total {
		return errors.New("itemIds must cover all items of the practice")
	}
	for i, iid := range itemIDs {
		if _, err := tx.Exec(`UPDATE space_practice_items SET order_no=? WHERE id=?`, i+1, iid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddSpaceChapterItems 章节追加题目（跳过不存在的题；返回加入的 item id）。
func (s *Store) AddSpaceChapterItems(chapterID int64, problemIDs []int64) ([]int64, error) {
	var out []int64
	for _, pid := range problemIDs {
		res, err := s.DB.Exec(`INSERT INTO space_training_items(chapter_id,problem_id,order_no)
			SELECT ?,?,COALESCE(MAX(order_no),0)+1 FROM space_training_items WHERE chapter_id=?`,
			chapterID, pid, chapterID)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		out = append(out, id)
	}
	return nonNilSlice(out), nil
}

// RemoveSpaceChapterItem 从章节移除条目。
func (s *Store) RemoveSpaceChapterItem(itemID int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_training_items WHERE id=?`, itemID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SpaceIDOfTrainingItem 训练条目所属空间（越权删除防护；条目→章节→训练→空间）。
func (s *Store) SpaceIDOfTrainingItem(itemID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT t.space_id FROM space_training_items i
		JOIN space_training_chapters c ON i.chapter_id=c.id
		JOIN space_trainings t ON c.training_id=t.id
		WHERE i.id=?`, itemID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return spaceID, nil
}

// SpaceIDOfTraining 训练所属空间。
func (s *Store) SpaceIDOfTraining(trainingID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT space_id FROM space_trainings WHERE id=?`, trainingID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// SpaceIDOfChapter 训练章节所属空间。
func (s *Store) SpaceIDOfChapter(chapterID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT t.space_id FROM space_training_chapters c
		JOIN space_trainings t ON c.training_id=t.id WHERE c.id=?`, chapterID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// SpaceIDOfPractice 练习所属空间。
func (s *Store) SpaceIDOfPractice(practiceID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT space_id FROM space_practices WHERE id=?`, practiceID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// SpaceIDOfQuiz 刷题项目所属空间。
func (s *Store) SpaceIDOfQuiz(quizID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT space_id FROM space_quizzes WHERE id=?`, quizID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// SpaceIDOfPracticeItem 练习条目所属空间（练习条目删除校验用）。
func (s *Store) SpaceIDOfPracticeItem(itemID int64) (int64, error) {
	var spaceID int64
	err := s.DB.QueryRow(`SELECT p.space_id FROM space_practice_items i
		JOIN space_practices p ON i.practice_id=p.id WHERE i.id=?`, itemID).Scan(&spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// ListSpaceChapters 训练全部章节（含条目题目信息，LEFT JOIN problems 读 title/type/uuid）。
func (s *Store) ListSpaceChapters(trainingID int64) ([]SpaceChapter, error) {
	rows, err := s.DB.Query(`SELECT id,title,order_no FROM space_training_chapters
		WHERE training_id=? ORDER BY order_no,id`, trainingID)
	if err != nil {
		return nil, err
	}
	var chapters []SpaceChapter
	for rows.Next() {
		var ch SpaceChapter
		if err := rows.Scan(&ch.ID, &ch.Title, &ch.OrderNo); err != nil {
			rows.Close()
			return nil, err
		}
		chapters = append(chapters, ch)
	}
	rows.Close()
	for i := range chapters {
		items, err := s.spaceChapterItems(chapters[i].ID)
		if err != nil {
			return nil, err
		}
		chapters[i].Items = items
	}
	return nonNilSlice(chapters), nil
}

func (s *Store) spaceChapterItems(chapterID int64) ([]SpaceChapterItem, error) {
	rows, err := s.DB.Query(`SELECT i.id,i.chapter_id,i.problem_id,i.order_no,p.title,p.type,p.uuid
		FROM space_training_items i LEFT JOIN problems p ON p.id=i.problem_id
		WHERE i.chapter_id=? ORDER BY i.order_no,i.id`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceChapterItem
	for rows.Next() {
		var it SpaceChapterItem
		var title, typ, u string
		var titleN, typN, uN sql.NullString
		if err := rows.Scan(&it.ID, &it.ChapterID, &it.ProblemID, &it.OrderNo, &titleN, &typN, &uN); err != nil {
			return nil, err
		}
		if titleN.Valid {
			title = titleN.String
		}
		if typN.Valid {
			typ = typN.String
		}
		if uN.Valid {
			u = uN.String
		}
		it.ProblemTitle = title
		it.ProblemType = typ
		it.ProblemUUID = u
		out = append(out, it)
	}
	return nonNilSlice(out), rows.Err()
}

// ---------- 空间练习 ----------

// SpacePractice 空间练习视图。
type SpacePractice struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	ProblemCount int      `json:"problemCount"`
}

// SpacePracticeItem 练习条目（含题目信息）。
type SpacePracticeItem struct {
	ID           int64  `json:"id"`
	PracticeID   int64  `json:"practiceId"`
	ProblemID    int64  `json:"problemId"`
	OrderNo      int    `json:"orderNo"`
	ProblemTitle string `json:"problemTitle,omitempty"`
	ProblemType  string `json:"problemType,omitempty"`
	ProblemUUID  string `json:"problemUuid,omitempty"`
}

// CreateSpacePractice 建空间练习。
func (s *Store) CreateSpacePractice(spaceID int64, title, description string, tags []string) (int64, error) {
	u, err := NewUUIDv7()
	if err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO space_practices(uuid,space_id,title,description,tags_json) VALUES(?,?,?,?,?)`,
		u, spaceID, title, description, encodeTags(tags))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSpacePractices 空间练习列表。
func (s *Store) ListSpacePractices(spaceID int64) ([]SpacePractice, error) {
	rows, err := s.DB.Query(`SELECT p.id,p.uuid,p.space_id,p.title,p.description,p.tags_json,
		(SELECT COUNT(*) FROM space_practice_items i WHERE i.practice_id=p.id)
		FROM space_practices p WHERE p.space_id=? ORDER BY p.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpacePractice
	for rows.Next() {
		var p SpacePractice
		var tags string
		if err := rows.Scan(&p.ID, &p.UUID, &p.SpaceID, &p.Title, &p.Description, &tags, &p.ProblemCount); err != nil {
			return nil, err
		}
		p.Tags = decodeTags(tags)
		out = append(out, p)
	}
	return nonNilSlice(out), rows.Err()
}

// GetSpacePractice 取练习及条目。
func (s *Store) GetSpacePractice(id int64) (*SpacePractice, []SpacePracticeItem, error) {
	var p SpacePractice
	var tags string
	err := s.DB.QueryRow(`SELECT p.id,p.uuid,p.space_id,p.title,p.description,p.tags_json,
		(SELECT COUNT(*) FROM space_practice_items i WHERE i.practice_id=p.id)
		FROM space_practices p WHERE p.id=?`, id).
		Scan(&p.ID, &p.UUID, &p.SpaceID, &p.Title, &p.Description, &tags, &p.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	p.Tags = decodeTags(tags)
	items, err := s.ListSpacePracticeItems(id)
	if err != nil {
		return nil, nil, err
	}
	return &p, items, nil
}

// UpdateSpacePracticeMeta 更新练习名称/描述/标签。
func (s *Store) UpdateSpacePracticeMeta(id int64, title, description string, tags []string) error {
	res, err := s.DB.Exec(`UPDATE space_practices SET title=?,description=?,tags_json=? WHERE id=?`,
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

// DeleteSpacePractice 删练习（级联条目；学生交卷记录在 orangeoj.db，由上层清理）。
func (s *Store) DeleteSpacePractice(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_practices WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddSpacePracticeItems 练习追加题目（排末尾）。
func (s *Store) AddSpacePracticeItems(practiceID int64, problemIDs []int64) error {
	for _, pid := range problemIDs {
		if _, err := s.DB.Exec(`INSERT INTO space_practice_items(practice_id,problem_id,order_no)
			SELECT ?,?,COALESCE(MAX(order_no),0)+1 FROM space_practice_items WHERE practice_id=?`,
			practiceID, pid, practiceID); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSpacePracticeItem 移除练习条目。
func (s *Store) RemoveSpacePracticeItem(itemID int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_practice_items WHERE id=?`, itemID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListSpacePracticeItems 练习条目（含题目信息）。
func (s *Store) ListSpacePracticeItems(practiceID int64) ([]SpacePracticeItem, error) {
	rows, err := s.DB.Query(`SELECT i.id,i.practice_id,i.problem_id,i.order_no,p.title,p.type,p.uuid
		FROM space_practice_items i LEFT JOIN problems p ON p.id=i.problem_id
		WHERE i.practice_id=? ORDER BY i.order_no,i.id`, practiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpacePracticeItem
	for rows.Next() {
		var it SpacePracticeItem
		var titleN, typN, uN sql.NullString
		if err := rows.Scan(&it.ID, &it.PracticeID, &it.ProblemID, &it.OrderNo, &titleN, &typN, &uN); err != nil {
			return nil, err
		}
		if titleN.Valid {
			it.ProblemTitle = titleN.String
		}
		if typN.Valid {
			it.ProblemType = typN.String
		}
		if uN.Valid {
			it.ProblemUUID = uN.String
		}
		out = append(out, it)
	}
	return nonNilSlice(out), rows.Err()
}

// ---------- 空间刷题项目 ----------

// SpaceQuiz 刷题项目视图。
type SpaceQuiz struct {
	ID           int64    `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	SpaceID      int64    `json:"spaceId"`
	Title        string   `json:"title"`
	Tags         []string `json:"tags"`
	SourceType   string   `json:"sourceType"` // tags | repo
	RepoKind     string   `json:"repoKind,omitempty"`
	RepoID       int64    `json:"repoId,omitempty"`
	ProblemCount int      `json:"problemCount"`
}

// CreateSpaceQuiz 建刷题项目。
func (s *Store) CreateSpaceQuiz(spaceID int64, title string, tags []string, sourceType, repoKind string, repoID int64) (int64, error) {
	u, err := NewUUIDv7()
	if err != nil {
		return 0, err
	}
	res, err := s.DB.Exec(`INSERT INTO space_quizzes(uuid,space_id,title,tags_json,source_type,repo_kind,repo_id)
		VALUES(?,?,?,?,?,?,?)`, u, spaceID, title, encodeTags(tags), sourceType, repoKind, repoID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSpaceQuizzes 空间刷题项目列表。
func (s *Store) ListSpaceQuizzes(spaceID int64) ([]SpaceQuiz, error) {
	rows, err := s.DB.Query(`SELECT id,uuid,space_id,title,tags_json,source_type,repo_kind,repo_id
		FROM space_quizzes WHERE space_id=? ORDER BY id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceQuiz
	for rows.Next() {
		var q SpaceQuiz
		var tags string
		if err := rows.Scan(&q.ID, &q.UUID, &q.SpaceID, &q.Title, &tags, &q.SourceType, &q.RepoKind, &q.RepoID); err != nil {
			return nil, err
		}
		q.Tags = decodeTags(tags)
		out = append(out, q)
	}
	return nonNilSlice(out), rows.Err()
}

// DeleteSpaceQuiz 删刷题项目。
func (s *Store) DeleteSpaceQuiz(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM space_quizzes WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
