// 仓库模板域校验：空间「从仓库选训练/练习」时，模板的题目须全部属于目标域
// （模板表本身无 domain 列，按条目题目域判定）。
package store

import (
	"database/sql"
	"strings"

	"orangeoj/internal/model"
)

// ListTrainingsInDomain 返回含至少一道该域题目的训练模板（仓库页按域选模板用）。
// problemCount 与 ListTrainings 同口径（题册内全部条目），供管理端题册栏显示「N 题」。
func (s *Store) ListTrainingsInDomain(domainID int64) ([]model.Training, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT t.id,t.uuid,t.title,t.description,t.tags_json,t.created_at,
		t.folder_id,
		(SELECT COUNT(*) FROM training_items ti JOIN training_chapters tc ON ti.chapter_id=tc.id WHERE tc.training_id=t.id)
		FROM trainings t
		WHERE EXISTS (
			SELECT 1 FROM training_items i JOIN training_chapters c ON i.chapter_id=c.id
			JOIN problems p ON p.id=i.problem_id
			WHERE c.training_id=t.id AND p.domain_id=?
		) ORDER BY t.id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Training
	for rows.Next() {
		var t model.Training
		var tags string
		var folder sql.NullInt64
		var count int
		if err := rows.Scan(&t.ID, &t.UUID, &t.Title, &t.Description, &tags, &t.CreatedAt, &folder, &count); err != nil {
			return nil, err
		}
		t.ProblemCount = count
		t.Tags = decodeTags(tags)
		if folder.Valid {
			id := folder.Int64
			t.FolderID = &id
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListPracticesInDomain 含至少一道该域题目的练习模板。
// problemCount 与 ListPractices 同口径（题册内全部条目），供管理端题册栏显示「N 题」。
func (s *Store) ListPracticesInDomain(domainID int64) ([]model.Practice, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT p.id,p.uuid,p.title,p.description,p.tags_json,p.created_at,
		p.folder_id,
		(SELECT COUNT(*) FROM practice_items pi WHERE pi.practice_id=p.id)
		FROM practices p
		WHERE EXISTS (
			SELECT 1 FROM practice_items i JOIN problems pr ON pr.id=i.problem_id
			WHERE i.practice_id=p.id AND pr.domain_id=?
		) ORDER BY p.id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Practice
	for rows.Next() {
		var p model.Practice
		var tags string
		var folder sql.NullInt64
		var count int
		if err := rows.Scan(&p.ID, &p.UUID, &p.Title, &p.Description, &tags, &p.CreatedAt, &folder, &count); err != nil {
			return nil, err
		}
		p.ProblemCount = count
		p.Tags = decodeTags(tags)
		if folder.Valid {
			id := folder.Int64
			p.FolderID = &id
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TrainingInDomain 训练全部条目题目属于 domainID（无条目视为真）。
func (s *Store) TrainingInDomain(trainingID, domainID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM training_items i
		JOIN training_chapters c ON i.chapter_id=c.id
		JOIN problems p ON p.id=i.problem_id
		WHERE c.training_id=? AND (p.domain_id IS NULL OR p.domain_id != ?)`, trainingID, domainID).Scan(&n)
	return n == 0, err
}

// PracticeInDomain 练习全部条目题目属于 domainID。
func (s *Store) PracticeInDomain(practiceID, domainID int64) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM practice_items i
		JOIN problems p ON p.id=i.problem_id
		WHERE i.practice_id=? AND (p.domain_id IS NULL OR p.domain_id != ?)`, practiceID, domainID).Scan(&n)
	return n == 0, err
}

// FilterProblemsInDomain 过滤出属于 domainID 的题目 id（加题域门禁：仅放行同域题，
// 不存在的题目自然被滤除；返回过滤后的列表与是否全部通过）。
func (s *Store) FilterProblemsInDomain(ids []int64, domainID int64) ([]int64, bool, error) {
	if len(ids) == 0 {
		return ids, true, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, domainID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.DB.Query(`SELECT id FROM problems WHERE domain_id=? AND id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	allowed := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		allowed[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out, len(out) == len(ids), nil
}
