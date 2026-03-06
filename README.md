# Go + Chromium + Xvfb 远程浏览器（Linux + noVNC）

本项目用于在 **Linux 服务器** 启动 Chromium，并通过 noVNC 页面进行远程操作。

主要能力：

- 启动 Chromium（可配合 Xvfb）
- 通过 `x11vnc + websockify` 暴露 VNC WebSocket
- 内置 Web 页面（`src/frontend/index.html`）
- HTTP 入口 Basic Auth（`user` 配置，默认 `root:root`）

---

## 1. 当前目录结构（已按你的新结构更新）

```text
newChromium/
├─ README.md
├─ build.sh
├─ build.bat
├─ build/                      # 多平台编译产物
└─ src/
   ├─ main.go
   ├─ go.mod
   ├─ go.sum
   ├─ config.json
   └─ frontend/
      ├─ index.html
      ├─ LICENSE.txt
      └─ core/
         └─ ... noVNC 核心文件
```

> 注意：程序运行时的工作目录应为 `src/`，因为配置和前端都在该目录下。

---

## 2. 依赖

- Go 1.22+
- Chromium（默认 `chromium-browser`）
- Xvfb
- x11vnc
- websockify

Ubuntu/Debian：

```bash
sudo apt update
sudo apt install -y chromium-browser xvfb x11vnc websockify
```

---

## 3. 快速运行

在项目根目录执行：

```bash
cd src
go mod tidy
go run .
```

默认读取 `src/config.json`。

启动后访问：

```text
http://<服务器IP>:8080
```

默认账号密码（Basic Auth）：

- 用户名：`root`
- 密码：`root`

---

## 4. 配置文件（`src/config.json`）

当前示例：

```json
{
  "listen": ":8080",
  "user": "root:root",
  "lang": "en-US",
  "chromium": "chromium-browser",
  "xvfb_enable": true,
  "url": "https://google.com"
}
```

说明：

- `listen`：HTTP 监听地址
- `user`：Basic Auth，格式 `username:password`
- `lang`：浏览器语言（如 `zh-CN` / `en-US`）
- `chromium`：Chromium 可执行文件
- `xvfb_enable`：是否启用 Xvfb
- `url`：启动后打开的页面

---

## 5. 前端资源约定

程序固定从 `src/frontend` 读取页面资源：

- `src/frontend/index.html`
- `src/frontend/core/rfb.js`

并映射为：

- `/` -> `index.html`
- `/core/*` -> `src/frontend/core/*`
- `/vendor/*` -> `src/frontend/vendor/*`（若你后续补充 vendor）

---

## 6. 常用命令行参数

在 `src/` 下执行：

```bash
go run . -h
```

常用参数：

- `-config`：配置文件路径（默认 `config.json`）
- `-listen`：HTTP监听地址（默认 `:8080`）
- `-user`：账号密码（`username:password`）
- `-lang`：浏览器语言
- `-width` / `-height`：虚拟分辨率
- `-chromium` / `-xvfb` / `-x11vnc` / `-websockify`
- `-xvfb-enable`：是否启用 Xvfb
- `-vnc-port` / `-vnc-ws-port`
- `-url`：初始页面

---

## 7. 打包构建

### Linux/macOS

```bash
chmod +x ./build.sh
./build.sh
```

### Windows

```bat
build.bat
```

编译产物输出到根目录 `build/`。

---

## 8. 运行注意事项

1. 本项目 noVNC 模式仅支持 Linux 运行（依赖 X11 组件）。
2. 若页面可打开但连接失败，请检查：
   - `x11vnc`/`websockify` 是否安装
   - `6080` 端口是否可达
3. 外网暴露建议配合 HTTPS、反向代理和更强认证策略。


