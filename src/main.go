package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

// main 初始化配置、启动子进程并对外提供 HTTP 与 WebSocket 服务。
func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	a := &app{cfg: cfg}
	if err := a.start(); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
	defer a.stop()

	wsProxy := a.websocketProxyHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && isWebSocketUpgradeRequest(r) {
			wsProxy.ServeHTTP(w, r)
			return
		}
		a.handleIndex(w, r)
	})
	mux.Handle("/core/", http.StripPrefix("/core/", http.FileServer(http.Dir(filepath.Join(fixedFrontendDir, "core")))))
	mux.Handle("/vendor/", http.StripPrefix("/vendor/", http.FileServer(http.Dir(filepath.Join(fixedFrontendDir, "vendor")))))
	mux.HandleFunc("/api/runtime", a.handleRuntime)
	mux.Handle("/ws", wsProxy)

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: basicAuth(cfg.AuthUser, mux),
	}

	go func() {
		<-waitSignal()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("noVNC 页面: http://0.0.0.0%s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务异常: %v", err)
	}
}
