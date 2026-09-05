# agent/ — XPanel gost 中转 agent

本目录是 [flux-panel](https://github.com/bqlpfy/flux-panel)（哆啦A梦转发面板）所用的
go-gost agent 源码快照，XPanel 与其保持 **WebSocket + HTTP 上报协议完全兼容**，
因此 XPanel 面板可以直接管理用本目录编译出的 agent。

- agent 在中转节点上运行，启动后连接面板 `/system-info`（WebSocket）注册，
  接收转发/隧道配置（AddService / AddChains / AddLimiters 等命令），
  并通过 `/flow/upload`、`/flow/config` 上报流量与配置。
- 通信内容使用 AES-GCM 加密（密钥 = SHA256(节点 Token)）。
- 许可：go-gost 为 MIT，flux-panel 为 Apache-2.0（见本目录 LICENSE）。

构建：

```bash
cd agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gost .
```

正常情况下无需手动构建 —— 推送 `v*` 标签后 GitHub Actions 会自动把
`gost-linux-amd64.gz` / `gost-linux-arm64.gz` 附到 Release，配合根目录
`agent-install.sh` 完成中转节点一键安装。
