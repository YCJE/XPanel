# XPanel v1.0.4

第九/十轮代码审查的修复版本：转发模块节点变更残留清理、agent 并发安全、管理界面交互完善。

## 🐛 修复与加固

### 转发模块（重点）
- **修复：更换规则/隧道的节点后，旧节点上的 gost 服务不会被删除** ——
  端口转发换节点、隧道换入口/出口节点时，旧节点会残留转发服务与隧道链路
  （与 v1.0.3 修复的"删节点幽灵服务"同类，但发生在更新流程中）
- 修复 agent 上报协议变量的并发数据竞争（改用原子操作）

### 管理界面（UI）
- 未添加节点（或隧道场景少于 2 个节点）时，"添加转发/添加隧道"按钮置灰并给出引导提示
- 「同步配置」「启用/停用」操作增加成功/失败反馈提示，不再静默无响应

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

## 📝 版本号规范

- 小改动（bug 修复、文案优化）：递增修订号，如 v1.0.3 → v1.0.4
- 大改动（新功能、架构调整）：递增次版本号，如 v1.0.x → v1.1.0

## 🙏 致谢

- [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）
- [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0）
- [go-gost](https://github.com/go-gost/gost)（MIT）
- [Xray-core](https://github.com/XTLS/Xray-core)
