// 书包（Scratch 工程库）：文件夹树 + 工程元数据。
// 文件本体（.sb3）由 server 层落盘到 <DataDir>/scratch/<user_id>/<uuid>.sb3，本层只管元数据与权限归属。
// 设计要点：
//   - 一切都按 user_id 归属：所有查询/改动都带 user_id 条件，越权访问返回 ErrNotFound（不泄露存在性）
//   - 删除文件夹：其中的工程回到根目录（不连带删除用户作品），子文件夹一并删除（其工程同样回到根目录）
//   - 配额：单文件上限 MaxScratchProjectBytes，每人总量上限 ScratchUserQuotaBytes
package quizstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 书包容量限制。
const (
	// MaxScratchProjectBytes 单个工程（.sb3）大小上限：20MiB。
	MaxScratchProjectBytes = 20 << 20
	// ScratchUserQuotaBytes 每人书包总容量上限：200MiB。
	ScratchUserQuotaBytes = 200 << 20
	// MaxScratchFolders 每人文件夹数量上限（防滥用）。
	MaxScratchFolders = 200
)

// ScratchFolder 书包文件夹。
type ScratchFolder struct {
	ID        int64     `json:"id"`
	ParentID  *int64    `json:"parentId,omitempty"`
	Name      string    `json:"name"`
	OrderNo   int       `json:"orderNo"`
	CreatedAt time.Time `json:"createdAt"`
}

// ScratchProject 书包中的 Scratch 工程（元数据；文件按 UUID 取）。
type ScratchProject struct {
	ID        int64     `json:"id"`
	UUID      string    `json:"uuid"`
	FolderID  *int64    `json:"folderId,omitempty"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListScratchFolders 我的全部文件夹（前端自行组树）。
func (s *Store) ListScratchFolders(userID int64) ([]ScratchFolder, error) {
	rows, err := s.DB.Query(`SELECT id,parent_id,name,order_no,created_at FROM scratch_folders
		WHERE user_id=? ORDER BY order_no, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScratchFolder{}
	for rows.Next() {
		var f ScratchFolder
		var parent sql.NullInt64
		var raw string
		if err := rows.Scan(&f.ID, &parent, &f.Name, &f.OrderNo, &raw); err != nil {
			return nil, err
		}
		if parent.Valid {
			id := parent.Int64
			f.ParentID = &id
		}
		f.CreatedAt = parsePracticeTime(raw)
		out = append(out, f)
	}
	return out, rows.Err()
}

// CreateScratchFolder 新建文件夹（parentID 为空=根）。
func (s *Store) CreateScratchFolder(userID int64, name string, parentID *int64) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("文件夹名不能为空")
	}
	if len([]rune(name)) > 60 {
		return 0, errors.New("文件夹名过长（最多 60 字）")
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM scratch_folders WHERE user_id=?`, userID).Scan(&n); err != nil {
		return 0, err
	}
	if n >= MaxScratchFolders {
		return 0, fmt.Errorf("文件夹数量已达上限（%d）", MaxScratchFolders)
	}
	if parentID != nil {
		if err := s.ensureScratchFolder(userID, *parentID); err != nil {
			return 0, err
		}
	}
	var order int
	_ = s.DB.QueryRow(`SELECT COALESCE(MAX(order_no),0)+1 FROM scratch_folders
		WHERE user_id=? AND COALESCE(parent_id,0)=COALESCE(?,0)`, userID, parentID).Scan(&order)
	res, err := s.DB.Exec(`INSERT INTO scratch_folders(user_id,parent_id,name,order_no) VALUES(?,?,?,?)`,
		userID, parentID, name, order)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateScratchFolder 重命名/移动文件夹（nil 字段不改；parentID 传 0 表示移到根）。
func (s *Store) UpdateScratchFolder(userID, folderID int64, name *string, parentID *int64) error {
	if err := s.ensureScratchFolder(userID, folderID); err != nil {
		return err
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errors.New("文件夹名不能为空")
		}
		if len([]rune(n)) > 60 {
			return errors.New("文件夹名过长（最多 60 字）")
		}
		if _, err := s.DB.Exec(`UPDATE scratch_folders SET name=? WHERE id=? AND user_id=?`, n, folderID, userID); err != nil {
			return err
		}
	}
	if parentID != nil {
		pid := *parentID
		if pid == 0 {
			if _, err := s.DB.Exec(`UPDATE scratch_folders SET parent_id=NULL WHERE id=? AND user_id=?`, folderID, userID); err != nil {
				return err
			}
		} else {
			if pid == folderID {
				return errors.New("不能把文件夹移动到它自己里面")
			}
			if err := s.ensureScratchFolder(userID, pid); err != nil {
				return err
			}
			if isDescendant, err := s.scratchFolderIsDescendant(userID, folderID, pid); err != nil {
				return err
			} else if isDescendant {
				return errors.New("不能把文件夹移动到它的子文件夹里")
			}
			if _, err := s.DB.Exec(`UPDATE scratch_folders SET parent_id=? WHERE id=? AND user_id=?`, pid, folderID, userID); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteScratchFolder 删除文件夹：其中工程（含子文件夹内的）回到根目录，子文件夹删除。
func (s *Store) DeleteScratchFolder(userID, folderID int64) error {
	if err := s.ensureScratchFolder(userID, folderID); err != nil {
		return err
	}
	ids, err := s.scratchFolderSubtree(userID, folderID)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	// 工程回根目录（不删作品）
	if _, err := tx.Exec(`UPDATE scratch_projects SET folder_id=NULL, updated_at=CURRENT_TIMESTAMP
		WHERE user_id=? AND folder_id IN (`+ph+`)`, args...); err != nil {
		return err
	}
	// 子文件夹（parent 为自引用 FK，逐个删避免顺序问题）
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM scratch_folders WHERE id=? AND user_id=?`, id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListScratchProjects 我的工程；folderID 为 nil 时列全部，0 表示"根目录"。
func (s *Store) ListScratchProjects(userID int64, folderID *int64) ([]ScratchProject, error) {
	q := `SELECT id,uuid,folder_id,name,size,sha256,created_at,updated_at FROM scratch_projects WHERE user_id=?`
	args := []any{userID}
	if folderID != nil {
		if *folderID == 0 {
			q += ` AND folder_id IS NULL`
		} else {
			q += ` AND folder_id=?`
			args = append(args, *folderID)
		}
	}
	q += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScratchProject{}
	for rows.Next() {
		var p ScratchProject
		var folder sql.NullInt64
		var created, updated string
		if err := rows.Scan(&p.ID, &p.UUID, &folder, &p.Name, &p.Size, &p.SHA256, &created, &updated); err != nil {
			return nil, err
		}
		if folder.Valid {
			id := folder.Int64
			p.FolderID = &id
		}
		p.CreatedAt = parsePracticeTime(created)
		p.UpdatedAt = parsePracticeTime(updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetScratchProject 取我的某个工程（含 UUID，供落盘读取）。非本人 → ErrNotFound。
func (s *Store) GetScratchProject(userID, projectID int64) (*ScratchProject, error) {
	var p ScratchProject
	var folder sql.NullInt64
	var created, updated string
	err := s.DB.QueryRow(`SELECT id,uuid,folder_id,name,size,sha256,created_at,updated_at
		FROM scratch_projects WHERE id=? AND user_id=?`, projectID, userID).
		Scan(&p.ID, &p.UUID, &folder, &p.Name, &p.Size, &p.SHA256, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if folder.Valid {
		id := folder.Int64
		p.FolderID = &id
	}
	p.CreatedAt = parsePracticeTime(created)
	p.UpdatedAt = parsePracticeTime(updated)
	return &p, nil
}

// ScratchUsage 我的书包已用字节数。
func (s *Store) ScratchUsage(userID int64) (int64, error) {
	var n int64
	err := s.DB.QueryRow(`SELECT COALESCE(SUM(size),0) FROM scratch_projects WHERE user_id=?`, userID).Scan(&n)
	return n, err
}

// CreateScratchProject 记录一个新工程（文件已由 server 层写盘）。返回工程 id。
// size 超限或超出配额时返回错误（调用方负责删除已写入的文件）。
func (s *Store) CreateScratchProject(userID int64, uuid, name string, folderID *int64, size int64, sha256 string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未命名作品"
	}
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	if size <= 0 {
		return 0, errors.New("工程内容为空")
	}
	if size > MaxScratchProjectBytes {
		return 0, fmt.Errorf("工程过大（单个上限 %d MB）", MaxScratchProjectBytes>>20)
	}
	used, err := s.ScratchUsage(userID)
	if err != nil {
		return 0, err
	}
	if used+size > ScratchUserQuotaBytes {
		return 0, fmt.Errorf("书包空间不足（上限 %d MB）", ScratchUserQuotaBytes>>20)
	}
	if folderID != nil && *folderID > 0 {
		if err := s.ensureScratchFolder(userID, *folderID); err != nil {
			return 0, err
		}
	} else {
		folderID = nil
	}
	res, err := s.DB.Exec(`INSERT INTO scratch_projects(uuid,user_id,folder_id,name,size,sha256) VALUES(?,?,?,?,?,?)`,
		uuid, userID, folderID, name, size, sha256)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateScratchProjectContent 覆盖工程内容（实时暂存用：同一作品反复写入，不新增记录）。
// 大小仍受单文件上限与用户配额约束（配额按“替换后的总量”计算）。
func (s *Store) UpdateScratchProjectContent(userID, projectID int64, size int64, sha256 string) error {
	p, err := s.GetScratchProject(userID, projectID)
	if err != nil {
		return err
	}
	if size <= 0 {
		return errors.New("工程内容为空")
	}
	if size > MaxScratchProjectBytes {
		return fmt.Errorf("工程过大（单个上限 %d MB）", MaxScratchProjectBytes>>20)
	}
	used, err := s.ScratchUsage(userID)
	if err != nil {
		return err
	}
	if used-p.Size+size > ScratchUserQuotaBytes {
		return fmt.Errorf("书包空间不足（上限 %d MB）", ScratchUserQuotaBytes>>20)
	}
	_, err = s.DB.Exec(`UPDATE scratch_projects SET size=?, sha256=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`,
		size, sha256, projectID, userID)
	return err
}

// UpdateScratchProject 改名/移动（写回内容用 UpdateScratchProjectContent）。
func (s *Store) UpdateScratchProject(userID, projectID int64, name *string, folderID *int64) error {
	if _, err := s.GetScratchProject(userID, projectID); err != nil {
		return err
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errors.New("名称不能为空")
		}
		if len([]rune(n)) > 120 {
			return errors.New("名称过长（最多 120 字）")
		}
		if _, err := s.DB.Exec(`UPDATE scratch_projects SET name=?, updated_at=CURRENT_TIMESTAMP
			WHERE id=? AND user_id=?`, n, projectID, userID); err != nil {
			return err
		}
	}
	if folderID != nil {
		if *folderID > 0 {
			if err := s.ensureScratchFolder(userID, *folderID); err != nil {
				return err
			}
			if _, err := s.DB.Exec(`UPDATE scratch_projects SET folder_id=?, updated_at=CURRENT_TIMESTAMP
				WHERE id=? AND user_id=?`, *folderID, projectID, userID); err != nil {
				return err
			}
		} else {
			if _, err := s.DB.Exec(`UPDATE scratch_projects SET folder_id=NULL, updated_at=CURRENT_TIMESTAMP
				WHERE id=? AND user_id=?`, projectID, userID); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteScratchProject 删除工程元数据（返回 UUID 供调用方删文件）。
func (s *Store) DeleteScratchProject(userID, projectID int64) (string, error) {
	p, err := s.GetScratchProject(userID, projectID)
	if err != nil {
		return "", err
	}
	if _, err := s.DB.Exec(`DELETE FROM scratch_projects WHERE id=? AND user_id=?`, projectID, userID); err != nil {
		return "", err
	}
	return p.UUID, nil
}

// ---------- 内部辅助 ----------

// ensureScratchFolder 该文件夹属于该用户，否则 ErrNotFound（不区分"不存在"与"不是你的"）。
func (s *Store) ensureScratchFolder(userID, folderID int64) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM scratch_folders WHERE id=? AND user_id=?`, folderID, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scratchFolderSubtree 返回该文件夹及其全部后代的 id（含自身）。
func (s *Store) scratchFolderSubtree(userID, folderID int64) ([]int64, error) {
	out := []int64{folderID}
	frontier := []int64{folderID}
	for len(frontier) > 0 {
		var next []int64
		for _, id := range frontier {
			rows, err := s.DB.Query(`SELECT id FROM scratch_folders WHERE user_id=? AND parent_id=?`, userID, id)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var child int64
				if err := rows.Scan(&child); err != nil {
					rows.Close()
					return nil, err
				}
				next = append(next, child)
				out = append(out, child)
			}
			rows.Close()
		}
		frontier = next
	}
	return out, nil
}

// scratchFolderIsDescendant 判断 maybeDesc 是否位于 folderID 的子树内（防把父目录移进自己的子孙）。
func (s *Store) scratchFolderIsDescendant(userID, folderID, maybeDesc int64) (bool, error) {
	cur := maybeDesc
	for i := 0; i < 100; i++ { // 深度上限，防环
		var parent sql.NullInt64
		err := s.DB.QueryRow(`SELECT parent_id FROM scratch_folders WHERE id=? AND user_id=?`, cur, userID).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !parent.Valid {
			return false, nil
		}
		if parent.Int64 == folderID {
			return true, nil
		}
		cur = parent.Int64
	}
	return false, nil
}
