// Package app 组装「后端合服」后的单一服务进程：管理端 API + 门户/刷题 API + 判题队列
// 共用同一 Go 进程、同一 fiber.App、同一 sql.DB（orangeoj.db 单库）。
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"orangeoj/internal/accounts"
	"orangeoj/internal/judge"
	"orangeoj/internal/quizserver"
	"orangeoj/internal/quizstore"
	"orangeoj/internal/server"
	"orangeoj/internal/store"
)

// Config 合服进程配置。
type Config struct {
	DataDir string // 数据目录（orangeoj.db 与 uploads）
	// WebDist 学生门户前端 dist，挂载到 "/"（空=不托管静态）。
	WebDist string
	// WebAdminDist 管理端前端 dist，尽力而为挂载到 /admin 前缀
	// （Vite base 未按 /admin 配置时资源会 404——前端合并任务后移除，见任务说明）。
	WebAdminDist string
	// UploadsDir 上传图片目录；空时默认 <DataDir>/uploads。
	UploadsDir   string
	JudgeRunner  judge.Runner // 判题 runner（nil=禁用判题队列）
	JudgeWorkers int
}

// App 合服进程句柄（供退出前 StopQueue + 关库）。
type App struct {
	Store    *store.Store
	QS       *quizstore.Store
	Accounts *accounts.Store
	MainSrv  *server.Server
	QuizSrv  *quizserver.Server
	Fiber    *fiber.App
}

// Open 打开单库（store.Open 迁移题库侧表；同一连接补齐账号/判题/作答表）、
// 构造单一 Fiber app、挂载全部路由并（runner 非空时）启动判题队列。
// 返回 App；服务退出前调用 a.Close()（先 StopQueue 再关库）。
func Open(cfg Config) (*App, error) {
	uploadsDir := cfg.UploadsDir
	if uploadsDir == "" {
		uploadsDir = filepath.Join(cfg.DataDir, "uploads")
	}

	// 1) 主库打开（store schema：题库/域/空间结构表）。
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	// 2) 单连接：账号表 + 判题/作答表（幂等；题库侧表已由 store.Open 保证）。
	qs := quizstore.Wrap(st.DB)
	if err := qs.EnsureSchema(); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	acc := qs.Accounts

	a := &App{
		Store:    st,
		QS:       qs,
		Accounts: acc,
		MainSrv:  &server.Server{Store: st, Accounts: acc, UploadsDir: uploadsDir},
		QuizSrv:  &quizserver.Server{QS: qs, UploadsDir: uploadsDir},
	}

	app := server.NewApp()
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })

	// 3) 统一会话/auth 只挂一份（login 放行任意角色）：
	//    管理 API（/api/problems、/api/admin/* 等）→ requireAdmin；
	//    门户/刷题（/api/oj/*、/api/portal/*）→ 仅需有效 token。
	a.MainSrv.RegisterAuth(app)
	server.RegisterRoutes(st, acc, uploadsDir, app)
	a.QuizSrv.RegisterRoutes(app)

	// 4) 判题队列并入主进程（runner 非空才启动）。
	a.QuizSrv.StartQueue(cfg.JudgeRunner, cfg.JudgeWorkers)

	// 5) 前端静态（门户 /；管理端 /admin 尽力而为）。
	mountStatics(app, cfg.WebDist, cfg.WebAdminDist)

	a.Fiber = app
	return a, nil
}

// Close 停止判题队列并关闭数据库（先队列后关库，避免 worker 写已关连接）。
func (a *App) Close() error {
	a.QuizSrv.StopQueue()
	return a.Store.Close()
}

// mountStatics 静态托管：门户 dist 挂 "/"（SPA fallback）；
// 管理端 dist 若存在则尽力而为挂 /admin 前缀（其 index.html 经 SPA fallback 可达，
// 但 Vite base 未按 /admin 配置时资源路径会错位——留待前端合并任务处理）。
func mountStatics(app *fiber.App, webDist, webAdminDist string) {
	// 先挂 /admin（较具体），再挂门户 "/" 与全局 SPA 回退（较宽）。
	if webAdminDist != "" && hasIndex(webAdminDist) {
		app.Static("/admin", webAdminDist)
		app.Get("/admin/*", func(c *fiber.Ctx) error {
			return c.SendFile(filepath.Join(webAdminDist, "index.html"))
		})
	}
	if webDist != "" && hasIndex(webDist) {
		app.Static("/", webDist)
		app.Get("*", func(c *fiber.Ctx) error {
			return c.SendFile(filepath.Join(webDist, "index.html"))
		})
	}
}

func hasIndex(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil
}
