// Package quizserver 组装刷题服务 Fiber 应用：路由、会话认证、静态资源。
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

	"orangeoj/internal/accounts"
	"orangeoj/internal/judge"
	"orangeoj/internal/quizstore"
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

// StopQueue 停止队列 worker 并等待全部退出（服务退出前调用；须早于数据库关闭）。
func (s *Server) StopQueue() {
	if s.queueCancel != nil {
		s.queueCancel()
	}
	if s.queue != nil {
		s.queue.Stop()
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

	// ---- OrangeOJ：学生端做题（/api/oj：题目正文/判题/历史，可见性=空间模型） ----
	oj := app.Group("/api/oj", s.requireSession)
	oj.Get("/problem/:id", s.handleOJProblem)
	oj.Post("/problem/:id/run", s.handleOJRun)
	oj.Post("/problem/:id/test", s.handleOJTest)
	oj.Post("/problem/:id/submit", s.handleOJSubmit)
	oj.Post("/problem/:id/objective-submit", s.handleOJObjectiveSubmit)
	oj.Get("/problem/:id/submissions", s.handleOJSubmissions)
	oj.Get("/submission/:id/poll", s.handleOJSubmissionPoll)

	// ---- OrangeOJ 门户（空间化）：空间切换 / 训练 / 练习 / 刷题 / 排行榜 ----
	portal := app.Group("/api/portal", s.requireSession)
	portal.Get("/spaces", s.handlePortalSpaces)
	portal.Get("/space/:id/home", s.handlePortalSpaceHome)
	portal.Get("/space/:id/training/:tid", s.handlePortalTraining)
	portal.Post("/space/:id/training/:tid/answer", s.handlePortalTrainingAnswer)
	portal.Get("/space/:id/practice/:pid", s.handlePortalPractice)
	portal.Post("/space/:id/practice/:pid/submit", s.handlePortalPracticeSubmit)
	portal.Get("/space/:id/practice/:pid/submissions", s.handlePortalPracticeSubmissions)
	portal.Get("/space/:id/quizzes", s.handlePortalSpaceQuizzes)
	portal.Get("/quiz/:qid/problem", s.handlePortalQuizProblem)
	portal.Post("/quiz/:qid/answer", s.handlePortalQuizAnswer)
	portal.Get("/rank", s.handlePortalRank)

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

// isAdminRole 管理员角色（系统/域管理员）。
func isAdminRole(r accounts.Role) bool {
	return r == accounts.RoleGlobalAdmin || r == accounts.RoleDomainAdmin
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

// respondError 返回 *fiber.Error（不写响应）：由全局 ErrorHandler 统一输出 JSON。
// 返回非 nil 保证中间件/调用链正确中断。
func respondError(c *fiber.Ctx, status int, msg string) error {
	return fiber.NewError(status, msg)
}

// paramID 解析路径参数中的正整数 id。
func paramID(c *fiber.Ctx, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Params(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
