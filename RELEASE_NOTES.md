# XPanel v1.0.3

第六轮深度审查的修复版本，聚焦敏感文件权限与转发节点生命周期管理。

## 🐛 修复与加固

### 面板崩溃隐患（重点）
- **修复流量统计任务（每 10 秒运行）中的三处未保护类型断言**：入站 settings JSON 中
  client 条目为非对象、email 或 expiryTime 字段类型异常时，统计任务会 panic 且每
  10 秒循环崩溃（gin 的异常恢复不覆盖定时任务 goroutine；导入旧版 3x-ui 数据库或
  手工编辑数据可触发）。现已全部改为安全断言，异常数据自动跳过处理

### 敏感文件权限（安全加固）
- **Xray 配置文件（`bin/config.json`）写入权限从 0777 收紧为 0600** —— 该文件包含
  所有客户端的 UUID/密码等凭据，此前同机其他用户/进程可读
- **SQLite 数据库文件权限收紧为 0600** —— 含管理员凭据哈希与客户端配置

### 转发节点生命周期（功能修复）
- **删除节点时清理对端节点的幽灵服务**：此前删除隧道任一端的节点后，
  存活对端节点上的 gost 服务（relay 监听/链路）不会被下发删除，会一直空转
- **修改节点 IP 后自动重推配置**：此前修改出口节点的服务器 IP 后，
  入口节点上的隧道链路仍指向旧 IP；现在保存后自动重推该节点及所有关联隧道的配置
- 节点删除确认框增加提示：需在节点服务器上手动卸载 agent（面板侧删除不会停止节点上的 gost 进程）

## 📦 附件说明

| 文件 | 用途 |
|---|---|
| `xpanel-linux-amd64.tar.gz` | 落地机面板（x86_64，含 Xray 与 geo 文件） |
| `xpanel-linux-arm64.tar.gz` | 落地机面板（ARM64） |
| `gost-linux-amd64.gz` | 中转机 gost agent（x86_64） |
| `gost-linux-arm64.gz` | 中转机 gost agent（ARM64） |

## 🚀 安装 / 更新

**落地机（安装或更新，数据不会丢失）：**

```bash
bash <(curl -Ls https://raw.githubusercontent.com/YCJE/XPanel/main/install.sh)
```

**中转机（agent）：**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh |   bash -s -- "<面板地址:端口>" "<节点Token>"
```

**Docker：**

```bash
docker compose up -d
```

## 📝 版本号规范

- 小改动（bug 修复、文案优化）：递增修订号，如 v1.0.2 → v1.0.3
- 大改动（新功能、架构调整）：递增次版本号，如 v1.0.x → v1.1.0

## 🙏 致谢

- [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）
- [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0）
- [go-gost](https://github.com/go-gost/gost)（MIT）
- [Xray-core](https://github.com/XTLS/Xray-core)
