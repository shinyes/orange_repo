package quizserver

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/accounts"
)

// BootstrapAdmin / BootstrapPassword 首次启动的默认管理员账号。
// 与主站（初始密码 123456）惯例一致；登录后可在「我的」页修改。
const (
	BootstrapAdmin    = "admin"
	BootstrapPassword = "123456"
)

// EnsureBootstrap 保证存在管理员账号；返回是否执行了首次引导。
func (s *Server) EnsureBootstrap() bool {
	has, err := s.QS.Accounts.HasAdmin()
	if err != nil || has {
		return false
	}
	if _, err := s.QS.Accounts.CreateUser(BootstrapAdmin, BootstrapPassword, accounts.RoleAdmin); err != nil {
		log.Printf("[ERROR] 创建初始管理员失败: %v", err)
		return false
	}
	log.Printf("[BOOTSTRAP] 初始管理员 %q 密码 %q，请登录后在「我的」页修改。", BootstrapAdmin, BootstrapPassword)
	return true
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	u, err := s.QS.Accounts.CheckPassword(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		return respondError(c, fiber.StatusUnauthorized, "用户名或密码错误")
	}
	token, err := s.QS.Accounts.CreateSession(u.ID)
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   30 * 24 * 3600,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleLogout(c *fiber.Ctx) error {
	_ = s.QS.Accounts.DeleteSession(c.Cookies(SessionCookie))
	c.Cookie(&fiber.Cookie{Name: SessionCookie, Value: "", Path: "/", HTTPOnly: true, MaxAge: -1})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleMe(c *fiber.Ctx) error {
	u, ok := s.QS.Accounts.GetUserByToken(c.Cookies(SessionCookie))
	if !ok {
		return respondData(c, fiber.StatusOK, fiber.Map{"authenticated": false})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"authenticated": true, "user": u})
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// handleChangePassword 修改当前用户密码并轮换会话。
func (s *Server) handleChangePassword(c *fiber.Ctx) error {
	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil || req.NewPassword == "" || req.OldPassword == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	u := currentUser(c)
	if _, err := s.QS.Accounts.CheckPassword(u.Username, req.OldPassword); err != nil {
		return respondError(c, fiber.StatusUnauthorized, "原密码错误")
	}
	if err := s.QS.Accounts.SetPassword(u.ID, req.NewPassword); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	token, err := s.QS.Accounts.RotateSession(c.Cookies(SessionCookie), u.ID)
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name: SessionCookie, Value: token, Path: "/", HTTPOnly: true, SameSite: "Lax", MaxAge: 30 * 24 * 3600,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
