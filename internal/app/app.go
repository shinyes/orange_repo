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
	// 与 ScratchURL（公开子域）同时设置时，主站按 Host 命中该子域并在**根路径**反代到容器：
	// 容器无需对外暴露端口，也无需为它单独准备证书。
	// 只配本项而缺 ScratchURL 时：**仅告警并忽略**（Scratch 视为未部署），不影响服务启动——
	// Scratch 是可选功能，配置不全不应导致整站起不来。
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

	// 0) Scratch 反代配置先解析（在打开数据库之前，避免配置错误时漏掉已打开的库句柄）。
	//
	// 两种可用形态（配置不全一律只告警、不拦启动——Scratch 是可选功能）：
	//   · 内部地址 + 公开子域 → 主站按 Host 命中该子域并在**根路径**反代容器（容器不暴露端口）
	//   · 只配内部地址       → 主站在**同源路径前缀** /scratch-app/ 反代容器（无需子域/DNS/证书）
	// 前缀模式成立的前提：镜像构建时已把上游写死的 webpack publicPath 改为运行期可注入
	// （见 scratch/Dockerfile；宿主页注入 window.__ORANGEOJ_PUBLIC_PATH__），否则 chunk 会去
	// 域名根取，被主站 SPA 兜底成 HTML（"Unexpected token '<'"）。
	scratchURL := strings.TrimRight(strings.TrimSpace(cfg.ScratchURL), "/")
	internal := strings.TrimRight(strings.TrimSpace(cfg.ScratchInternalURL), "/")
	var scratchTarget *url.URL
	var scratchHost string // 非空 = 子域根路径模式
	scratchPrefix := false // true = 同源前缀模式（/scratch-app）
	if internal != "" {
		target, err := url.Parse(internal)
		if err != nil || target.Host == "" {
			warnf("ORANGEOJ_SCRATCH_INTERNAL_URL 不合法（%q）：已忽略，Scratch 视为未部署", internal)
		} else {
			scratchTarget = target
			if scratchURL == "" {
				scratchPrefix = true
			} else if public, perr := url.Parse(scratchURL); perr != nil || public.Host == "" {
				warnf("ORANGEOJ_SCRATCH_URL 不合法（%q）：改用同源前缀模式提供 Scratch", scratchURL)
				scratchPrefix = true
			} else {
				scratchHost = hostOnly(public.Host)
			}
		}
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

	// Scratch 容器作为**内部容器**（不对外暴露端口）时，主站反代它。两种挂载方式：
	//   · 子域根路径（配了 ORANGEOJ_SCRATCH_URL）：按 Host 命中，路径原样转发。
	//     上游产物的路径都以 "/" 为基准，挂在域名根上最自然。
	//   · 同源前缀（只配 ORANGEOJ_SCRATCH_INTERNAL_URL）：挂在 /scratch-app/。
	//     这要求镜像里已把上游写死的 publicPath 改为运行期注入（Dockerfile 已做），
	//     宿主页据此把 chunk 请求指回前缀内，因此无需子域/DNS/证书。
	if scratchTarget != nil {
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(scratchTarget) // scheme/host → 容器
				path := pr.In.URL.Path
				if scratchPrefix {
					path = strings.TrimPrefix(path, "/scratch-app")
					if !strings.HasPrefix(path, "/") {
						path = "/" + path
					}
				}
				pr.Out.URL.Path = path
				pr.Out.URL.RawPath = ""
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
		if scratchPrefix {
			app.Use("/scratch-app", handler)
			scratchURL = "/scratch-app" // 前端同源 iframe 前缀
		} else {
			app.Use(func(c *fiber.Ctx) error {
				if !strings.EqualFold(hostOnly(c.Hostname()), scratchHost) {
					return c.Next() // 其他域名照常走主站
				}
				return handler(c)
			})
		}
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

// warnf 输出配置告警（不影响启动）。前缀与主程序日志风格一致，便于在 docker logs 里定位。
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}
