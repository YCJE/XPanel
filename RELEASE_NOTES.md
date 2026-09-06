# XPanel v1.0.1

本版本修复 v1.0.0 发布后部署过程中发现的证书签发与发布流水线问题，建议所有用户更新。

## 🐛 修复

### SSL 证书签发（重点）
- **签发失败自动重试 3 次**（间隔 15 秒）：此前 Let's Encrypt 端偶发网络波动
  （验证成功、最后下载证书阶段空白失败）会导致一次失败直接中止，现在会自动重试
- acme.sh 调用开启 `--log`，签发失败时自动在终端输出最近 25 行日志，不再"空白报错"
- 修复失败清理遗漏：ECC 模式下证书目录带 `_ecc` 后缀，此前清理不彻底可能影响下次签发
- 签发失败时自动重启面板（避免面板卡在停止状态）并提示常见原因（80 端口占用/未放行等）

### 发布流水线
- 修复 CI 中 Xray 资源包命名错误（官方为 `Xray-linux-64.zip` / `Xray-linux-arm64-v8a.zip`，
  原命名会导致构建在下载 Xray 步骤必然失败）
- 修复发布附件通配符遗漏 gost agent 二进制的问题

### 其他
- 修复 agent 启动日志两处 `fmt.Println` 误用格式化占位符的问题

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

在面板「转发管理 → 节点管理」添加节点，点击「安装命令」复制到中转机执行：

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh | \
  bash -s -- "<面板地址:端口>" "<节点Token>"
```

**Docker：**

```bash
docker compose up -d
```

默认账号 `admin / admin`，安装过程中会要求设置随机凭据；首次登录后请立即修改。

## 📝 版本号规范

- 小改动（bug 修复、文案优化）：递增修订号，如 v1.0.1 → v1.0.2
- 大改动（新功能、架构调整）：递增次版本号，如 v1.0.x → v1.1.0

## 🙏 致谢

- [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）
- [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0）
- [go-gost](https://github.com/go-gost/gost)（MIT）
- [Xray-core](https://github.com/XTLS/Xray-core)
