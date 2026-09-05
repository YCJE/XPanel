# XPanel v1.0.0 — 首个正式版

个人自用的代理 + 转发一体化面板首发版本：以 [3x-ui](https://github.com/MHSanaei/3x-ui) 为底座精简的 **Xray 协议代理管理**，加上与 [flux-panel](https://github.com/bqlpfy/flux-panel) agent 协议完全兼容的 **gost 中转/端口转发**。中文界面、单管理员、Go 单二进制 + SQLite，零外部依赖。

## ✨ 功能亮点

### 代理管理（落地机）
- Xray-core 进程管理，入站协议：VLESS / VMess / Trojan / Shadowsocks / Hysteria / WireGuard 等
- 传输层：TCP / WS / gRPC / HTTPUpgrade / XHTTP / KCP，支持 TLS 与 Reality
- Xray 高级设置：路由规则、出站、DNS、负载均衡
- 多客户端（每台设备一个 client）+ 订阅链接（Base64 / Clash / JSON）
- 每客户端流量统计与在线状态、二维码分享、流量重置
- 系统实时监控、面板证书管理、备份恢复、2FA 两步验证、登录失败限流

### 转发管理（中转机，gost）
- 节点管理：gost agent 一键安装、WebSocket 注册上线、CPU/内存/网络实时上报
- 端口转发：单节点 TCP+UDP 映射，多目标负载均衡（fifo / round / hash）
- 隧道转发：入口节点 →（ws / tls / quic 加密隧道）→ 出口节点 → 最终目标
- 每条规则支持流量倍率、限速（Mbps）、启用开关与流量统计（上下行 / 累计）
- 节点上线自动同步配置；agent 通信全程 AES-GCM 加密
- 兼容 flux-panel 面板的 agent 协议，已有中转节点可直接接入

### 相对上游 3x-ui 的精简
移除多语言（仅中文）、Telegram Bot、Cloudflare WARP、NordVPN、LDAP、
客户端到期/限流/IP 限制/批量创建（多租户功能）、面板在线更新。

## 📦 附件说明

| 文件 | 用途 |
|---|---|
| `xpanel-linux-amd64.tar.gz` | 落地机面板（x86_64，含 Xray 与 geo 文件） |
| `xpanel-linux-arm64.tar.gz` | 落地机面板（ARM64） |
| `gost-linux-amd64.gz` | 中转机 gost agent（x86_64） |
| `gost-linux-arm64.gz` | 中转机 gost agent（ARM64） |

## 🚀 安装

**落地机（运行面板 + Xray）：**

```bash
bash <(curl -Ls https://raw.githubusercontent.com/YCJE/XPanel/main/install.sh)
```

安装完成后请保存脚本输出的随机用户名/密码/端口/访问路径，并按引导配置 SSL 证书。

**中转机（运行 gost agent）：**

在面板「转发管理 → 节点管理」添加节点，点击「安装命令」复制生成的命令在中转机执行：

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh | \
  bash -s -- "<面板地址:端口>" "<节点Token>"
```

**Docker：**

```bash
docker compose up -d
```

## 📝 说明

- 默认账号 `admin / admin`，首次登录后请立即在「面板设置」中修改
- 纯 Go SQLite 驱动，构建与运行不再依赖 CGO
- 更新面板：重新运行安装脚本，数据（SQLite）不会丢失
- 本项目仅供个人学习与自用，请遵守当地法律法规

## 🙏 致谢

- [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）
- [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0）
- [go-gost](https://github.com/go-gost/gost)（MIT）
- [Xray-core](https://github.com/XTLS/Xray-core)
