// judge-runtime HTTP 服务（迁移自上游 OrangeOJ backend/internal/judgeserver/server.go，
// 来源: https://github.com/shinyes/OrangeOJ）。
package judgeserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"orangerepo/internal/judge"
)

// Config judge-runtime 配置。
type Config struct {
	Port              string
	SharedToken       string
	WorkDir           string
	CompileTimeoutSec int
	ReadTimeoutSec    int
	WriteTimeoutSec   int
}

// Server judge-runtime HTTP 服务。
type Server struct {
	cfg      Config
	executor *Executor
	server   *http.Server
}

// NewServer 构造服务（校验 token/前置条件，初始化执行器）。
func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = "9090"
	}
	if strings.TrimSpace(cfg.SharedToken) == "" {
		return nil, fmt.Errorf("ORANGEOJ_JUDGE_SHARED_TOKEN is required")
	}
	if cfg.CompileTimeoutSec <= 0 {
		cfg.CompileTimeoutSec = 10
	}
	if cfg.ReadTimeoutSec <= 0 {
		cfg.ReadTimeoutSec = 15
	}
	if cfg.WriteTimeoutSec <= 0 {
		cfg.WriteTimeoutSec = 300
	}
	if strings.TrimSpace(cfg.WorkDir) == "" {
		cfg.WorkDir = "/work/jobs"
	}

	executor, err := NewExecutor(cfg.WorkDir, time.Duration(cfg.CompileTimeoutSec)*time.Second)
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: cfg, executor: executor}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/internal/judge/execute", s.handleExecute)

	s.server = &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.WriteTimeoutSec) * time.Second,
	}
	return s, nil
}

// Start 启动监听（阻塞）。
func (s *Server) Start() error {
	log.Printf("[judge-runtime] listening on :%s (backend: %s)", s.cfg.Port, s.executor.backend.Describe())
	for name, path := range s.executor.toolchains {
		status := path
		if path == "" {
			status = "MISSING"
		}
		log.Printf("[judge-runtime] toolchain %s: %s", name, status)
	}
	if s.executor.ToolchainMissing("g++") {
		log.Printf("[judge-runtime] WARNING: g++ 缺失——C++ 评测将全部返回 RE/CE（生产请用 nsjail 容器镜像）")
	}
	if s.executor.ToolchainMissing("python") {
		log.Printf("[judge-runtime] WARNING: python3 缺失——Python 评测将全部返回 RE")
	}
	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown 优雅停机。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-Judge-Token"))
	if token == "" || token != s.cfg.SharedToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var task judge.JudgeTask
	if err := json.Unmarshal(body, &task); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	result, err := s.executor.Execute(r.Context(), task)
	if err != nil {
		http.Error(w, fmt.Sprintf("execute failed: %v", err), http.StatusInternalServerError)
		return
	}
	respBytes, err := json.Marshal(result)
	if err != nil {
		http.Error(w, "marshal result failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}
