#!/bin/bash
# XPanel 中转节点 agent 安装脚本
# 在中转节点服务器上以 root 运行：
#   curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh | bash -s -- "<面板地址:端口>" "<节点Token>"
#
# 面板「转发管理 → 节点管理 → 安装命令」会生成完整命令。
set -e

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
cyan='\033[0;36m'
plain='\033[0m'

ok()   { echo -e "${green}  ✔ $1${plain}"; }
fail() { echo -e "${red}  ✘ $1${plain}"; }
info() { echo -e "${yellow}  ➜ $1${plain}"; }
step() {
    echo ""
    echo -e "${cyan}▶ [$1/5] $2${plain}"
}

PANEL_ADDR="$1"
SECRET="$2"
# 可选第 3 个参数：本地 agent 二进制路径（无 Release 下载时使用）
LOCAL_BIN="$3"

echo -e "${cyan}═══════════════════════════════════════════"
echo -e "      XPanel 中转节点 Agent 安装"
echo -e "═══════════════════════════════════════════${plain}"

if [ -z "$PANEL_ADDR" ] || [ -z "$SECRET" ]; then
    echo -e "${red}缺少参数！${plain}"
    echo "用法: bash agent-install.sh <面板地址:端口> <节点Token> [本地agent二进制路径]"
    echo "  面板地址形如 1.2.3.4:2053（浏览器访问面板时使用的地址与端口）"
    exit 1
fi

# ---------- 步骤 1/5: 环境检测 ----------
step 1 "检测运行环境"

if [ "$(id -u)" != "0" ]; then
    fail "请使用 root 运行本脚本"
    exit 1
fi
ok "root 权限确认"

ARCH=$(uname -m)
case "$ARCH" in
    x86_64 | amd64) GOST_ARCH="amd64" ;;
    aarch64 | arm64) GOST_ARCH="arm64" ;;
    *) fail "不支持的架构: $ARCH" && exit 1 ;;
esac
ok "CPU 架构: $ARCH (agent: $GOST_ARCH)"

OS_ID=""
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID="$ID"
fi
ok "操作系统: ${OS_ID:-未知}"

if command -v curl >/dev/null 2>&1; then
    ok "依赖 curl 已安装"
else
    info "未检测到 curl，尝试安装..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update -qq && apt-get install -y -q curl
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y -q curl
    elif command -v yum >/dev/null 2>&1; then
        yum install -y -q curl
    elif command -v apk >/dev/null 2>&1; then
        apk add curl
    else
        fail "无法自动安装 curl，请手动安装后重试"
        exit 1
    fi
    ok "curl 安装完成"
fi

mkdir -p /etc/gost /var/log/gost

# ---------- 步骤 2/5: 下载 agent ----------
step 2 "下载 gost agent"

GOST_BIN="/etc/gost/gost"
if [ -n "$LOCAL_BIN" ] && [ -f "$LOCAL_BIN" ]; then
    echo -e "  ➜ 使用本地 agent: $LOCAL_BIN"
    cp "$LOCAL_BIN" "$GOST_BIN"
elif [ -n "$GOST_BIN_LOCAL" ] && [ -f "$GOST_BIN_LOCAL" ]; then
    echo -e "  ➜ 使用本地 agent: $GOST_BIN_LOCAL"
    cp "$GOST_BIN_LOCAL" "$GOST_BIN"
else
    URL="https://github.com/YCJE/XPanel/releases/latest/download/gost-linux-${GOST_ARCH}.gz"
    info "下载地址: $URL"
    if ! curl -# -4fSL --retry 3 --retry-delay 2 -o /tmp/gost.gz "$URL"; then
        fail "agent 下载失败，请确认服务器可以访问 GitHub"
        info "也可以手动构建后传入第 3 个参数指定本地二进制路径"
        exit 1
    fi
    gunzip -f /tmp/gost.gz
    mv /tmp/gost "$GOST_BIN"
fi
chmod +x "$GOST_BIN"
ok "agent 就绪: $GOST_BIN"

# ---------- 步骤 3/5: 写入配置 ----------
step 3 "写入节点配置"

cat > /etc/gost/config.json <<EOF
{
    "addr": "${PANEL_ADDR}",
    "secret": "${SECRET}",
    "http": 0,
    "tls": 0,
    "socks": 0
}
EOF
chmod 600 /etc/gost/config.json
ok "配置已写入 /etc/gost/config.json (权限 600)"

# ---------- 步骤 4/5: 注册系统服务 ----------
step 4 "注册系统服务"

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    cat > /etc/systemd/system/gost.service <<EOF
[Unit]
Description=XPanel gost agent
After=network.target
Wants=network.target

[Service]
Type=simple
WorkingDirectory=/etc/gost
ExecStart=${GOST_BIN}
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable gost > /dev/null 2>&1
    ok "systemd 服务已注册并设置开机自启"
    USE_SYSTEMD=1
else
    info "未检测到 systemd，请手动启动 agent："
    echo "    cd /etc/gost && nohup ./gost >> /var/log/gost/gost.log 2>&1 &"
    USE_SYSTEMD=0
fi

# ---------- 步骤 5/5: 启动 ----------
step 5 "启动 agent"

if [ "${USE_SYSTEMD}" = "1" ]; then
    systemctl restart gost
    sleep 2
    if systemctl is-active --quiet gost; then
        ok "gost agent 已启动并正在连接面板"
        echo ""
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${green}        🎉 Agent 安装完成！${plain}"
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${yellow}➜ 回到面板「转发管理 → 节点管理」确认节点显示 [在线]${plain}"
        echo -e "${yellow}➜ 常用命令: systemctl status gost | journalctl -u gost -f${plain}"
    else
        fail "服务启动失败，请检查日志: journalctl -u gost -n 50"
        exit 1
    fi
else
    echo -e "${green}安装完成，请按上方提示手动启动 agent。${plain}"
fi
