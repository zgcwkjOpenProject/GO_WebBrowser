package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseFlags 合并默认值、配置文件和命令行参数，返回最终配置。
func parseFlags() (serverConfig, error) {
	cfg := defaultConfig()

	configPath := detectConfigPath(os.Args[1:])
	if err := mergeConfigFromFile(&cfg, configPath); err != nil {
		return cfg, err
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "HTTP 监听地址")
	fs.StringVar(&cfg.AuthUser, "user", cfg.AuthUser, "Basic Auth 账号密码，格式 username:password")
	fs.StringVar(&cfg.BrowserDataDir, "data-dir", cfg.BrowserDataDir, "Chromium 用户数据目录（为空则使用临时目录）")
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

// defaultConfig 提供程序启动时的默认配置。
func defaultConfig() serverConfig {
	return serverConfig{
		ListenAddr:        ":8080",
		AuthUser:          "root:root",
		BrowserDataDir:    "",
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

// detectConfigPath 从命令行参数中提取配置文件路径。
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

// mergeConfigFromFile 读取 JSON 配置并覆盖到现有配置对象。
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

// validateConfig 校验关键配置与前端资源文件是否可用。
func validateConfig(cfg serverConfig) error {
	if strings.TrimSpace(cfg.AuthUser) == "" {
		return fmt.Errorf("user 不能为空，格式应为 username:password")
	}
	parts := strings.SplitN(cfg.AuthUser, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("user 格式非法，必须是 username:password")
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
