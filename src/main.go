package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const fixedFrontendDir = "frontend"

type serverConfig struct {
	ListenAddr        string `json:"listen"`
	AuthUser          string `json:"user"`
	BrowserLang       string `json:"lang"`
	DisplayWidth      int64  `json:"width"`
	DisplayHeight     int64  `json:"height"`
	ChromiumBin       string `json:"chromium"`
	XvfbBin           string `json:"xvfb"`
	X11VNCBin         string `json:"x11vnc"`
	WebsockifyBin     string `json:"websockify"`
	VNCPort           int    `json:"vnc_port"`
	VNCWebsocketPort  int    `json:"vnc_ws_port"`
	InitialURL        string `json:"url"`
	EnableXvfb        bool   `json:"xvfb_enable"`
	InsecureNoSandbox bool   `json:"no_sandbox"`
}

type app struct {
	cfg serverConfig

	chromeCmd *exec.Cmd
	xvfbCmd   *exec.Cmd
	vncCmd    *exec.Cmd
	wsProxy   *exec.Cmd
}

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

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.Handle("/core/", http.StripPrefix("/core/", http.FileServer(http.Dir(filepath.Join(fixedFrontendDir, "core")))))
	mux.Handle("/vendor/", http.StripPrefix("/vendor/", http.FileServer(http.Dir(filepath.Join(fixedFrontendDir, "vendor")))))
	mux.HandleFunc("/api/runtime", a.handleRuntime)

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

	log.Printf("noVNC 页面: http://0.0.0.0%s (websocket: %d)", cfg.ListenAddr, cfg.VNCWebsocketPort)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP 服务异常: %v", err)
	}
}

func parseFlags() (serverConfig, error) {
	cfg := defaultConfig()

	configPath := detectConfigPath(os.Args[1:])
	if err := mergeConfigFromFile(&cfg, configPath); err != nil {
		return cfg, err
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "HTTP 监听地址")
	fs.StringVar(&cfg.AuthUser, "user", cfg.AuthUser, "Basic Auth 账号密码，格式 username:password")
	fs.StringVar(&cfg.BrowserLang, "lang", cfg.BrowserLang, "Chromium 语言，如 zh-CN / en-US")
	fs.Int64Var(&cfg.DisplayWidth, "width", cfg.DisplayWidth, "虚拟屏幕宽")
	fs.Int64Var(&cfg.DisplayHeight, "height", cfg.DisplayHeight, "虚拟屏幕高")
	fs.StringVar(&cfg.ChromiumBin, "chromium", cfg.ChromiumBin, "Chromium 可执行文件")
	fs.StringVar(&cfg.XvfbBin, "xvfb", cfg.XvfbBin, "Xvfb 可执行文件")
	fs.StringVar(&cfg.X11VNCBin, "x11vnc", cfg.X11VNCBin, "x11vnc 可执行文件")
	fs.StringVar(&cfg.WebsockifyBin, "websockify", cfg.WebsockifyBin, "websockify 可执行文件")
	fs.IntVar(&cfg.VNCPort, "vnc-port", cfg.VNCPort, "VNC TCP 端口")
	fs.IntVar(&cfg.VNCWebsocketPort, "vnc-ws-port", cfg.VNCWebsocketPort, "VNC WebSocket 端口")
	fs.StringVar(&cfg.InitialURL, "url", cfg.InitialURL, "初始打开 URL")
	fs.BoolVar(&cfg.EnableXvfb, "xvfb-enable", cfg.EnableXvfb, "是否启用 Xvfb")
	fs.BoolVar(&cfg.InsecureNoSandbox, "no-sandbox", cfg.InsecureNoSandbox, "为容器环境追加 --no-sandbox")
	fs.StringVar(&configPath, "config", configPath, "配置文件路径（JSON）")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func defaultConfig() serverConfig {
	return serverConfig{
		ListenAddr:        ":8080",
		AuthUser:          "root:root",
		BrowserLang:       "zh-CN",
		DisplayWidth:      1366,
		DisplayHeight:     768,
		ChromiumBin:       "chromium-browser",
		XvfbBin:           "Xvfb",
		X11VNCBin:         "x11vnc",
		WebsockifyBin:     "websockify",
		VNCPort:           5900,
		VNCWebsocketPort:  6080,
		InitialURL:        "https://example.com",
		EnableXvfb:        true,
		InsecureNoSandbox: true,
	}
}

func detectConfigPath(args []string) string {
	path := "config.json"
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-config=") || strings.HasPrefix(a, "--config=") {
			parts := strings.SplitN(a, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				path = parts[1]
			}
			continue
		}
		if a == "-config" || a == "--config" {
			if i+1 < len(args) && strings.TrimSpace(args[i+1]) != "" {
				path = args[i+1]
				i++
			}
		}
	}
	return path
}

func mergeConfigFromFile(cfg *serverConfig, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("读取配置文件失败(%s): %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return fmt.Errorf("解析配置文件失败(%s): %w", path, err)
	}
	return nil
}

func validateConfig(cfg serverConfig) error {
	if strings.TrimSpace(cfg.AuthUser) == "" {
		return fmt.Errorf("user 不能为空，格式应为 username:password")
	}
	parts := strings.SplitN(cfg.AuthUser, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("user 格式非法，必须是 username:password")
	}
	if strings.TrimSpace(cfg.BrowserLang) == "" {
		return fmt.Errorf("lang 不能为空")
	}
	if cfg.DisplayWidth <= 0 || cfg.DisplayHeight <= 0 {
		return fmt.Errorf("width/height 必须大于 0")
	}
	if cfg.VNCPort <= 0 || cfg.VNCPort > 65535 {
		return fmt.Errorf("vnc_port 端口非法")
	}
	if cfg.VNCWebsocketPort <= 0 || cfg.VNCWebsocketPort > 65535 {
		return fmt.Errorf("vnc_ws_port 端口非法")
	}
	entry := filepath.Join(fixedFrontendDir, "index.html")
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("缺少前端入口文件: %s", entry)
	}
	rfb := filepath.Join(fixedFrontendDir, "core", "rfb.js")
	if _, err := os.Stat(rfb); err != nil {
		return fmt.Errorf("缺少 noVNC 核心资源: %s（请确认 noVNC core 已放到 frontend/core）", rfb)
	}
	return nil
}

func (a *app) start() error {
	if a.cfg.EnableXvfb {
		if err := a.startXvfb(); err != nil {
			return err
		}
	}
	if err := a.startChromium(); err != nil {
		return err
	}
	if err := a.startVNCBridge(); err != nil {
		return err
	}
	return nil
}

func (a *app) stop() {
	killProc(a.wsProxy)
	killProc(a.vncCmd)
	killProc(a.chromeCmd)
	killProc(a.xvfbCmd)
}

func (a *app) startVNCBridge() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("novnc 模式目前仅支持 Linux（依赖 X11 + x11vnc + websockify）")
	}

	display := os.Getenv("DISPLAY")
	if strings.TrimSpace(display) == "" {
		display = ":99"
	}

	vncArgs := []string{"-display", display, "-rfbport", fmt.Sprintf("%d", a.cfg.VNCPort), "-localhost", "-forever", "-shared", "-nopw", "-xkb"}
	vnc := exec.Command(a.cfg.X11VNCBin, vncArgs...)
	vnc.Stdout = os.Stdout
	vnc.Stderr = os.Stderr
	if err := vnc.Start(); err != nil {
		return fmt.Errorf("启动 x11vnc 失败: %w", err)
	}
	a.vncCmd = vnc

	time.Sleep(350 * time.Millisecond)

	wsArgs := []string{fmt.Sprintf("%d", a.cfg.VNCWebsocketPort), fmt.Sprintf("127.0.0.1:%d", a.cfg.VNCPort)}
	ws := exec.Command(a.cfg.WebsockifyBin, wsArgs...)
	ws.Stdout = os.Stdout
	ws.Stderr = os.Stderr
	if err := ws.Start(); err != nil {
		killProc(a.vncCmd)
		a.vncCmd = nil
		return fmt.Errorf("启动 websockify 失败: %w", err)
	}
	a.wsProxy = ws
	return nil
}

func (a *app) startXvfb() error {
	display := ":99"
	cmd := exec.Command(a.cfg.XvfbBin, display, "-screen", "0", fmt.Sprintf("%dx%dx24", a.cfg.DisplayWidth, a.cfg.DisplayHeight), "-ac")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Xvfb 失败: %w", err)
	}
	a.xvfbCmd = cmd
	os.Setenv("DISPLAY", display)
	time.Sleep(350 * time.Millisecond)
	return nil
}

func (a *app) startChromium() error {
	userDataDir, err := os.MkdirTemp("", "remote-chromium-profile-*")
	if err != nil {
		return err
	}

	if err := seedChromiumLocalePrefs(userDataDir, a.cfg.BrowserLang); err != nil {
		return fmt.Errorf("写入 Chromium 语言偏好失败: %w", err)
	}

	args := []string{
		fmt.Sprintf("--lang=%s", a.cfg.BrowserLang),
		fmt.Sprintf("--accept-lang=%s", a.cfg.BrowserLang),
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-background-networking",
		"--disable-features=Translate,BackForwardCache",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-position=0,0",
		fmt.Sprintf("--window-size=%d,%d", a.cfg.DisplayWidth, a.cfg.DisplayHeight),
		"--start-maximized",
		fmt.Sprintf("--user-data-dir=%s", filepath.ToSlash(userDataDir)),
		a.cfg.InitialURL,
	}
	if a.cfg.InsecureNoSandbox {
		args = append(args, "--no-sandbox")
	}

	cmd := exec.Command(a.cfg.ChromiumBin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("LANGUAGE=%s", a.cfg.BrowserLang),
		fmt.Sprintf("LANG=%s", normalizeLangEnv(a.cfg.BrowserLang)),
		fmt.Sprintf("LC_ALL=%s", normalizeLangEnv(a.cfg.BrowserLang)),
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Chromium 失败: %w", err)
	}
	a.chromeCmd = cmd
	return nil
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	entry := filepath.Join(fixedFrontendDir, "index.html")
	if _, err := os.Stat(entry); err != nil {
		http.Error(w, fmt.Sprintf("前端文件不存在: %s", entry), http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, entry)
}

func (a *app) handleRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode":       "novnc",
		"vncWsPort":  a.cfg.VNCWebsocketPort,
		"listenAddr": a.cfg.ListenAddr,
	})
}

func killProc(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	time.Sleep(150 * time.Millisecond)
	_ = cmd.Process.Kill()
}

func waitSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}

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

func normalizeLangEnv(lang string) string {
	v := strings.TrimSpace(lang)
	if v == "" {
		return "C.UTF-8"
	}
	v = strings.ReplaceAll(v, "-", "_")
	if !strings.Contains(v, ".") {
		v += ".UTF-8"
	}
	return v
}

func seedChromiumLocalePrefs(userDataDir, lang string) error {
	v := strings.TrimSpace(lang)
	if v == "" {
		return nil
	}

	defaultDir := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return err
	}

	localStatePath := filepath.Join(userDataDir, "Local State")
	localState := map[string]any{
		"intl": map[string]any{
			"app_locale": v,
		},
	}
	if err := writeJSONFile(localStatePath, localState); err != nil {
		return err
	}

	prefsPath := filepath.Join(defaultDir, "Preferences")
	prefs := map[string]any{
		"intl": map[string]any{
			"accept_languages": v,
		},
	}
	if err := writeJSONFile(prefsPath, prefs); err != nil {
		return err
	}

	return nil
}

func writeJSONFile(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
