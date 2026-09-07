// 空间内容可见成员授权（训练/练习/刷题）。
// 语义：空名单 = 默认无成员可见（仅管理员）；名单可随时增删；成员级授权。
package store

import (
	"errors"
)

// visibleColumn 表名 → 资源 id 列名（表名与列名不一致：space_training_visible.training_id 等）。
func visibleColumn(table string) string {
	switch table {
	case "space_training_visible":
		return "training_id"
	case "space_practice_visible":
		return "practice_id"
	case "space_quiz_visible":
		return "quiz_id"
	}
	return ""
}

// SetVisibleUsers 覆盖式设置某空间内容的可见成员名单（空=清空授权，无成员可见）。
func (s *Store) SetVisibleUsers(table string, itemID int64, userIDs []int64) error {
	if !validVisibleTable(table) {
		return errors.New("invalid visible table")
	}
	idCol := visibleColumn(table)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE `+idCol+`=?`, itemID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(`INSERT INTO `+table+`(`+idCol+`,user_id) VALUES(?,?)`, itemID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// VisibleUserIDs 读取某空间内容的可见成员名单。
func (s *Store) VisibleUserIDs(table string, itemID int64) ([]int64, error) {
	if !validVisibleTable(table) {
		return nil, errors.New("invalid visible table")
	}
	idCol := visibleColumn(table)
	rows, err := s.DB.Query(`SELECT user_id FROM `+table+` WHERE `+idCol+`=? ORDER BY user_id`, itemID)
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

// IsUserVisible 该用户是否在可见名单（member 门户过滤用）。
func (s *Store) IsUserVisible(table string, itemID, userID int64) (bool, error) {
	if !validVisibleTable(table) {
		return false, errors.New("invalid visible table")
	}
	idCol := visibleColumn(table)
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(1) FROM `+table+` WHERE `+idCol+`=? AND user_id=?`, itemID, userID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// RemoveUserFromAllVisible 用户被移出空间成员/删号时，清除其全部可见授权（空间删除级联由 FK 完成）。
func (s *Store) RemoveUserFromAllVisible(userID int64) error {
	for _, table := range []string{"space_training_visible", "space_practice_visible", "space_quiz_visible"} {
		if _, err := s.DB.Exec(`DELETE FROM `+table+` WHERE user_id=?`, userID); err != nil {
			return err
		}
	}
	return nil
}

func validVisibleTable(t string) bool {
	switch t {
	case "space_training_visible", "space_practice_visible", "space_quiz_visible":
		return true
	}
	return false
}
