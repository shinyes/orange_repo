package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

const (
	settingPasswordHash = "password_hash"
	settingSessionToken = "session_token"
)

// BootstrapPassword 首次启动的默认密码。
const BootstrapPassword = "123456"

// EnsureBootstrap 保证存在密码哈希；返回是否执行了首次引导。
func (s *Server) EnsureBootstrap() bool {
	if _, ok := s.Store.GetSetting(settingPasswordHash); ok {
		return false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(BootstrapPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] bootstrap bcrypt: %v", err)
		return false
	}
	_ = s.Store.SetSetting(settingPasswordHash, string(hash))
	log.Printf("[BOOTSTRAP] 初始管理员密码为 %q，请登录后尽快在「设置」中修改。", BootstrapPassword)
	return true
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// requireSession 会话校验中间件。
func (s *Server) requireSession(c *fiber.Ctx) error {
	token := c.Cookies(SessionCookie)
	want, ok := s.Store.GetSetting(settingSessionToken)
	if !ok || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}
	return c.Next()
}

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil || req.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	hash, ok := s.Store.GetSetting(settingPasswordHash)
	if !ok {
		return respondError(c, fiber.StatusInternalServerError, "not initialized")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return respondError(c, fiber.StatusUnauthorized, "wrong password")
	}
	token := newToken()
	if err := s.Store.SetSetting(settingSessionToken, token); err != nil {
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
	_ = s.Store.SetSetting(settingSessionToken, "")
	c.Cookie(&fiber.Cookie{Name: SessionCookie, Value: "", Path: "/", HTTPOnly: true, MaxAge: -1})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleMe(c *fiber.Ctx) error {
	token := c.Cookies(SessionCookie)
	want, ok := s.Store.GetSetting(settingSessionToken)
	authed := ok && token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
	return respondData(c, fiber.StatusOK, fiber.Map{"authenticated": authed})
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

func (s *Server) handleChangePassword(c *fiber.Ctx) error {
	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil || req.NewPassword == "" {
		return respondError(c, fiber.StatusBadRequest, "invalid request")
	}
	hash, ok := s.Store.GetSetting(settingPasswordHash)
	if !ok {
		return respondError(c, fiber.StatusInternalServerError, "not initialized")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		return respondError(c, fiber.StatusUnauthorized, "wrong password")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.Store.SetSetting(settingPasswordHash, string(newHash)); err != nil {
		return err
	}
	// 轮换会话：所有旧会话失效
	token := newToken()
	if err := s.Store.SetSetting(settingSessionToken, token); err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name: SessionCookie, Value: token, Path: "/", HTTPOnly: true, SameSite: "Lax", MaxAge: 30 * 24 * 3600,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
