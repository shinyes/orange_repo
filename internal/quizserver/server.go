// Package quizserver 组装刷题服务 Fiber 应用：路由、会话认证、管理员鉴权、静态资源。
package quizserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"orangerepo/internal/accounts"
	"orangerepo/internal/judge"
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
	Runner     judge.Runner
	// queue 判题队列（Runner 配置时由 New 启动）。
	queue *judge.QueueService
	// queueCtx/queueCancel 队列生命周期。
	queueCtx    context.Context
	queueCancel context.CancelFunc
}

// Queue 返回判题队列服务（nil 表示未启用——judge token 未配置）。
func (s *Server) Queue() *judge.QueueService { return s.queue }

// StopQueue 停止队列 worker（服务退出前调用）。
func (s *Server) StopQueue() {
	if s.queueCancel != nil {
		s.queueCancel()
	}
}

// New 在 srv 上组装 Fiber 应用（路由/中间件/判题队列），并返回应用。
// runner 非空时启动判题队列 worker（workers<=0 用 1）；服务退出前调用 srv.StopQueue()。
func New(srv *Server, runner judge.Runner, workers int) *fiber.App {
	srv.Runner = runner
	if runner != nil {
		srv.queueCtx, srv.queueCancel = context.WithCancel(context.Background())
		srv.queue = judge.NewQueueService(srv.QS.DB, runner, srv.QS, workers)
		srv.queue.Start(srv.queueCtx)
	}
	return srv.buildApp()
}

// buildApp 组装路由（拆出以便测试构造裸 Server 时复用）。
func (s *Server) buildApp() *fiber.App {
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
	auth.Post("/login", s.handleLogin)
	auth.Post("/logout", s.requireSession, s.handleLogout)
	auth.Get("/me", s.handleMe)
	auth.Put("/password", s.requireSession, s.handleChangePassword)

	quiz := app.Group("/api/quiz", s.requireSession)
	quiz.Get("/subjects", s.handleListSubjects)
	quiz.Post("/round", s.handleStartRound)
	quiz.Post("/submit", s.handleSubmit)
	quiz.Post("/wrong-round", s.handleStartWrongRound)
	quiz.Get("/wrong-summary", s.handleWrongSummary)

	// ---- OrangeOJ：学生端做题（/api/oj） ----
	oj := app.Group("/api/oj", s.requireSession)
	oj.Get("/assigned", s.handleOJAssigned)
	oj.Get("/training/:id", s.handleOJTraining)
	oj.Get("/practice/:id", s.handleOJPractice)
	oj.Get("/problem/:id", s.handleOJProblem)
	oj.Post("/problem/:id/run", s.handleOJRun)
	oj.Post("/problem/:id/test", s.handleOJTest)
	oj.Post("/problem/:id/submit", s.handleOJSubmit)
	oj.Post("/problem/:id/objective-submit", s.handleOJObjectiveSubmit)
	oj.Get("/problem/:id/submissions", s.handleOJSubmissions)
	oj.Get("/submission/:id/poll", s.handleOJSubmissionPoll)

	admin := app.Group("/api/admin", s.requireSession, s.requireAdmin)
	admin.Get("/subjects", s.handleAdminListSubjects)
	admin.Post("/subjects", s.handleAdminCreateSubject)
	admin.Put("/subjects/order", s.handleAdminSetSubjectOrder)
	admin.Patch("/subjects/:id", s.handleAdminRenameSubject)
	admin.Delete("/subjects/:id", s.handleAdminDeleteSubject)
	admin.Post("/categories", s.handleAdminCreateCategory)
	admin.Patch("/categories/:id", s.handleAdminUpdateCategory)
	admin.Delete("/categories/:id", s.handleAdminDeleteCategory)
	admin.Put("/subjects/:id/categories/order", s.handleAdminSetCategoryOrder)
	admin.Get("/problems-count", s.handleAdminProblemsCount)
	admin.Get("/students", s.handleAdminListStudents)
	admin.Post("/students", s.handleAdminCreateStudent)
	admin.Put("/students/:id/password", s.handleAdminResetStudentPassword)
	admin.Delete("/students/:id", s.handleAdminDeleteStudent)
	admin.Get("/admins", s.handleAdminListAdmins)
	admin.Put("/admins/:id/password", s.handleAdminResetAdminPassword)
	admin.Get("/settings", s.handleAdminGetSettings)
	admin.Put("/settings", s.handleAdminPutSettings)

	// ---- OrangeOJ：管理员布置（/api/admin/assignments + repo 浏览） ----
	admin.Get("/repo-trainings", s.handleAdminRepoTrainings)
	admin.Get("/repo-practices", s.handleAdminRepoPractices)
	admin.Get("/repo-trainings/:id", s.handleAdminRepoTraining)
	admin.Get("/repo-practices/:id", s.handleAdminRepoPractice)
	admin.Get("/assignments", s.handleAdminListAssignments)
	admin.Post("/assignments", s.handleAdminCreateAssignment)
	admin.Patch("/assignments/:id", s.handleAdminUpdateAssignment)
	admin.Put("/assignments/:id/students", s.handleAdminSetAssignmentStudents)
	admin.Delete("/assignments/:id", s.handleAdminDeleteAssignment)
	admin.Get("/assignments/:id/students", s.handleAdminAssignmentStudents)
	admin.Get("/assignments/:id/stats", s.handleAdminAssignmentStats)

	// 上传图片与主站同路径约定（题面/解析中的 /api/uploads/... 可正常显示）
	if s.UploadsDir != "" {
		if _, err := os.Stat(s.UploadsDir); err == nil {
			app.Group("/api").Static("/uploads", s.UploadsDir)
		}
	}

	// 前端静态资源 + SPA 回退
	if s.WebDist != "" {
		if _, err := os.Stat(filepath.Join(s.WebDist, "index.html")); err == nil {
			app.Static("/", s.WebDist)
			app.Get("*", func(c *fiber.Ctx) error {
				return c.SendFile(filepath.Join(s.WebDist, "index.html"))
			})
		}
	}
	return app
}

// ---------- 中间件 ----------

// requireSession 会话校验：token 有效则注入当前用户。
func (s *Server) requireSession(c *fiber.Ctx) error {
	u, ok := s.QS.Accounts.GetUserByToken(c.Cookies(SessionCookie))
	if !ok {
		return respondError(c, fiber.StatusUnauthorized, "unauthorized")
	}
	c.Locals(userLocals, u)
	return c.Next()
}

// requireAdmin 管理员鉴权（须置于 requireSession 之后）。
func (s *Server) requireAdmin(c *fiber.Ctx) error {
	u := currentUser(c)
	if u == nil || u.Role != accounts.RoleAdmin {
		return respondError(c, fiber.StatusForbidden, "需要管理员权限")
	}
	return c.Next()
}

func currentUser(c *fiber.Ctx) *accounts.User {
	if v := c.Locals(userLocals); v != nil {
		return v.(*accounts.User)
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
