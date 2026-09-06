// OrangeOJ — 后端合服后的单一服务进程。
//
// 单进程：管理端仓库 API + 门户/刷题 API + 判题队列共用同一 fiber.App 与
// 同一 orangeoj.db（单库）。前端已合为单应用（app/，管理区走前端路由 /admin），
// 构建产物由 -web 指向并挂载 "/"（SPA fallback，见 internal/app）。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orangeoj/internal/app"
	"orangeoj/internal/bootstrap"
	"orangeoj/internal/judge"
	"orangeoj/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录（SQLite 与上传图片）")
	webDist := flag.String("web", "./app/dist", "单前端构建产物目录（门户 / + 管理区 /admin 均走前端路由，挂载 /）")
	judgeEndpoint := flag.String("judge-endpoint", "", "judge-runtime 地址（默认 http://judge-runtime:9090；留空则禁用判题入队）")
	judgeToken := flag.String("judge-token", "", "与 judge-runtime 共享的评测 token（留空则禁用判题入队）")
	judgeWorkers := flag.Int("judge-workers", 2, "判题队列 worker 数")
	seed := flag.Bool("seed", false, "空库时导入 samples/orangeoj-sample.zip 示例数据")
	flag.Parse()

	// 容器以 root 启动时（绑定挂载宿主机目录的场景），先修正数据目录属主再降权到 65532。
	if err := bootstrap.DataDir(*dataDir); err != nil {
		log.Fatalf("[FATAL] 数据目录引导失败: %v", err)
	}

	var runner judge.Runner
	if strings.TrimSpace(*judgeToken) != "" {
		endpoint := *judgeEndpoint
		if endpoint == "" {
			endpoint = "http://judge-runtime:9090"
		}
		runner = judge.NewHTTPRunner(endpoint, *judgeToken, 5*time.Minute)
		log.Printf("[JUDGE] 判题队列已启用：runner=%s workers=%d", endpoint, *judgeWorkers)
	} else {
		log.Printf("[JUDGE] 未配置 -judge-token，判题功能禁用（run/test/submit 将返回 503）")
	}

	a, err := app.Open(app.Config{
		DataDir:      *dataDir,
		WebDist:      *webDist,
		JudgeRunner:  runner,
		JudgeWorkers: *judgeWorkers,
	})
	if err != nil {
		log.Fatalf("[FATAL] 服务初始化失败: %v", err)
	}
	defer a.Close()

	if a.MainSrv.EnsureBootstrap() {
		log.Printf("[BOOTSTRAP] 数据目录 %s 已初始化（初始管理员 %s/%s，请登录后修改）。", *dataDir, server.BootstrapAdmin, server.BootstrapPassword)
	}

	if *seed {
		if n, _ := a.Store.CountProblems(); n == 0 {
			sample := filepath.Join("samples", "orangeoj-sample.zip")
			if data, err := os.ReadFile(sample); err == nil {
				log.Printf("[SEED] 检测到空库，导入示例包 %s", sample)
				if _, err := a.MainSrv.ImportZipData(data, "training", "示例训练计划", nil, nil); err != nil {
					log.Printf("[WARN] 导入示例数据失败: %v", err)
				}
			}
		}
	}

	log.Printf("[START] OrangeOJ 监听 http://localhost%s （默认密码 123456，请登录后修改）", *addr)
	if err := a.Fiber.Listen(*addr); err != nil {
		log.Fatalf("[FATAL] 服务退出: %v", err)
	}
}
