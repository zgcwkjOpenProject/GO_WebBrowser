package main

import "os/exec"

const fixedFrontendDir = "frontend"

type serverConfig struct {
	ListenAddr        string `json:"listen"`
	AuthUser          string `json:"user"`
	BrowserDataDir    string `json:"data_dir"`
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
