package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// start 按顺序启动显示环境、浏览器和 VNC 桥接进程。
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

// stop 按依赖反序停止相关子进程。
func (a *app) stop() {
	killProc(a.wsProxy)
	killProc(a.vncCmd)
	killProc(a.chromeCmd)
	killProc(a.xvfbCmd)
}

// startVNCBridge 启动 x11vnc 与 websockify，将 VNC 转为 WebSocket。
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

	wsArgs := []string{fmt.Sprintf("127.0.0.1:%d", a.cfg.VNCWebsocketPort), fmt.Sprintf("127.0.0.1:%d", a.cfg.VNCPort)}
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

// startXvfb 启动虚拟显示并设置 DISPLAY 环境变量。
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

// startChromium 使用指定参数启动 Chromium 进程。
func (a *app) startChromium() error {
	userDataDir := strings.TrimSpace(a.cfg.BrowserDataDir)
	if userDataDir == "" {
		var err error
		userDataDir, err = os.MkdirTemp("", "remote-chromium-profile-*")
		if err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(userDataDir, 0o755); err != nil {
			return fmt.Errorf("创建 Chromium 数据目录失败: %w", err)
		}
	}

	args := []string{
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
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Chromium 失败: %w", err)
	}
	a.chromeCmd = cmd
	return nil
}
