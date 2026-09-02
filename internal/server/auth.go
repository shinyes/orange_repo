package server

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"

	"orangerepo/internal/accounts"
)

const (
	settingPasswordHash = "password_hash"
	settingSessionToken = "session_token"
)

// BootstrapAdmin / BootstrapPassword 首次启动的默认管理员账号。
const (
	BootstrapAdmin    = "admin"
	BootstrapPassword = "123456"
)

const userLocals = "main_user"

// requireSession 会话校验：令牌有效且为管理员（主站是管理工具，学生会话无权进入）。
func (s *Server) requireSession(c *fiber.Ctx) error {
	u, ok := s.Accounts.GetUserByToken(c.Cookies(SessionCookie))
	if !ok || u.Role != accounts.RoleAdmin {
		return respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}
	c.Locals(userLocals, u)
	return c.Next()
}

func currentUser(c *fiber.Ctx) *accounts.User {
	if v := c.Locals(userLocals); v != nil {
		return v.(*accounts.User)
	}
	return nil
}

// EnsureBootstrap 保证存在管理员账号（统一账号库）：
//
//  1. 旧版 settings.password_hash 迁移为 admin 账号（沿用旧 bcrypt 哈希，升级后原密码无缝可用），
//     随后清理旧 settings 键（旧会话随之失效，需重新登录一次）；
//  2. 账号库仍无管理员时创建默认 admin/123456。
//
// 返回是否执行了首次引导；两进程并发引导时以唯一约束容错（另一方创建即为已引导）。
func (s *Server) EnsureBootstrap() bool {
	has, err := s.Accounts.HasAdmin()
	if err != nil || has {
		return false
	}
	if hash, ok := s.Store.GetSetting(settingPasswordHash); ok && hash != "" {
		if _, err := s.Accounts.CreateAdminFromHash(BootstrapAdmin, hash); err == nil {
			_ = s.Store.SetSetting(settingPasswordHash, "")
			_ = s.Store.SetSetting(settingSessionToken, "")
			log.Printf("[BOOTSTRAP] 旧版管理员密码已迁移为账号 %q（沿用原密码），旧会话已失效。", BootstrapAdmin)
			return true
		}
	}
	if _, err := s.Accounts.CreateUser(BootstrapAdmin, BootstrapPassword, accounts.RoleAdmin); err == nil {
		log.Printf("[BOOTSTRAP] 初始管理员 %q 密码 %q，请登录后在设置中修改。", BootstrapAdmin, BootstrapPassword)
		return true
	}
	return false
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin 统一账号库登录：仅管理员可登录主站（主站是管理工具，学生账号不可用）。
func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	u, err := s.Accounts.CheckPassword(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		return respondError(c, fiber.StatusUnauthorized, "用户名或密码错误")
	}
	if u.Role != accounts.RoleAdmin {
		return respondError(c, fiber.StatusForbidden, "仅管理员可登录主站")
	}
	token, err := s.Accounts.CreateSession(u.ID)
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
	_ = s.Accounts.DeleteSession(c.Cookies(SessionCookie))
	c.Cookie(&fiber.Cookie{Name: SessionCookie, Value: "", Path: "/", HTTPOnly: true, MaxAge: -1})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleMe(c *fiber.Ctx) error {
	u, ok := s.Accounts.GetUserByToken(c.Cookies(SessionCookie))
	if !ok {
		return respondData(c, fiber.StatusOK, fiber.Map{"authenticated": false})
	}
	return respondData(c, fiber.StatusOK, fiber.Map{"authenticated": true, "user": u})
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// handleChangePassword 修改当前用户密码（统一账号：改密后该用户全端会话失效，本端自动重建）。
func (s *Server) handleChangePassword(c *fiber.Ctx) error {
	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil || req.NewPassword == "" || req.OldPassword == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	u := currentUser(c)
	if _, err := s.Accounts.CheckPassword(u.Username, req.OldPassword); err != nil {
		return respondError(c, fiber.StatusUnauthorized, "原密码错误")
	}
	if err := s.Accounts.SetPassword(u.ID, req.NewPassword); err != nil {
		return respondError(c, fiber.StatusBadRequest, err.Error())
	}
	token, err := s.Accounts.CreateSession(u.ID)
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name: SessionCookie, Value: token, Path: "/", HTTPOnly: true, SameSite: "Lax", MaxAge: 30 * 24 * 3600,
	})
	return c.SendStatus(fiber.StatusNoContent)
}