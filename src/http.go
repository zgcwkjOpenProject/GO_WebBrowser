package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// handleIndex 返回 noVNC 前端入口页面。
func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	entry := filepath.Join(fixedFrontendDir, "index.html")
	if _, err := os.Stat(entry); err != nil {
		http.Error(w, fmt.Sprintf("前端文件不存在: %s", entry), http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, entry)
}

// handleRuntime 输出前端运行时连接参数。
func (a *app) handleRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	listenPort := extractPort(a.cfg.ListenAddr)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode":       "novnc",
		"vncWsPath":  "/ws",
		"vncWsPort":  listenPort,
		"listenAddr": a.cfg.ListenAddr,
	})
}

// websocketProxyHandler 将外部 /ws 连接反向代理到本地 websockify。
func (a *app) websocketProxyHandler() http.Handler {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", a.cfg.VNCWebsocketPort))
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "WebSocket 代理地址配置错误", http.StatusInternalServerError)
		})
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originDirector(req)
		req.URL.Path = "/"
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, fmt.Sprintf("WebSocket 代理失败: %v", e), http.StatusBadGateway)
	}

	return proxy
}

// basicAuth 为所有受保护路由提供基础认证校验。
func basicAuth(userSpec string, next http.Handler) http.Handler {
	parts := strings.SplitN(userSpec, ":", 2)
	username, password := "root", "root"
	if len(parts) == 2 {
		username = strings.TrimSpace(parts[0])
		password = strings.TrimSpace(parts[1])
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != username || p != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="remote-chromium"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isWebSocketUpgradeRequest 判断请求是否为 WebSocket 升级握手。
func isWebSocketUpgradeRequest(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	connection := strings.ToLower(r.Header.Get("Connection"))
	return strings.Contains(connection, "upgrade")
}

// extractPort 从监听地址中提取端口字符串。
func extractPort(listenAddr string) string {
	v := strings.TrimSpace(listenAddr)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, ":") {
		return strings.TrimPrefix(v, ":")
	}
	_, port, err := net.SplitHostPort(v)
	if err == nil {
		return port
	}
	return ""
}
