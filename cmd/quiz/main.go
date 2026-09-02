// Orange 刷题 — 独立端口刷题服务（与主站 OrangeRepo 共享题库）。
//
// 数据边界：只读打开 <data>/orangerepo.db（主站权威题库，绝不写入/迁移）；
// 自有数据（用户/科目/分类/错题/设置）写入 <data>/quiz.db。
package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"
	"time"

	"orangerepo/internal/bootstrap"
	"orangerepo/internal/judge"
	"orangerepo/internal/quizserver"
	"orangerepo/internal/quizstore"
)

func main() {
	addr := flag.String("addr", ":8081", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录（quiz.db 与上传图片）")
	webDist := flag.String("web", "./web-quiz/dist", "刷题前端构建产物目录")
	repoDB := flag.String("repo-db", "", "主站题库数据库路径（默认 <data>/orangerepo.db）")
	judgeEndpoint := flag.String("judge-endpoint", "", "judge-runtime 地址（默认 http://judge-runtime:9090；留空则禁用判题入队）")
	judgeToken := flag.String("judge-token", "", "与 judge-runtime 共享的评测 token（留空则禁用判题入队）")
	judgeWorkers := flag.Int("judge-workers", 2, "判题队列 worker 数")
	flag.Parse()

	// 容器以 root 启动时（绑定挂载宿主机目录的场景），先修正数据目录属主再降权到 65532。
	if err := bootstrap.DataDir(*dataDir); err != nil {
		log.Fatalf("[FATAL] 数据目录引导失败: %v", err)
	}

	repoPath := *repoDB
	if repoPath == "" {
		repoPath = filepath.Join(*dataDir, "orangerepo.db")
	}

	qs, err := quizstore.Open(*dataDir, repoPath)
	if err != nil {
		log.Fatalf("[FATAL] 刷题服务存储初始化失败: %v", err)
	}
	defer qs.Close()

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

	srv := &quizserver.Server{QS: qs, UploadsDir: filepath.Join(*dataDir, "uploads"), WebDist: *webDist}
	if srv.EnsureBootstrap() {
		log.Printf("[BOOTSTRAP] 已创建初始管理员 %s/%s，请登录后在「我的」页修改密码。", quizserver.BootstrapAdmin, quizserver.BootstrapPassword)
	}

	app := quizserver.New(srv, runner, *judgeWorkers)
	defer srv.StopQueue()
	log.Printf("[START] OrangeOJ 刷题服务监听 http://localhost%s （题库: %s，前端: %s）", *addr, repoPath, *webDist)
	if err := app.Listen(*addr); err != nil {
		log.Fatalf("[FATAL] 刷题服务退出: %v", err)
	}
}