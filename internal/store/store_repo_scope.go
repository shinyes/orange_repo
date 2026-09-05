// 仓库模板域校验：空间「从仓库选训练/练习」时，模板的题目须全部属于目标域
// （模板表本身无 domain 列，按条目题目域判定）。
package store

import (
	"database/sql"

	"orangerepo/internal/model"
)

// ListTrainingsInDomain 返回含至少一道该域题目的训练模板（仓库页按域选模板用）。
func (s *Store) ListTrainingsInDomain(domainID int64) ([]model.Training, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT t.id,t.title,t.description,t.tags_json,t.created_at,
		COALESCE(t.folder_id,0)
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
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &tags, &t.CreatedAt, &folder); err != nil {
			return nil, err
		}
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
func (s *Store) ListPracticesInDomain(domainID int64) ([]model.Practice, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT p.id,p.title,p.description,p.tags_json,p.created_at,
		COALESCE(p.folder_id,0)
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
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &tags, &p.CreatedAt, &folder); err != nil {
			return nil, err
		}
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
