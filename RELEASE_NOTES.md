# XPanel v1.0.2

本版本加固证书配置链路，杜绝"空/无效证书静默写入导致面板回退 HTTP"的问题。建议所有用户更新。

## 🐛 修复与加固

### 证书配置链路（重点）
- **`xpanel cert` 命令行写入前校验证书对**：文件为空、非 PEM 格式、或证书与私钥不匹配时
  直接拒绝保存并给出明确提示 —— 根治"配置成功但面板静默回退 HTTP"的部署坑
- 面板与订阅服务的 TLS 回退日志带上证书/私钥的具体路径，排障不再需要猜文件位置
- install.sh 自定义证书流程接入 CLI 校验，无效证书在配置阶段即被拦截

### 背景说明
v1.0.1 的部署实践中发现：证书签发未完成时执行安装命令会创建空的 `fullchain.pem`，
面板启动加载失败后静默回退 HTTP，而设置页仍显示"已配置 SSL"，极具误导性。
本版本从配置入口到运行日志全程拦截与标注。

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

- 小改动（bug 修复、文案优化）：递增修订号，如 v1.0.1 → v1.0.2
- 大改动（新功能、架构调整）：递增次版本号，如 v1.0.x → v1.1.0

## 🙏 致谢

- [3x-ui](https://github.com/MHSanaei/3x-ui)（GPL-3.0）
- [flux-panel](https://github.com/bqlpfy/flux-panel)（Apache-2.0）
- [go-gost](https://github.com/go-gost/gost)（MIT）
- [Xray-core](https://github.com/XTLS/Xray-core)
