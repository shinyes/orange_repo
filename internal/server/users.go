// 仓库页用户管理（OJ 重构）：系统/域管理员在此维护空间成员账号
// （与 orangeoj.db 统一账号库联动；成员 role=member）。
package server

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
)

// handleListAllUsers GET /api/admin/all-users → 全账号列表（系统管理员专用；
// 含角色/归属域与域名，供集中用户管理页）。
func (s *Server) handleListAllUsers(c *fiber.Ctx) error {
	users, err := s.Accounts.ListAllUsers()
	if err != nil {
		return err
	}
	// 域名映射（按需取用）
	domainName := map[int64]string{}
	domains, err := s.Store.ListDomains()
	if err == nil {
		for _, d := range domains {
			domainName[d.ID] = d.Name
		}
	}
	// 各域成员集合（按空间归属；供前端判断某账号是否属于该域）——
	// member 不持久化 users.domain_id，故以空间归属为准
	domainMembers := map[int64]map[int64]bool{}
	if rows, err := s.Store.DB.Query(`SELECT DISTINCT sp.domain_id, m.user_id
		FROM space_members m JOIN spaces sp ON sp.id = m.space_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var did, uid int64
			if err := rows.Scan(&did, &uid); err != nil {
				break
			}
			if domainMembers[did] == nil {
				domainMembers[did] = map[int64]bool{}
			}
			domainMembers[did][uid] = true
		}
	}
	type view struct {
		ID         int64   `json:"id"`
		Username   string  `json:"username"`
		Role       string  `json:"role"`
		DomainID   *int64  `json:"domainId,omitempty"`
		DomainName *string `json:"domainName,omitempty"`
		// 该账号是否属于某个域（域管理员判定可改名范围用）
		DomainIDs []int64 `json:"domainIds,omitempty"`
	}
	out := make([]view, 0, len(users))
	for _, u := range users {
		v := view{ID: u.ID, Username: u.Username, Role: string(u.Role), DomainID: u.DomainID}
		if u.DomainID != nil {
			if n, ok := domainName[*u.DomainID]; ok {
				v.DomainName = &n
			}
			v.DomainIDs = append(v.DomainIDs, *u.DomainID)
		}
		for did, members := range domainMembers {
			if members[u.ID] && (u.DomainID == nil || *u.DomainID != did) {
				v.DomainIDs = append(v.DomainIDs, did)
			}
		}
		out = append(out, v)
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"users": out})
}

// handleCreateUser POST /api/admin/users {username, password} → 新建空间成员（member）。
// 系统管理员或域管理员可用；域管理员只能建 member（不能建管理员）。
func (s *Server) handleCreateUser(c *fiber.Ctx) error {
	user := currentUser(c)
	if user == nil || !isAdminRole(user.Role) {
		return respondError(c, fiber.StatusForbidden, "需要管理员权限")
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "用户名与密码必填")
	}
	id, err := s.Accounts.CreateUser(req.Username, req.Password, accounts.RoleMember)
	if err != nil {
		if err == accounts.ErrConflict {
			return respondError(c, fiber.StatusConflict, "用户名已存在")
		}
		if err.Error() == "用户名不合法" || strings.Contains(err.Error(), "1-32") {
			return respondError(c, fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return respondData(c, fiber.StatusCreated, fiber.Map{"id": id})
}

// handleListUsers GET /api/admin/users → 空间成员列表（含用户名，供拉入空间选择）。
func (s *Server) handleListUsers(c *fiber.Ctx) error {
	students, err := s.Accounts.ListStudents()
	if err != nil {
		return err
	}
	type view struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	out := make([]view, 0, len(students))
	for _, st := range students {
		out = append(out, view{ID: st.ID, Username: st.Username})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"users": out})
}

// handleDeleteUser DELETE /api/admin/users/:id → 删除空间成员（仅 member）。
func (s *Server) handleDeleteUser(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	if err := s.Accounts.DeleteStudent(id); err != nil {
		if err == accounts.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "用户不存在或非成员")
		}
		return err
	}
	// 从全部空间移除该成员
	if err := s.Store.RemoveUserFromAllSpaces(id); err != nil {
		return err
	}
	// 清除其在空间内容（训练/练习/刷题）可见名单中的授权
	if err := s.Store.RemoveUserFromAllVisible(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleRenameUser PUT /api/admin/users/:id/username {username} → 修改用户名。
// 权限：系统管理员可改任意账号；域管理员仅可改其域内的普通成员。
func (s *Server) handleRenameUser(c *fiber.Ctx) error {
	operator := currentUser(c)
	if operator == nil || !isAdminRole(operator.Role) {
		return respondError(c, fiber.StatusForbidden, "需要管理员权限")
	}
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := accounts.ValidateUsername(req.Username); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	target, err := s.Accounts.GetUserByID(id)
	if err != nil {
		if err == accounts.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "用户不存在")
		}
		return err
	}
	// 域管理员仅限本域成员。注意：member 账号不持久化 users.domain_id
	// （该列语义是「域管理员归属域」），因此「本域成员」按空间归属判定——
	// 与 requireSpaceAccess 的域口径一致：该用户属于本域任一空间即视为本域成员。
	if operator.Role == accounts.RoleDomainAdmin {
		if operator.DomainID == nil {
			return respondError(c, fiber.StatusForbidden, "当前账号未关联域，无法修改用户名")
		}
		if target.Role != accounts.RoleMember {
			return respondError(c, fiber.StatusForbidden, "域管理员仅可修改本域成员的用户名")
		}
		ok, err := s.userInDomain(id, *operator.DomainID)
		if err != nil {
			return err
		}
		if !ok {
			return respondError(c, fiber.StatusForbidden, "域管理员仅可修改本域成员的用户名")
		}
	}
	if err := s.Accounts.RenameUser(id, req.Username); err != nil {
		if err == accounts.ErrConflict {
			return respondError(c, fiber.StatusConflict, "用户名已存在")
		}
		if err == accounts.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "用户不存在")
		}
		if err.Error() == "用户名不合法" || strings.Contains(err.Error(), "32") {
			return respondError(c, fiber.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// userInDomain 判断用户是否属于某域（本域任一空间的成员即视为该域成员）。
// member 不持久化归属域，故按空间归属判定；域管理员/系统管理员按其 domain_id 判定。
func (s *Server) userInDomain(userID, domainID int64) (bool, error) {
	var n int
	err := s.Store.DB.QueryRow(`SELECT COUNT(1) FROM space_members m
		JOIN spaces sp ON sp.id = m.space_id
		WHERE m.user_id = ? AND sp.domain_id = ?`, userID, domainID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// handleResetUserPassword PUT /api/admin/users/:id/password {password}。
func (s *Server) handleResetUserPassword(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid id")
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	if err := s.Accounts.SetStudentPassword(id, req.Password); err != nil {
		if err == accounts.ErrNotFound {
			return respondError(c, fiber.StatusNotFound, "用户不存在或非成员")
		}
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
