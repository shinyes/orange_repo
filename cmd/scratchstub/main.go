// 临时静态服务：本地验证"Scratch 作为内部容器 + 主站反代"的链路。
// 用 app/dist/scratch-app（编辑器产物 + 宿主页 + 积木素材）模拟 orangescratch 容器。
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	dir := flag.String("dir", "app/dist/scratch-app", "静态目录")
	addr := flag.String("addr", "127.0.0.1:8081", "监听地址")
	flag.Parse()
	fs := http.FileServer(http.Dir(*dir))
	mux := http.NewServeMux()
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	}))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 与容器 nginx 一致的缓存头（用于确认反代是否透传）
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	}))
	log.Printf("scratch stub serving %s on %s", *dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
