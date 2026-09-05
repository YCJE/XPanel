# XPanel

个人自用的代理 + 转发一体化面板：以 [3x-ui](https://github.com/MHSanaei/3x-ui) 为底座精简而来的 **Xray 协议代理管理**，加上与 [flux-panel（哆啦A梦转发面板）](https://github.com/bqlpfy/flux-panel) agent 协议完全兼容的 **gost 中转/端口转发管理**。

一个面板管完「落地机 Xray 节点 + 中转机 gost 转发」，中文界面、单管理员、零外部依赖（Go 单二进制 + SQLite）。

## 功能

### 代理（来自 3x-ui 精简版）
- Xray-core 进程管理、入站管理（VLESS / VMess / Trojan / Shadowsocks / Hysteria / WireGuard 等）
- 传输层：TCP / WS / gRPC / HTTPUpgrade / XHTTP / KCP，TLS / Reality
- Xray 高级设置：路由规则、出站、DNS、负载均衡
- 多客户端（每台设备一个 client）+ 订阅链接（Base64 / Clash / JSON），多设备共用
- 每客户端流量统计与展示、在线设备、二维码、流量重置
- 系统监控（CPU / 内存 / 网络）、面板证书、备份恢复、2FA 登录

### 转发（新增 gost 模块，兼容 flux-panel agent）
- **节点管理**：中转机运行 gost agent，WebSocket 注册上线，实时上报 CPU / 内存 / 网络
- **端口转发**：单节点 TCP+UDP 端口映射，支持多目标负载均衡（fifo / round / hash）
- **隧道转发**：入口节点 →（ws / tls / quic 隧道）→ 出口节点 → 最终目标
- 每条规则：流量倍率、限速（Mbps）、启用开关、流量统计（上下行 / 累计）
- 节点上线自动同步配置，无需手动干预
- agent 兼容 flux-panel 面板的通信协议（AES-GCM 加密）

### 相比 3x-ui 移除的内容
多语言界面（仅保留中文）、Telegram Bot、Cloudflare WARP、NordVPN 导入、LDAP 登录、
客户端到期/限流/IP 限制/批量创建（多租户功能）、面板在线更新（避免覆盖本 fork）、
波斯历等区域资产。

## 架构

```
 客户端设备                    中转节点                        落地节点
 (v2rayNG 等)                (gost agent)                  (XPanel + Xray)
     │                            │                              │
     │      代理连接               │   隧道转发 (ws/tls/quic)      │
     ├──────────────────────────► 入口端口 ────────────────────► Xray 入站端口
     │        （也可直连落地）       │        gost relay            ▲
     │                            │                              │
     │                            │  agent WebSocket /system-info│
     │                            │  流量上报  POST /flow/upload │
     │                            └──────────────┬───────────────┤
     │                                           └──── ► 面板「转发管理」
```

- 落地机：运行 XPanel（内含 Xray 管理 + agent 通信端口，默认 Web `2053`）
- 中转机：运行 gost agent，在面板「转发管理 → 节点管理」生成一键安装命令

## 部署

### 1. 落地机安装面板

**方式 A：一键脚本（需要仓库已发布 Release）**

```bash
bash <(curl -Ls https://raw.githubusercontent.com/YCJE/XPanel/main/install.sh)
```

**方式 B：从源码构建（无 Release 时）**

```bash
git clone https://github.com/YCJE/XPanel.git
cd XPanel
go build -trimpath -ldflags "-s -w" -o xpanel .
sudo mkdir -p /usr/local/xpanel/bin
sudo mv xpanel /usr/local/xpanel/
sudo cp xpanel.sh /usr/local/xpanel/
# 安装 Xray-core 到 /usr/local/xpanel/bin/，或直接用 xpanel.sh 菜单里的「安装 Xray」
sudo ./xpanel.sh   # 管理菜单（注册 systemd 服务等）
```

**方式 C：Docker**

```bash
docker compose up -d
```

首次登录：用户名 `admin` / 密码 `admin`，登录后请立即在「面板设置」中修改，并配置证书与路径。

### 2. 中转机安装 agent

在落地机面板「转发管理 → 节点管理 → 添加节点」，然后点击「安装命令」，在中转机执行生成的命令：

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh | \
  bash -s -- "<面板地址:端口>" "<节点Token>"
```

安装完成后节点显示「在线」。没有 Release 二进制时，可自行构建 agent，并以第三个参数传入：

```bash
bash agent-install.sh "<面板地址:端口>" "<节点Token>" /path/to/gost
```

### 3. 建立转发

- **端口转发**：中转机端口 → 任意目标（如落地机 `IP:443`）
- **隧道转发**：入口节点端口 → 出口节点（ws/tls/quic）→ 最终目标（如落地机 `127.0.0.1:443`）

## 发布流程

推送 `v*` 标签后，GitHub Actions 自动构建并附加到 Release：

- `xpanel-linux-amd64.tar.gz` / `xpanel-linux-arm64.tar.gz`（面板，含 xray 二进制与 geo 文件）
- `gost-linux-amd64.gz` / `gost-linux-arm64.gz`（中转 agent）

```bash
git tag v1.0.0 && git push origin v1.0.0
```

## 开发

```bash
go build ./...      # 编译
go test ./...       # 测试（含 agent 协议端到端测试）
```

- 后端：Go + Gin + GORM(SQLite) + gorilla/websocket
- 前端：Vue 2 + Ant Design Vue（服务端模板渲染，与 3x-ui 相同）

调试面板与节点的通信协议时，可以在本机运行 agent 模拟器：

```bash
go run ./tools/agent-sim -addr 127.0.0.1:2053 -secret <节点Token>
```

## 目录说明

```
├── main.go / config/ / database/ / xray/ / sub/ / web/   # 面板（3x-ui 精简版）
├── web/service/forward/   # gost 转发模块（agent 通信 + 规则下发 + 流量入账）
├── web/html/forward.html  # 「转发管理」页面
├── agent/                 # gost 中转 agent（兼容 flux-panel 协议）
├── agent-install.sh       # 中转节点一键安装脚本
└── install.sh / xpanel.sh # 面板安装与管理脚本
```

## 许可与致谢

- 面板基于 [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）精简修改，见 [LICENSE](LICENSE)
- 转发模块协议与 agent 兼容 [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0），
  agent 源码基于 [go-gost](https://github.com/go-gost/gost)（MIT），见 [agent/README.md](agent/README.md)

仅供个人学习与自用，请遵守当地法律法规。
