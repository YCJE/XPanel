#!/bin/bash
# XPanel 中转节点 agent 安装脚本
# 在中转节点服务器上以 root 运行：
#   curl -fsSL https://raw.githubusercontent.com/YCJE/XPanel/main/agent-install.sh | bash -s -- "<面板地址:端口>" "<节点Token>"
#
# 面板「转发管理 → 节点管理 → 安装命令」会生成完整命令。
set -e

PANEL_ADDR="$1"
SECRET="$2"

if [ -z "$PANEL_ADDR" ] || [ -z "$SECRET" ]; then
    echo "用法: bash agent-install.sh <面板地址:端口> <节点Token>"
    echo "  面板地址形如 1.2.3.4:2053（浏览器访问面板时使用的地址与端口）"
    exit 1
fi

# ---------- 环境检测 ----------
if [ "$(id -u)" != "0" ]; then
    echo "请使用 root 运行本脚本" && exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
    x86_64 | amd64) GOST_ARCH="amd64" ;;
    aarch64 | arm64) GOST_ARCH="arm64" ;;
    *) echo "不支持的架构: $ARCH" && exit 1 ;;
esac

OS_ID=""
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID="$ID"
fi

echo ">>> 检测系统: $OS_ID $ARCH (gost agent arch: $GOST_ARCH)"

mkdir -p /etc/gost /var/log/gost

# ---------- 下载 agent ----------
GOST_BIN="/etc/gost/gost"
if [ -n "$GOST_BIN_LOCAL" ] && [ -f "$GOST_BIN_LOCAL" ]; then
    echo ">>> 使用本地 agent: $GOST_BIN_LOCAL"
    cp "$GOST_BIN_LOCAL" "$GOST_BIN"
else
    URL="https://github.com/YCJE/XPanel/releases/latest/download/gost-linux-${GOST_ARCH}.gz"
    echo ">>> 下载 agent: $URL"
    curl -4fSL --retry 3 -o /tmp/gost.gz "$URL"
    gunzip -f /tmp/gost.gz
    mv /tmp/gost "$GOST_BIN"
fi
chmod +x "$GOST_BIN"

# ---------- 写入配置 ----------
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

# ---------- systemd 服务 ----------
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
    systemctl enable gost
    systemctl restart gost
    echo ">>> 安装完成，服务状态："
    sleep 2
    systemctl status gost --no-pager -l | head -12 || true
else
    echo ">>> 未检测到 systemd，请手动启动 agent："
    echo "    cd /etc/gost && nohup ./gost >> /var/log/gost/gost.log 2>&1 &"
fi

echo ">>> 完成。回到面板「转发管理 → 节点管理」确认节点已在线。"
