package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// killProc 尝试优雅结束进程，超时后强制终止。
func killProc(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	time.Sleep(150 * time.Millisecond)
	_ = cmd.Process.Kill()
}

// waitSignal 监听退出信号并返回通知通道。
func waitSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}
