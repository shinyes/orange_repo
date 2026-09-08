// 删域时的刷题侧数据清理：submissions/进度/草稿/错题集/会话/通过记录 等
// 引用被删域题目的数据一并清除（与主库删题/删域联动）。
package quizstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// CleanupDomainProblems 清理引用 problemIDs 的作答侧数据；uuids 用于 student_solved。
func (s *Store) CleanupDomainProblems(problemIDs []int64, uuids []string) error {
	if len(problemIDs) == 0 && len(uuids) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 直接按 problem_id 删的表（judge_jobs/user_problem_progress 经 submissions FK 级联）
	for _, pid := range problemIDs {
		for _, stmt := range []string{
			`DELETE FROM submissions WHERE problem_id=?`,
			`DELETE FROM code_drafts WHERE problem_id=?`,
			`DELETE FROM wrong_book WHERE problem_id=?`,
			`DELETE FROM space_training_attempts WHERE problem_id=?`,
		} {
			if _, err := tx.Exec(stmt, pid); err != nil {
				return fmt.Errorf("cleanup problems: %w", err)
			}
		}
	}
	// student_solved：uuid 去重记录
	for _, u := range uuids {
		if u == "" {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM student_solved WHERE problem_uuid=?`, u); err != nil {
			return fmt.Errorf("cleanup solved: %w", err)
		}
	}
	// 快照/会话 JSON 内嵌引用：拉取解码后过滤写回（行数有限）
	if err := s.cleanupJSONRefs(tx, problemIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// cleanupJSONRefs 清理 practice 提交快照与刷题会话 JSON 里引用已删题目的行：
// space_practice_submissions.answers_json（元素 problemId）、
// quiz_sessions.wrong_json/drawn_json（纯 id 数组）。行级过滤回写。
func (s *Store) cleanupJSONRefs(tx *sql.Tx, problemIDs []int64) error {
	del := map[int64]bool{}
	for _, id := range problemIDs {
		del[id] = true
	}
	// 1) 练习提交快照：answers_json 元素 {problemId,...} → 移除引用该题的记录（快照整体作废语义：
	//    整卷含被删题则该次记录不可还原——直接删除该次提交记录）
	rows, err := tx.Query(`SELECT id, answers_json FROM space_practice_submissions`)
	if err != nil {
		return err
	}
	type item struct {
		ProblemID int64 `json:"problemId"`
	}
	dropIDs := []int64{}
	for rows.Next() {
		var id int64
		var aj string
		if err := rows.Scan(&id, &aj); err != nil {
			rows.Close()
			return err
		}
		var items []item
		if json.Unmarshal([]byte(aj), &items) == nil {
			for _, it := range items {
				if del[it.ProblemID] {
					dropIDs = append(dropIDs, id)
					break
				}
			}
		}
	}
	rows.Close()
	for _, id := range dropIDs {
		if _, err := tx.Exec(`DELETE FROM space_practice_submissions WHERE id=?`, id); err != nil {
			return err
		}
	}

	// 2) 刷题会话：wrong/drawn 数组过滤后写回（保留其他题进度）
	sessRows, err := tx.Query(`SELECT user_id, quiz_id, wrong_json, drawn_json FROM quiz_sessions`)
	if err != nil {
		return err
	}
	type sessFix struct {
		user, quiz      int64
		wrong, drawn    []int64
	}
	var fixes []sessFix
	for sessRows.Next() {
		var user, quiz int64
		var wj, dj string
		if err := sessRows.Scan(&user, &quiz, &wj, &dj); err != nil {
			sessRows.Close()
			return err
		}
		var w, d []int64
		_ = json.Unmarshal([]byte(wj), &w)
		_ = json.Unmarshal([]byte(dj), &d)
		w2 := make([]int64, 0, len(w))
		for _, x := range w {
			if !del[x] {
				w2 = append(w2, x)
			}
		}
		d2 := make([]int64, 0, len(d))
		for _, x := range d {
			if !del[x] {
				d2 = append(d2, x)
			}
		}
		if len(w2) != len(w) || len(d2) != len(d) {
			fixes = append(fixes, sessFix{user: user, quiz: quiz, wrong: w2, drawn: d2})
		}
	}
	sessRows.Close()
	for _, f := range fixes {
		wj, _ := json.Marshal(f.wrong)
		dj, _ := json.Marshal(f.drawn)
		if _, err := tx.Exec(`UPDATE quiz_sessions SET wrong_json=?, drawn_json=? WHERE user_id=? AND quiz_id=?`,
			string(wj), string(dj), f.user, f.quiz); err != nil {
			return err
		}
	}
	return nil
}
