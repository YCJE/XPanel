# XPanel：3x-ui 精简版 + gost 转发模块 合并计划（含 GitHub 发布）

## 一、背景结论

| 项目 | 技术栈 | 领域 |
|---|---|---|
| 3x-ui 2.9.4 | Go + Gin + SQLite + Vue2/antd 服务端模板 | Xray 协议代理（inbound/客户端/订阅/流量统计） |
| flux-panel（哆啦A梦转发面板） | Java Spring Boot + MySQL + React + go-gost agent | gost 端口转发 / 隧道转发（多节点、流量、限速） |

两者技术栈完全不同，无法直接合并代码。**方案：以 3x-ui (Go) 为底座精简，新增 gost 转发模块**；flux-panel 的 go-gost agent（Go 源码，跑在中转机上）可直接复用，面板侧用 Go 重新实现 agent 通信。

部署形态：**落地机**跑 XPanel（Xray 代理 + 面板），**中转机**跑 go-gost agent，隧道转发到落地机 Xray 端口。单管理员，多设备客户端通过订阅使用。

## 二、多余功能清单（确认砍掉）

**1. 多语言系统**：13 个语言包只留 `zh_CN.toml`，强制 zh_CN，删除语言切换与波斯资产（Vazirmatn 字体、persian-datepicker、moment-jalali）。
**2. 外部集成**：Telegram Bot（tgbot.go + 设置页）、Cloudflare WARP（warp.go + modal）、NordVPN 导入（nord.go + modal）。
**3. 客户端限制体系**：客户端到期/限速/IP数限制/批量创建；定时任务只保留流量统计采集。保留每客户端流量统计展示与订阅。
**4. 杂项**：LDAP 工具、捐赠链接、多语言 README、.github CI。

**保留不动**：Xray 高级设置（路由/负载均衡/DNS/FakeDNS/出站）、全部协议、订阅（base64/Clash/JSON）、2FA、系统监控、备份恢复。

## 三、实施步骤

### Phase 0：建立工程
1. 解压 3x-ui 源码到 `G:\Zcode\workplace\XPanel\` 作为基线；flux-panel 解压到 `reference/flux-panel/`（只读参考）。

### Phase 1：3x-ui 精简（XPanel v1）
2. 中文单语言（保留 i18n 机制、只装中文包，避免改动全部模板）。
3. 删外部集成：tgbot/warp/nord 的 service、modal、设置页、侧边栏、settings 键。
4. 删客户端限制体系：client 表单精简，删批量创建与限制执行定时任务（保留 email 标识的流量统计）。
5. 杂项清理 + 品牌改名 3x-ui → XPanel（config/name、登录页、侧边栏、标题、install.sh）。
6. 回归验证：go build + 全功能检查。

### Phase 2：gost 转发模块
7. 协议确认：读 flux-panel 的 NodeController/WebSocketConfig/go-gost register 源码，确认 agent↔面板协议（6365、token、配置下发、流量上报）。
8. SQLite 数据模型：forward_nodes / forward_rules / tunnel_rules / forward_stats。
9. 后端：agent 注册/心跳/系统信息、规则下发、流量上报入库与定时汇总、controller API（砍掉多用户配额/两级权限/月重置/sub-store API）。
10. 前端：侧边栏"转发管理"→ 节点管理 / 端口转发 / 隧道转发三页面；隧道出口可直接选择落地机 Xray inbound 端口。
11. go-gost agent 交叉编译 Linux 二进制；面板生成中转机一键安装命令（备选：自定义简化 WebSocket 协议 + 微调 go-gost 重编译）。

### Phase 3：收尾
12. 全流程测试：面板 + 模拟 agent 注册 → 下发规则 → 流量统计；Xray 侧回归。
13. 编写中文 README.md（项目介绍、功能、部署：落地机装面板、中转机执行安装命令、订阅使用）。

### Phase 4：发布 GitHub（用户新增要求）
14. `git init` → 全量提交（含 README）→ 推送到 `https://github.com/YCJE/XPanel.git`。
    - reference/ 目录与构建产物加入 .gitignore 不入库。
    - 若 GitHub 认证失败则报告用户。

## 四、风险与说明
- agent 协议细节在实施第 7 步确认（已列备选方案）。
- 全程只动 `G:\Zcode\workplace\XPanel\`，不碰 Downloads 里的原始 zip。