// OrangeQuiz — 独立端口刷题服务（与主站 OrangeRepo 共享题库）。
//
// 数据边界：只读打开 <data>/orangerepo.db（主站权威题库，绝不写入/迁移）；
// 自有数据（用户/科目/分类/错题/设置）写入 <data>/quiz.db。
package main

import (
	"flag"
	"log"
	"path/filepath"

	"orangerepo/internal/bootstrap"
	"orangerepo/internal/quizserver"
	"orangerepo/internal/quizstore"
)

func main() {
	addr := flag.String("addr", ":8081", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录（quiz.db 与上传图片）")
	webDist := flag.String("web", "./web-quiz/dist", "刷题前端构建产物目录")
	repoDB := flag.String("repo-db", "", "主站题库数据库路径（默认 <data>/orangerepo.db）")
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

	srv := &quizserver.Server{QS: qs, UploadsDir: filepath.Join(*dataDir, "uploads"), WebDist: *webDist}
	if srv.EnsureBootstrap() {
		log.Printf("[BOOTSTRAP] 已创建初始管理员 %s/%s，请登录后在「我的」页修改密码。", quizserver.BootstrapAdmin, quizserver.BootstrapPassword)
	}

	app := quizserver.New(qs, srv.UploadsDir, srv.WebDist)
	log.Printf("[START] OrangeQuiz 刷题服务监听 http://localhost%s （题库: %s，前端: %s）", *addr, repoPath, *webDist)
	if err := app.Listen(*addr); err != nil {
		log.Fatalf("[FATAL] 刷题服务退出: %v", err)
	}
}