// Package app 组装「后端合服」后的单一服务进程：管理端 API + 门户/刷题 API + 判题队列
// 共用同一 Go 进程、同一 fiber.App、同一 sql.DB（orangeoj.db 单库）。
package app

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
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
	// WebDist 单前端构建产物目录（门户 / 与 管理区 /admin 均走前端路由），挂载到 "/"（空=不托管静态）。
	WebDist string
	// UploadsDir 上传图片目录；空时默认 <DataDir>/uploads。
	UploadsDir string
	// ScratchURL Scratch 编辑器容器地址（独立容器，前端以跨源 iframe 嵌入）。
	// 空 = 未部署：Scratch 空间页显示"未部署"提示。例：https://scratch.example.com
	ScratchURL string
	// ScratchInternalURL Scratch 容器的**内部**地址（如 http://orangescratch:80）。
	// 设置后主站在 /scratch-app/ 反向代理它：容器无需对外暴露端口，浏览器同源访问，
	// 也不必为它单独准备子域与证书。与 ScratchURL 同时设置时以本项为准（代理模式）。
	ScratchInternalURL string
	JudgeRunner        judge.Runner // 判题 runner（nil=禁用判题队列）
	JudgeWorkers       int
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

	// 0) Scratch 反代配置先校验（在打开数据库之前）：配置不合法时直接失败，
	//    避免"已打开库才发现配置错误"导致句柄泄漏（测试曾因此清理不掉临时目录）。
	scratchURL := strings.TrimRight(strings.TrimSpace(cfg.ScratchURL), "/")
	internal := strings.TrimRight(strings.TrimSpace(cfg.ScratchInternalURL), "/")
	var scratchTarget *url.URL
	var scratchHost string
	if internal != "" {
		target, err := url.Parse(internal)
		if err != nil || target.Host == "" {
			return nil, fmt.Errorf("scratch internal url 不合法: %q", internal)
		}
		if scratchURL == "" {
			return nil, fmt.Errorf("配置了 scratch 内部地址时必须同时给出公开地址（-scratch-url），" +
				"例如 https://scratch.example.com（主站按该域名反代）")
		}
		public, err := url.Parse(scratchURL)
		if err != nil || public.Host == "" {
			return nil, fmt.Errorf("scratch 公开地址不合法: %q", scratchURL)
		}
		scratchTarget = target
		scratchHost = hostOnly(public.Host)
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
		MainSrv:  &server.Server{Store: st, Accounts: acc, UploadsDir: uploadsDir, QuizStore: qs},
		QuizSrv:  &quizserver.Server{QS: qs, UploadsDir: uploadsDir, ScratchDir: filepath.Join(cfg.DataDir, "scratch")},
	}

	app := server.NewApp()
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })

	// 公开运行时配置（无需登录）：前端据此决定 Scratch 空间页是嵌 iframe 还是显示"未部署"。
	// （scratchURL / internal / scratchTarget 已在函数开头校验并解析，此处不再重复解析。）

	// Scratch 容器作为**内部容器**（不对外暴露端口）时：主站在它的**公开域名根路径**上反代它。
	//
	// 为什么必须是"域名根"而不是路径前缀（如 /scratch-app/）：
	// 上游 scratch-gui 的 standalone 构建把 webpack publicPath 硬编码为 "/"，运行期会以绝对路径取
	// 懒加载 chunk（/chunks/fetch-worker.*.js、/chunks/paper-source.*.js）、块素材
	// （/static/blocks-media/...）与教程图（/static/assets/...）。挂在路径前缀下时这些请求会打到
	// 主站域名根，被 SPA 兜底成 index.html（表现为 "Unexpected token '<'" 与素材 404）。
	// 因此：SCRATCH_URL 给公开地址（如 https://scratch.example.com）→ 主站按 Host 命中该地址的
	// 请求在根路径反代到容器；只配 SCRATCH_URL 不配内部地址时，则为"子域直连容器"模式（前端直接用该地址）。
	if scratchTarget != nil {
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(scratchTarget)         // scheme/host → 容器
				pr.Out.URL.Path = pr.In.URL.Path // 根路径原样转发（不剥前缀）
				pr.Out.URL.RawPath = pr.In.URL.RawPath
				pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, perr error) {
				// 容器未启动/不可达时给出可诊断提示（前端据此显示"未部署/不可用"）
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error":"Scratch 服务不可用：` + perr.Error() + `"}`))
			},
		}
		handler := adaptor.HTTPHandler(proxy)
		app.Use(func(c *fiber.Ctx) error {
			if !strings.EqualFold(hostOnly(c.Hostname()), scratchHost) {
				return c.Next() // 其他域名照常走主站
			}
			return handler(c)
		})
	}

	app.Get("/api/config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"scratchUrl": scratchURL,
			// 与宿主页 host.js 的协议版本；不一致时前端提示升级
			"scratchProtocol": 1,
		})
	})

	// 3) 统一会话/auth 只挂一份（login 放行任意角色）：
	//    管理 API（/api/problems、/api/admin/* 等）→ requireAdmin；
	//    门户/刷题（/api/oj/*、/api/portal/*）→ 仅需有效 token。
	a.MainSrv.RegisterAuth(app)
	server.RegisterRoutes(st, acc, uploadsDir, app)
	a.QuizSrv.RegisterRoutes(app)

	// 5) /api/* 未匹配兜底：返回 JSON 404（而非被 SPA 兜底吞成 200/405）。
	// 路径或方法写错时给出可诊断的错误，避免出现费解的 "Method Not Allowed"
	// （此前 SPA 的 app.Get("*") 会让任意 GET 路径"存在"，DELETE 打到同一路径即 405）。
	app.All("/api/*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "接口不存在：" + c.Method() + " " + c.Path(),
		})
	})

	// 4) 判题队列并入主进程（runner 非空才启动）。
	a.QuizSrv.StartQueue(cfg.JudgeRunner, cfg.JudgeWorkers)

	// 5) 前端静态（单应用 dist 挂 "/"；管理区 /admin 由前端路由处理，无需后端前缀托管）。
	mountStatics(app, cfg.WebDist)

	a.Fiber = app
	return a, nil
}

// Close 停止判题队列并关闭数据库（先队列后关库，避免 worker 写已关连接）。
func (a *App) Close() error {
	a.QuizSrv.StopQueue()
	return a.Store.Close()
}

// mountStatics 静态托管：单应用 dist 挂 "/"（SPA fallback——/admin 等前端路由刷新直达）。
func mountStatics(app *fiber.App, webDist string) {
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

// hostOnly 去掉主机名里的端口（Host 头可能带端口，比较时统一按主机名）。
func hostOnly(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		return host[:i]
	}
	return host
}
