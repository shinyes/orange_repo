// OrangeRepo — OrangeOJ 兼容的题库管理应用。
//
// 单进程：提供 /api REST 接口与 web/dist 静态资源。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"orangerepo/internal/server"
	"orangerepo/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录（SQLite 与上传图片）")
	webDist := flag.String("web", "./web/dist", "前端构建产物目录")
	seed := flag.Bool("seed", false, "空库时导入 samples/orangeoj-sample.zip 示例数据")
	flag.Parse()

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("[FATAL] 打开数据库失败: %v", err)
	}
	defer st.Close()

	srv := &server.Server{Store: st, UploadsDir: filepath.Join(*dataDir, "uploads"), WebDist: *webDist}
	if srv.EnsureBootstrap() {
		log.Printf("[BOOTSTRAP] 数据目录 %s 已初始化。", *dataDir)
	}

	if *seed {
		if n, _ := st.CountProblems(); n == 0 {
			sample := filepath.Join("samples", "orangeoj-sample.zip")
			if data, err := os.ReadFile(sample); err == nil {
				log.Printf("[SEED] 检测到空库，导入示例包 %s", sample)
				if _, err := srv.ImportZipData(data, "training"); err != nil {
					log.Printf("[WARN] 导入示例数据失败: %v", err)
				}
			}
		}
	}

	app := server.New(st, srv.UploadsDir, srv.WebDist)
	log.Printf("[START] OrangeRepo 监听 http://localhost%s （默认密码 123456，请登录后修改）", *addr)
	if err := app.Listen(*addr); err != nil {
		log.Fatalf("[FATAL] 服务退出: %v", err)
	}
}
