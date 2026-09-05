// 仓库模板域校验：空间「从仓库选训练/练习」时，模板的题目须全部属于目标域
// （模板表本身无 domain 列，按条目题目域判定）。
package store

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
