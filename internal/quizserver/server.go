// Package quizserver 组装刷题服务 Fiber 应用：路由、会话认证、管理员鉴权、静态资源。
package quizserver

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"orangerepo/internal/quizstore"
)

// SessionCookie 会话 Cookie 名（与主站 orange_session 隔离）。
const SessionCookie = "quiz_session"

const userLocals = "quiz_user"

// Server 持有刷题服务存储与资源目录。
type Server struct {
	QS         *quizstore.Store
	UploadsDir string
	WebDist    string
}

// New 创建刷题服务 Fiber 应用（含路由与中间件）。
func New(qs *quizstore.Store, uploadsDir, webDist string) *fiber.App {
	srv := &Server{QS: qs, UploadsDir: uploadsDir, WebDist: webDist}
	app := fiber.New(fiber.Config{
		BodyLimit: 200 << 20,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if e, ok := err.(*fiber.Error); ok {
				return c.Status(e.Code).JSON(fiber.Map{"error": e.Message})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })

	auth := app.Group("/api/auth")
	auth.Post("/login", srv.handleLogin)
	auth.Post("/logout", srv.requireSession, srv.handleLogout)
	auth.Get("/me", srv.handleMe)
	auth.Put("/password", srv.requireSession, srv.handleChangePassword)

	quiz := app.Group("/api/quiz", srv.requireSession)
	quiz.Get("/subjects", srv.handleListSubjects)
	quiz.Post("/round", srv.handleStartRound)
	quiz.Post("/submit", srv.handleSubmit)
	quiz.Post("/wrong-round", srv.handleStartWrongRound)
	quiz.Get("/wrong-summary", srv.handleWrongSummary)

	admin := app.Group("/api/admin", srv.requireSession, srv.requireAdmin)
	admin.Get("/subjects", srv.handleAdminListSubjects)
	admin.Post("/subjects", srv.handleAdminCreateSubject)
	admin.Put("/subjects/order", srv.handleAdminSetSubjectOrder)
	admin.Patch("/subjects/:id", srv.handleAdminRenameSubject)
	admin.Delete("/subjects/:id", srv.handleAdminDeleteSubject)
	admin.Post("/categories", srv.handleAdminCreateCategory)
	admin.Patch("/categories/:id", srv.handleAdminUpdateCategory)
	admin.Delete("/categories/:id", srv.handleAdminDeleteCategory)
	admin.Put("/subjects/:id/categories/order", srv.handleAdminSetCategoryOrder)
	admin.Get("/problems-count", srv.handleAdminProblemsCount)
	admin.Get("/students", srv.handleAdminListStudents)
	admin.Post("/students", srv.handleAdminCreateStudent)
	admin.Put("/students/:id/password", srv.handleAdminResetStudentPassword)
	admin.Delete("/students/:id", srv.handleAdminDeleteStudent)
	admin.Get("/settings", srv.handleAdminGetSettings)
	admin.Put("/settings", srv.handleAdminPutSettings)

	// 上传图片与主站同路径约定（题面/解析中的 /api/uploads/... 可正常显示）
	if uploadsDir != "" {
		if _, err := os.Stat(uploadsDir); err == nil {
			app.Group("/api").Static("/uploads", uploadsDir)
		}
	}

	// 前端静态资源 + SPA 回退
	if webDist != "" {
		if _, err := os.Stat(filepath.Join(webDist, "index.html")); err == nil {
			app.Static("/", webDist)
			app.Get("*", func(c *fiber.Ctx) error {
				return c.SendFile(filepath.Join(webDist, "index.html"))
			})
		}
	}
	return app
}

// ---------- 中间件 ----------

// requireSession 会话校验：token 有效则注入当前用户。
func (s *Server) requireSession(c *fiber.Ctx) error {
	u, ok := s.QS.GetUserByToken(c.Cookies(SessionCookie))
	if !ok {
		return respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}
	c.Locals(userLocals, u)
	return c.Next()
}

// requireAdmin 管理员鉴权（须置于 requireSession 之后）。
func (s *Server) requireAdmin(c *fiber.Ctx) error {
	u := currentUser(c)
	if u == nil || u.Role != quizstore.RoleAdmin {
		return respondError(c, fiber.StatusForbidden, "需要管理员权限")
	}
	return c.Next()
}

func currentUser(c *fiber.Ctx) *quizstore.User {
	if v := c.Locals(userLocals); v != nil {
		return v.(*quizstore.User)
	}
	return nil
}

// ---------- 响应辅助 ----------

func respondData(c *fiber.Ctx, status int, data any) error {
	return c.Status(status).JSON(data)
}

func respondError(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// paramID 解析路径参数中的正整数 id。
func paramID(c *fiber.Ctx, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Params(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}