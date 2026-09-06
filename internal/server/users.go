// 仓库页用户管理（OJ 重构）：系统/域管理员在此维护空间成员账号
// （与 orangeoj.db 统一账号库联动；成员 role=member）。
package server

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangeoj/internal/accounts"
)

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
	return c.SendStatus(fiber.StatusNoContent)
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
