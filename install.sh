#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
cyan='\033[0;36m'
plain='\033[0m'

# ───── 进度提示助手 ─────
STEP_NO=0
step() {
    STEP_NO=$((STEP_NO + 1))
    echo -e "${blue}▶ 第 $STEP_NO 步: $1${plain}"
}
ok()   { echo -e "${green}  ✔ $1${plain}"; }
fail() { echo -e "${red}  ✘ $1${plain}"; }
info() { echo -e "${yellow}  ➜ $1${plain}"; }

# 转圈进度条: run_spinner "提示文字" 命令 [参数...]
# 命令输出重定向到 /tmp/xpanel-install.log, 结束后显示 ✔/✘ 结果
run_spinner() {
    local msg="$1"; shift
    local chars='|/-\'
    local i=0
    "$@" > /tmp/xpanel-install.log 2>&1 &
    local pid=$!
    while kill -0 "$pid" 2>/dev/null; do
        printf "\r    \e[36m[%c] %s\e[0m" "${chars:i%4:1}" "$msg"
        sleep 0.2
        i=$((i + 1))
    done
    wait "$pid"
    local ret=$?
    if [[ $ret -eq 0 ]]; then
        printf "\r    \e[32m✔ %s — 完成\e[0m\n" "$msg"
    else
        printf "\r    \e[31m✘ %s — 失败\e[0m\n" "$msg"
        echo -e "${yellow}  ➜ 详细输出: /tmp/xpanel-install.log${plain}"
    fi
    return $ret
}

cur_dir=$(pwd)

xpanel_folder="${XPANEL_MAIN_FOLDER:=/usr/local/xpanel}"
xpanel_service="${XPANEL_SERVICE:=/etc/systemd/system}"

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}错误: ${plain} 请使用 root 权限运行本脚本 \n " && exit 1

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo "无法识别操作系统类型，请联系开发者！" >&2
    exit 1
fi
echo "系统类型: $release"

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        i*86 | x86) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm) echo 'armv7' ;;
        armv6* | armv6) echo 'armv6' ;;
        armv5* | armv5) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) echo -e "${green}不支持的 CPU 架构！ ${plain}" && rm -f install.sh && exit 1 ;;
    esac
}

echo "CPU 架构: $(arch)"

# Simple helpers
is_ipv4() {
    [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && return 0 || return 1
}
is_ipv6() {
    [[ "$1" =~ : ]] && return 0 || return 1
}
is_ip() {
    is_ipv4 "$1" || is_ipv6 "$1"
}
is_domain() {
    [[ "$1" =~ ^([A-Za-z0-9](-*[A-Za-z0-9])*\.)+(xn--[a-z0-9]{2,}|[A-Za-z]{2,})$ ]] && return 0 || return 1
}
# 清理 acme.sh 中某域名/IP 的签发数据 (ECC 模式目录带 _ecc 后缀)
cleanup_acme_dir() {
    rm -rf ~/.acme.sh/"$1" 2> /dev/null
    rm -rf ~/.acme.sh/"$1_ecc" 2> /dev/null
}
# 从 acme.sh 日志提取最近错误, 方便排障
show_acme_log_tail() {
    tail -n 25 ~/.acme.sh/acme.sh.log 2> /dev/null | sed 's/^/    /'
}

# Port helpers
is_port_in_use() {
    local port="$1"
    if command -v ss > /dev/null 2>&1; then
        ss -ltn 2> /dev/null | awk -v p=":${port}$" '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v netstat > /dev/null 2>&1; then
        netstat -lnt 2> /dev/null | awk -v p=":${port} " '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v lsof > /dev/null 2>&1; then
        lsof -nP -iTCP:${port} -sTCP:LISTEN > /dev/null 2>&1 && return 0
    fi
    return 1
}

install_base() {
    step "安装系统依赖"
    case "${release}" in
        ubuntu | debian | armbian)
            run_spinner "更新软件源" apt-get update || return 1
            run_spinner "安装依赖软件包" apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl || return 1
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            run_spinner "更新软件源" dnf -y update || return 1
            run_spinner "安装依赖软件包" dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl || return 1
            ;;
        centos)
            if [[ "${VERSION_ID}" =~ ^7 ]]; then
                run_spinner "更新软件源" yum -y update || return 1
                run_spinner "安装依赖软件包" yum install -y cronie curl tar tzdata socat ca-certificates openssl || return 1
            else
                run_spinner "更新软件源" dnf -y update || return 1
                run_spinner "安装依赖软件包" dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl || return 1
            fi
            ;;
        arch | manjaro | parch)
            run_spinner "更新软件源并安装依赖" pacman -Syu --noconfirm cronie curl tar tzdata socat ca-certificates openssl || return 1
            ;;
        opensuse-tumbleweed | opensuse-leap)
            run_spinner "更新软件源" zypper refresh || return 1
            run_spinner "安装依赖软件包" zypper -q install -y cron curl tar timezone socat ca-certificates openssl || return 1
            ;;
        alpine)
            run_spinner "更新软件源" apk update || return 1
            run_spinner "安装依赖软件包" apk add dcron curl tar tzdata socat ca-certificates openssl || return 1
            ;;
        *)
            run_spinner "更新软件源" apt-get update || return 1
            run_spinner "安装依赖软件包" apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl || return 1
            ;;
    esac
}

gen_random_string() {
    local length="$1"
    openssl rand -base64 $((length * 2)) \
        | tr -dc 'a-zA-Z0-9' \
        | head -c "$length"
}

install_acme() {
    echo -e "${green}正在安装 acme.sh (SSL 证书管理工具)...${plain}"
    cd ~ || return 1
    run_spinner "安装 acme.sh" bash -c "curl -s https://get.acme.sh | sh"
    if [ $? -ne 0 ]; then
        echo -e "${red}acme.sh 安装失败${plain}"
        return 1
    else
        echo -e "${green}acme.sh 安装成功${plain}"
    fi
    return 0
}

setup_ssl_certificate() {
    local domain="$1"
    local server_ip="$2"
    local existing_port="$3"
    local existing_webBasePath="$4"

    echo -e "${green}正在配置 SSL 证书...${plain}"

    # Check if acme.sh is installed
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${yellow}acme.sh 安装失败，跳过 SSL 配置${plain}"
            return 1
        fi
    fi

    # Create certificate directory
    local certPath="/root/cert/${domain}"
    mkdir -p "$certPath"

    # Issue certificate
    echo -e "${green}正在为 ${domain} 签发 SSL 证书...${plain}"
    echo -e "${yellow}注意: 端口 80 必须开放且可从公网访问${plain}"

    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1
    ~/.acme.sh/acme.sh --issue -d ${domain} --listen-v6 --standalone --httpport 80 --force

    if [ $? -ne 0 ]; then
        echo -e "${yellow}为 ${domain} 签发证书失败${plain}"
        echo -e "${yellow}请确认 80 端口已开放，稍后可在 xpanel 菜单中重试${plain}"
        cleanup_acme_dir "${domain}"
        rm -rf "$certPath" 2> /dev/null
        return 1
    fi

    # Install certificate
    ~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem \
        --reloadcmd "systemctl restart xpanel" > /dev/null 2>&1

    if [ $? -ne 0 ]; then
        echo -e "${yellow}证书安装失败${plain}"
        return 1
    fi

    # Enable auto-renew
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1
    # Secure permissions: private key readable only by owner
    chmod 600 $certPath/privkey.pem 2> /dev/null
    chmod 644 $certPath/fullchain.pem 2> /dev/null

    # Set certificate for panel
    local webCertFile="/root/cert/${domain}/fullchain.pem"
    local webKeyFile="/root/cert/${domain}/privkey.pem"

    if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
        ${xpanel_folder}/xpanel cert -webCert "$webCertFile" -webCertKey "$webKeyFile" > /dev/null 2>&1
        echo -e "${green}SSL 证书安装并配置成功！${plain}"
        return 0
    else
        echo -e "${yellow}未找到证书文件${plain}"
        return 1
    fi
}

# Issue Let's Encrypt IP certificate with shortlived profile (~6 days validity)
# Requires acme.sh and port 80 open for HTTP-01 challenge
setup_ip_certificate() {
    local ipv4="$1"
    local ipv6="$2" # optional

    echo -e "${green}正在配置 Let's Encrypt IP 证书 (短期证书)...${plain}"
    echo -e "${yellow}注意: IP 证书有效期约 6 天，会自动续期。${plain}"
    echo -e "${yellow}默认监听 80 端口；若选择其他端口，请确保外部 80 端口转发到该端口。${plain}"

    # Check for acme.sh
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${red}acme.sh 安装失败${plain}"
            return 1
        fi
    fi

    # Validate IP address
    if [[ -z "$ipv4" ]]; then
        echo -e "${red}需要 IPv4 地址${plain}"
        return 1
    fi

    if ! is_ipv4 "$ipv4"; then
        echo -e "${red}IPv4 地址无效: $ipv4${plain}"
        return 1
    fi

    # Create certificate directory
    local certDir="/root/cert/ip"
    mkdir -p "$certDir"

    # Build domain arguments
    local domain_args="-d ${ipv4}"
    if [[ -n "$ipv6" ]] && is_ipv6 "$ipv6"; then
        domain_args="${domain_args} -d ${ipv6}"
        echo -e "${green}已包含 IPv6 地址: ${ipv6}${plain}"
    fi

    # Set reload command for auto-renewal (add || true so it doesn't fail during first install)
    local reloadCmd="systemctl restart xpanel 2>/dev/null || rc-service xpanel restart 2>/dev/null || true"

    # Choose port for HTTP-01 listener (default 80, prompt override)
    local WebPort=""
    read -rp "ACME HTTP-01 验证使用的端口 (默认 80): " WebPort
    WebPort="${WebPort:-80}"
    if ! [[ "${WebPort}" =~ ^[0-9]+$ ]] || ((WebPort < 1 || WebPort > 65535)); then
        echo -e "${red}端口号无效，回退为 80。${plain}"
        WebPort=80
    fi
    echo -e "${green}将使用 ${WebPort} 端口进行独立验证。${plain}"
    if [[ "${WebPort}" -ne 80 ]]; then
        echo -e "${yellow}提醒: Let's Encrypt 仍然连接 80 端口，请将外部 80 端口转发到 ${WebPort}。${plain}"
    fi

    # Ensure chosen port is available
    while true; do
        if is_port_in_use "${WebPort}"; then
            echo -e "${yellow}端口 ${WebPort} 已被占用。${plain}"

            local alt_port=""
            read -rp "请输入其他端口供 acme.sh 独立验证使用 (留空则中止): " alt_port
            alt_port="${alt_port// /}"
            if [[ -z "${alt_port}" ]]; then
                echo -e "${red}端口 ${WebPort} 被占用，无法继续。${plain}"
                return 1
            fi
            if ! [[ "${alt_port}" =~ ^[0-9]+$ ]] || ((alt_port < 1 || alt_port > 65535)); then
                echo -e "${red}端口号无效。${plain}"
                return 1
            fi
            WebPort="${alt_port}"
            continue
        else
            echo -e "${green}端口 ${WebPort} 空闲，可用于独立验证。${plain}"
            break
        fi
    done

    # Issue certificate with shortlived profile
    echo -e "${green}正在为 ${ipv4} 签发 IP 证书...${plain}"
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1

    local issued=0
    local attempt
    for attempt in 1 2 3; do
        echo -e "${yellow}正在签发证书 (第 $attempt/3 次尝试)...${plain}"
        ~/.acme.sh/acme.sh --issue             ${domain_args}             --standalone             --server letsencrypt             --certificate-profile shortlived             --days 6             --httpport ${WebPort}             --force             --log
        if [ $? -eq 0 ]; then
            issued=1
            break
        fi
        if [ $attempt -lt 3 ]; then
            echo -e "${yellow}本次签发失败（多为 CA 端瞬时网络问题），15 秒后自动重试...${plain}"
            sleep 15
        fi
    done

    if [[ ${issued} -ne 1 ]]; then
        echo -e "${red}IP 证书签发失败（已自动重试 3 次）。最近日志：${plain}"
        show_acme_log_tail
        echo -e "${yellow}请确保 ${WebPort} 端口可访问（或从外部 80 端口转发）${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        cleanup_acme_dir "${ipv4}"
        [[ -n "$ipv6" ]] && cleanup_acme_dir "${ipv6}"
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}证书签发成功，正在安装...${plain}"

    # Install certificate
    # Note: acme.sh may report "Reload error" and exit non-zero if reloadcmd fails,
    # but the cert files are still installed. We check for files instead of exit code.
    ~/.acme.sh/acme.sh --installcert -d ${ipv4} \
        --key-file "${certDir}/privkey.pem" \
        --fullchain-file "${certDir}/fullchain.pem" \
        --reloadcmd "${reloadCmd}" 2>&1 || true

    # Verify certificate files exist (don't rely on exit code - reloadcmd failure causes non-zero)
    if [[ ! -f "${certDir}/fullchain.pem" || ! -f "${certDir}/privkey.pem" ]]; then
        echo -e "${red}安装后未找到证书文件${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        cleanup_acme_dir "${ipv4}"
        [[ -n "$ipv6" ]] && cleanup_acme_dir "${ipv6}"
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}证书文件安装成功${plain}"

    # Enable auto-upgrade for acme.sh (ensures cron job runs)
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1

    # Secure permissions: private key readable only by owner
    chmod 600 ${certDir}/privkey.pem 2> /dev/null
    chmod 644 ${certDir}/fullchain.pem 2> /dev/null

    # Configure panel to use the certificate
    echo -e "${green}正在为面板设置证书路径...${plain}"
    ${xpanel_folder}/xpanel cert -webCert "${certDir}/fullchain.pem" -webCertKey "${certDir}/privkey.pem"

    if [ $? -ne 0 ]; then
        echo -e "${yellow}警告: 无法自动设置证书路径${plain}"
        echo -e "${yellow}证书文件位于:${plain}"
        echo -e "  证书: ${certDir}/fullchain.pem"
        echo -e "  私钥: ${certDir}/privkey.pem"
    else
        echo -e "${green}证书路径配置成功${plain}"
    fi

    echo -e "${green}IP 证书安装并配置成功！${plain}"
    echo -e "${green}证书有效期约 6 天，通过 acme.sh 定时任务自动续期。${plain}"
    echo -e "${yellow}acme.sh 将在到期前自动续期并重载面板。${plain}"
    return 0
}

# Comprehensive manual SSL certificate issuance via acme.sh
ssl_cert_issue() {
    local existing_webBasePath=$(${xpanel_folder}/xpanel setting -show true | grep 'webBasePath:' | awk -F': ' '{print $2}' | tr -d '[:space:]' | sed 's#^/##')
    local existing_port=$(${xpanel_folder}/xpanel setting -show true | grep 'port:' | awk -F': ' '{print $2}' | tr -d '[:space:]')

    # check for acme.sh first
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        echo "未找到 acme.sh，正在安装..."
        cd ~ || return 1
        curl -s https://get.acme.sh | sh
        if [ $? -ne 0 ]; then
            echo -e "${red}acme.sh 安装失败${plain}"
            return 1
        else
            echo -e "${green}acme.sh 安装成功${plain}"
        fi
    fi

    # get the domain here, and we need to verify it
    local domain=""
    while true; do
        read -rp "请输入你的域名: " domain
        domain="${domain// /}" # Trim whitespace

        if [[ -z "$domain" ]]; then
            echo -e "${red}域名不能为空，请重试。${plain}"
            continue
        fi

        if ! is_domain "$domain"; then
            echo -e "${red}域名格式无效: ${domain}，请输入有效的域名。${plain}"
            continue
        fi

        break
    done
    echo -e "${green}你的域名: ${domain}，正在检查...${plain}"
    SSL_ISSUED_DOMAIN="${domain}"

    # detect existing certificate and reuse it if present
    local cert_exists=0
    if ~/.acme.sh/acme.sh --list 2> /dev/null | awk '{print $1}' | grep -Fxq "${domain}"; then
        cert_exists=1
        local certInfo=$(~/.acme.sh/acme.sh --list 2> /dev/null | grep -F "${domain}")
        echo -e "${yellow}发现 ${domain} 的已有证书，将复用。${plain}"
        [[ -n "${certInfo}" ]] && echo "$certInfo"
    else
        echo -e "${green}域名已就绪，可以签发证书...${plain}"
    fi

    # create a directory for the certificate
    certPath="/root/cert/${domain}"
    if [ ! -d "$certPath" ]; then
        mkdir -p "$certPath"
    else
        rm -rf "$certPath"
        mkdir -p "$certPath"
    fi

    # get the port number for the standalone server
    local WebPort=80
    read -rp "请选择使用的端口 (默认 80): " WebPort
    if [[ ${WebPort} -gt 65535 || ${WebPort} -lt 1 ]]; then
        echo -e "${yellow}输入的 ${WebPort} 无效，将使用默认端口 80。${plain}"
        WebPort=80
    fi
    echo -e "${green}将使用端口 ${WebPort} 签发证书，请确保该端口已开放。${plain}"

    # Stop panel temporarily
    echo -e "${yellow}正在临时停止面板...${plain}"
    systemctl stop xpanel 2> /dev/null || rc-service xpanel stop 2> /dev/null

    if [[ ${cert_exists} -eq 0 ]]; then
        # issue the certificate (CA 偶发网络波动很常见, 失败自动重试 3 次)
        ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
        local issued=0
        local attempt
        for attempt in 1 2 3; do
            echo -e "${yellow}正在签发证书 (第 $attempt/3 次尝试)...${plain}"
            ~/.acme.sh/acme.sh --issue -d ${domain} --listen-v6 --standalone --httpport ${WebPort} --force --log
            if [ $? -eq 0 ]; then
                issued=1
                break
            fi
            if [ $attempt -lt 3 ]; then
                echo -e "${yellow}本次签发失败（多为 CA 端瞬时网络问题），15 秒后自动重试...${plain}"
                sleep 15
            fi
        done
        if [[ ${issued} -ne 1 ]]; then
            echo -e "${red}证书签发失败（已自动重试 3 次）。最近日志：${plain}"
            show_acme_log_tail
            echo -e "${yellow}常见原因: 80 端口被占用/未放行、CA 临时故障。可稍后在 xpanel 菜单重试。${plain}"
            cleanup_acme_dir "${domain}"
            systemctl start xpanel 2> /dev/null || rc-service xpanel start 2> /dev/null
            return 1
        else
            echo -e "${green}证书签发成功，正在安装证书...${plain}"
        fi
    else
        echo -e "${green}使用已有证书，正在安装...${plain}"
    fi

    # Setup reload command
    reloadCmd="systemctl restart xpanel || rc-service xpanel restart"
    echo -e "${green}默认续期命令为: ${yellow}systemctl restart xpanel || rc-service xpanel restart${plain}"
    echo -e "${green}该命令会在每次签发/续期证书时执行。${plain}"
    read -rp "是否修改 ACME 的 --reloadcmd 续期命令? (y/n): " setReloadcmd
    if [[ "$setReloadcmd" == "y" || "$setReloadcmd" == "Y" ]]; then
        echo -e "\n${green}\t1.${plain} 预设: systemctl reload nginx ; systemctl restart xpanel"
        echo -e "${green}\t2.${plain} 自行输入命令"
        echo -e "${green}\t0.${plain} 保持默认续期命令"
        read -rp "请选择: " choice
        case "$choice" in
            1)
                echo -e "${green}续期命令: systemctl reload nginx ; systemctl restart xpanel${plain}"
                reloadCmd="systemctl reload nginx ; systemctl restart xpanel"
                ;;
            2)
                echo -e "${yellow}建议把 xpanel 重启命令放在最后${plain}"
                read -rp "请输入自定义续期命令: " reloadCmd
                echo -e "${green}续期命令: ${reloadCmd}${plain}"
                ;;
            *)
                echo -e "${green}保持默认续期命令${plain}"
                ;;
        esac
    fi

    # install the certificate
    local installOutput=""
    installOutput=$(~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem --reloadcmd "${reloadCmd}" 2>&1)
    local installRc=$?
    echo "${installOutput}"

    local installWroteFiles=0
    if echo "${installOutput}" | grep -q "Installing key to:" && echo "${installOutput}" | grep -q "Installing full chain to:"; then
        installWroteFiles=1
    fi

    if [[ -f "/root/cert/${domain}/privkey.pem" && -f "/root/cert/${domain}/fullchain.pem" && (${installRc} -eq 0 || ${installWroteFiles} -eq 1) ]]; then
        echo -e "${green}证书安装成功，正在启用自动续期...${plain}"
    else
        echo -e "${red}证书安装失败，退出。${plain}"
        if [[ ${cert_exists} -eq 0 ]]; then
            cleanup_acme_dir "${domain}"
        fi
        systemctl start xpanel 2> /dev/null || rc-service xpanel start 2> /dev/null
        return 1
    fi

    # enable auto-renew
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade
    if [ $? -ne 0 ]; then
        echo -e "${yellow}自动续期配置异常，证书详情:${plain}"
        ls -lah /root/cert/${domain}/
        # Secure permissions: private key readable only by owner
        chmod 600 $certPath/privkey.pem 2> /dev/null
        chmod 644 $certPath/fullchain.pem 2> /dev/null
    else
        echo -e "${green}自动续期配置成功，证书详情:${plain}"
        ls -lah /root/cert/${domain}/
        # Secure permissions: private key readable only by owner
        chmod 600 $certPath/privkey.pem 2> /dev/null
        chmod 644 $certPath/fullchain.pem 2> /dev/null
    fi

    # start panel
    systemctl start xpanel 2> /dev/null || rc-service xpanel start 2> /dev/null

    # Prompt user to set panel paths after successful certificate installation
    read -rp "是否将此证书应用到面板? (y/n): " setPanel
    if [[ "$setPanel" == "y" || "$setPanel" == "Y" ]]; then
        local webCertFile="/root/cert/${domain}/fullchain.pem"
        local webKeyFile="/root/cert/${domain}/privkey.pem"

        if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
            ${xpanel_folder}/xpanel cert -webCert "$webCertFile" -webCertKey "$webKeyFile"
            echo -e "${green}面板证书路径已设置${plain}"
            echo -e "${green}证书文件: $webCertFile${plain}"
            echo -e "${green}私钥文件: $webKeyFile${plain}"
            echo ""
            echo -e "${green}访问地址: https://${domain}:${existing_port}/${existing_webBasePath}${plain}"
            echo -e "${yellow}面板将重启以应用 SSL 证书...${plain}"
            systemctl restart xpanel 2> /dev/null || rc-service xpanel restart 2> /dev/null
        else
            echo -e "${red}错误: 未找到 $domain 的证书或私钥文件。${plain}"
        fi
    else
        echo -e "${yellow}跳过面板路径设置。${plain}"
    fi

    return 0
}

# Reusable interactive SSL setup (domain or IP)
# Sets global `SSL_HOST` to the chosen domain/IP for Access URL usage
prompt_and_setup_ssl() {
    local panel_port="$1"
    local web_base_path="$2" # expected without leading slash
    local server_ip="$3"

    local ssl_choice=""

    echo -e "${yellow}请选择 SSL 证书配置方式:${plain}"
    echo -e "${green}1.${plain} Let's Encrypt 域名证书 (90 天有效期, 自动续期)"
    echo -e "${green}2.${plain} Let's Encrypt IP 证书 (6 天有效期, 自动续期)"
    echo -e "${green}3.${plain} 自定义证书 (手动指定已有证书文件路径)"
    echo -e "${blue}说明:${plain} 方式 1/2 需要开放 80 端口；方式 3 需要已有证书文件。"
    read -rp "请选择 (默认 2, IP 证书): " ssl_choice
    ssl_choice="${ssl_choice// /}" # Trim whitespace

    # Default to 2 (IP cert) if input is empty or invalid (not 1 or 3)
    if [[ "$ssl_choice" != "1" && "$ssl_choice" != "3" ]]; then
        ssl_choice="2"
    fi

    case "$ssl_choice" in
        1)
            # User chose Let's Encrypt domain option
            echo -e "${green}使用 Let's Encrypt 域名证书...${plain}"
            if ssl_cert_issue; then
                local cert_domain="${SSL_ISSUED_DOMAIN}"
                if [[ -z "${cert_domain}" ]]; then
                    cert_domain=$(~/.acme.sh/acme.sh --list 2> /dev/null | tail -1 | awk '{print $1}')
                fi

                if [[ -n "${cert_domain}" ]]; then
                    SSL_HOST="${cert_domain}"
                    echo -e "${green}✔ SSL 证书配置成功, 域名: ${cert_domain}${plain}"
                else
                    echo -e "${yellow}SSL 配置可能已完成, 但域名提取失败${plain}"
                    SSL_HOST="${server_ip}"
                fi
            else
                echo -e "${red}域名证书配置失败。${plain}"
                SSL_HOST="${server_ip}"
            fi
            ;;
        2)
            # User chose Let's Encrypt IP certificate option
            echo -e "${green}使用 Let's Encrypt IP 证书 (短期证书)...${plain}"

            # Ask for optional IPv6
            local ipv6_addr=""
            read -rp "是否有需要包含的 IPv6 地址? (留空跳过): " ipv6_addr
            ipv6_addr="${ipv6_addr// /}" # Trim whitespace

            # Stop panel if running (port 80 needed)
            if [[ $release == "alpine" ]]; then
                rc-service xpanel stop > /dev/null 2>&1
            else
                systemctl stop xpanel > /dev/null 2>&1
            fi

            setup_ip_certificate "${server_ip}" "${ipv6_addr}"
            if [ $? -eq 0 ]; then
                SSL_HOST="${server_ip}"
                echo -e "${green}✔ Let's Encrypt IP 证书配置成功${plain}"
            else
                echo -e "${red}✘ IP 证书配置失败, 请确认 80 端口已开放。${plain}"
                SSL_HOST="${server_ip}"
            fi
            ;;
        3)
            # User chose Custom Paths (User Provided) option
            echo -e "${green}使用自定义已有证书...${plain}"
            local custom_cert=""
            local custom_key=""
            local custom_domain=""

            # 3.1 Request Domain to compose Panel URL later
            read -rp "请输入证书对应的域名: " custom_domain
            custom_domain="${custom_domain// /}" # Remove spaces

            # 3.2 Loop for Certificate Path
            while true; do
                read -rp "请输入证书文件路径 (关键词: .crt / fullchain): " custom_cert
                # Strip quotes if present
                custom_cert=$(echo "$custom_cert" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_cert" && -r "$custom_cert" && -s "$custom_cert" ]]; then
                    break
                elif [[ ! -f "$custom_cert" ]]; then
                    echo -e "${red}错误: 文件不存在！请重试。${plain}"
                elif [[ ! -r "$custom_cert" ]]; then
                    echo -e "${red}错误: 文件存在但不可读（请检查权限）！${plain}"
                else
                    echo -e "${red}错误: 文件为空！${plain}"
                fi
            done

            # 3.3 Loop for Private Key Path
            while true; do
                read -rp "请输入私钥文件路径 (关键词: .key / privatekey): " custom_key
                # Strip quotes if present
                custom_key=$(echo "$custom_key" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_key" && -r "$custom_key" && -s "$custom_key" ]]; then
                    break
                elif [[ ! -f "$custom_key" ]]; then
                    echo -e "${red}错误: 文件不存在！请重试。${plain}"
                elif [[ ! -r "$custom_key" ]]; then
                    echo -e "${red}错误: 文件存在但不可读（请检查权限）！${plain}"
                else
                    echo -e "${red}错误: 文件为空！${plain}"
                fi
            done

            # 3.4 Apply Settings via xpanel binary
            ${xpanel_folder}/xpanel cert -webCert "$custom_cert" -webCertKey "$custom_key" > /dev/null 2>&1

            # Set SSL_HOST for composing Panel URL
            if [[ -n "$custom_domain" ]]; then
                SSL_HOST="$custom_domain"
            else
                SSL_HOST="${server_ip}"
            fi

            echo -e "${green}✔ 自定义证书路径已应用。${plain}"
            echo -e "${yellow}注意: 证书续期需要自行在外部完成。${plain}"

            systemctl restart xpanel > /dev/null 2>&1 || rc-service xpanel restart > /dev/null 2>&1
            ;;
        *)
            echo -e "${red}无效选项，跳过 SSL 配置。${plain}"
            SSL_HOST="${server_ip}"
            ;;
    esac
}

config_after_install() {
    local existing_hasDefaultCredential=$(${xpanel_folder}/xpanel setting -show true | grep -Eo 'hasDefaultCredential: .+' | awk '{print $2}')
    local existing_webBasePath=$(${xpanel_folder}/xpanel setting -show true | grep -Eo 'webBasePath: .+' | awk '{print $2}' | sed 's#^/##')
    local existing_port=$(${xpanel_folder}/xpanel setting -show true | grep -Eo 'port: .+' | awk '{print $2}')
    # Properly detect empty cert by checking if cert: line exists and has content after it
    local existing_cert=$(${xpanel_folder}/xpanel setting -getCert true | grep 'cert:' | awk -F': ' '{print $2}' | tr -d '[:space:]')
    local URL_lists=(
        "https://api4.ipify.org"
        "https://ipv4.icanhazip.com"
        "https://v4.api.ipinfo.io/ip"
        "https://ipv4.myexternalip.com/raw"
        "https://4.ident.me"
        "https://check-host.net/ip"
    )
    local server_ip=""
    for ip_address in "${URL_lists[@]}"; do
        local response=$(curl -s -w "\n%{http_code}" --max-time 3 "${ip_address}" 2> /dev/null)
        local http_code=$(echo "$response" | tail -n1)
        local ip_result=$(echo "$response" | head -n-1 | tr -d '[:space:]')
        if [[ "${http_code}" == "200" && -n "${ip_result}" ]]; then
            server_ip="${ip_result}"
            break
        fi
    done

    if [[ ${#existing_webBasePath} -lt 4 ]]; then
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_webBasePath=$(gen_random_string 18)
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            read -rp "是否自定义面板端口? (不自定义则使用随机端口) [y/n]: " config_confirm
            if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
                read -rp "请设置面板端口: " config_port
                echo -e "${yellow}你的面板端口: ${config_port}${plain}"
            else
                local config_port=$(shuf -i 1024-62000 -n 1)
                echo -e "${yellow}已生成随机端口: ${config_port}${plain}"
            fi

            ${xpanel_folder}/xpanel setting -username "${config_username}" -password "${config_password}" -port "${config_port}" -webBasePath "${config_webBasePath}"

            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}          SSL 证书配置 (必须)          ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}出于安全考虑, 面板必须配置 SSL 证书。${plain}"
            echo -e "${yellow}Let's Encrypt 现已同时支持域名和 IP 地址！${plain}"
            echo ""

            prompt_and_setup_ssl "${config_port}" "${config_webBasePath}" "${server_ip}"

            # Display final credentials and access information
            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}          面板安装完成！          ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}用户名:      ${config_username}${plain}"
            echo -e "${green}密码:        ${config_password}${plain}"
            echo -e "${green}端口:        ${config_port}${plain}"
            echo -e "${green}访问路径:    ${config_webBasePath}${plain}"
            echo -e "${green}访问地址:    https://${SSL_HOST}:${config_port}/${config_webBasePath}${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}⚠ 重要: 请妥善保存以上凭据！${plain}"
            echo -e "${yellow}⚠ SSL 证书: 已启用${plain}"
        else
            local config_webBasePath=$(gen_random_string 18)
            echo -e "${yellow}访问路径缺失或过短, 正在生成新路径...${plain}"
            ${xpanel_folder}/xpanel setting -webBasePath "${config_webBasePath}"
            echo -e "${green}New 访问路径:    ${config_webBasePath}${plain}"

            # If the panel is already installed but no certificate is configured, prompt for SSL now
            if [[ -z "${existing_cert}" ]]; then
                echo ""
                echo -e "${green}═══════════════════════════════════════════${plain}"
                echo -e "${green}          SSL 证书配置 (推荐)          ${plain}"
                echo -e "${green}═══════════════════════════════════════════${plain}"
                echo -e "${yellow}Let's Encrypt 现已同时支持域名和 IP 地址！${plain}"
                echo ""
                prompt_and_setup_ssl "${existing_port}" "${config_webBasePath}" "${server_ip}"
                echo -e "${green}访问地址:  https://${SSL_HOST}:${existing_port}/${config_webBasePath}${plain}"
            else
                # If a cert already exists, just show the access URL
                echo -e "${green}访问地址: https://${server_ip}:${existing_port}/${config_webBasePath}${plain}"
            fi
        fi
    else
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            echo -e "${yellow}检测到默认凭据, 需要安全更新...${plain}"
            ${xpanel_folder}/xpanel setting -username "${config_username}" -password "${config_password}"
            echo -e "已生成新的随机登录凭据:"
            echo -e "###############################################"
            echo -e "${green}用户名: ${config_username}${plain}"
            echo -e "${green}密码: ${config_password}${plain}"
            echo -e "###############################################"
        else
            echo -e "${green}用户名、密码和访问路径均已正确配置。${plain}"
        fi

        # Existing install: if no cert configured, prompt user for SSL setup
        # Properly detect empty cert by checking if cert: line exists and has content after it
        existing_cert=$(${xpanel_folder}/xpanel setting -getCert true | grep 'cert:' | awk -F': ' '{print $2}' | tr -d '[:space:]')
        if [[ -z "$existing_cert" ]]; then
            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}          SSL 证书配置 (推荐)          ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}Let's Encrypt 现已同时支持域名和 IP 地址！${plain}"
            echo ""
            prompt_and_setup_ssl "${existing_port}" "${existing_webBasePath}" "${server_ip}"
            echo -e "${green}访问地址:  https://${SSL_HOST}:${existing_port}/${existing_webBasePath}${plain}"
        else
            echo -e "${green}SSL 证书已配置, 无需操作。${plain}"
        fi
    fi

    ${xpanel_folder}/xpanel migrate
}

install_xpanel() {
    cd ${xpanel_folder%/xpanel}/

    # Download resources
    step "获取最新版本号"
    if [ $# == 0 ]; then
        tag_version=$(curl -Ls "https://api.github.com/repos/YCJE/XPanel/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$tag_version" ]]; then
            echo -e "${yellow}IPv6 方式获取版本失败, 尝试 IPv4...${plain}"
            tag_version=$(curl -4 -Ls "https://api.github.com/repos/YCJE/XPanel/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
            if [[ ! -n "$tag_version" ]]; then
                echo -e "${red}获取版本号失败（可能是 GitHub API 限制），请稍后重试${plain}"
                exit 1
            fi
        fi
        echo -e "获取到最新版本: ${tag_version}, 开始安装..."
        run_spinner "下载面板程序包 (${tag_version})" curl -4fsLRo ${xpanel_folder}-linux-$(arch).tar.gz https://github.com/YCJE/XPanel/releases/download/${tag_version}/xpanel-linux-$(arch).tar.gz
        if [[ $? -ne 0 ]]; then
            echo -e "${red}面板下载失败，请确认服务器可以访问 GitHub${plain}"
            exit 1
        fi
    else
        tag_version=$1
        tag_version_numeric=${tag_version#v}
        min_version="1.0.0"

        if [[ "$(printf '%s\n' "$min_version" "$tag_version_numeric" | sort -V | head -n1)" != "$min_version" ]]; then
            echo -e "${red}版本过低（至少需要 v2.3.5），退出安装。${plain}"
            exit 1
        fi

        url="https://github.com/YCJE/XPanel/releases/download/${tag_version}/xpanel-linux-$(arch).tar.gz"
        echo -e "开始安装 XPanel $1"
        run_spinner "下载面板程序包 ($1)" curl -4fsLRo ${xpanel_folder}-linux-$(arch).tar.gz ${url}
        if [[ $? -ne 0 ]]; then
            echo -e "${red}XPanel $1 下载失败，请确认该版本是否存在 ${plain}"
            exit 1
        fi
    fi
    run_spinner "下载管理脚本 xpanel.sh" curl -4fsLRo /usr/bin/xpanel-temp https://raw.githubusercontent.com/YCJE/XPanel/main/xpanel.sh
    if [[ $? -ne 0 ]]; then
        echo -e "${red}xpanel.sh 管理脚本下载失败${plain}"
        exit 1
    fi

    # Stop xpanel service and remove old resources
    step "停止旧服务并清理旧文件"
    if [[ -e ${xpanel_folder}/ ]]; then
        if [[ $release == "alpine" ]]; then
            rc-service xpanel stop
        else
            systemctl stop xpanel
        fi
        rm ${xpanel_folder}/ -rf
    fi

    # Extract resources and set permissions
    step "解压部署文件"
    run_spinner "解压程序包" tar zxf xpanel-linux-$(arch).tar.gz
    rm xpanel-linux-$(arch).tar.gz -f

    cd xpanel
    chmod +x xpanel
    chmod +x xpanel.sh

    # Check the system's architecture and rename the file accordingly
    if [[ $(arch) == "armv5" || $(arch) == "armv6" || $(arch) == "armv7" ]]; then
        mv bin/xray-linux-$(arch) bin/xray-linux-arm
        chmod +x bin/xray-linux-arm
    fi
    chmod +x xpanel bin/xray-linux-$(arch)

    # Update xpanel cli and se set permission
    mv -f /usr/bin/xpanel-temp /usr/bin/xpanel
    chmod +x /usr/bin/xpanel
    mkdir -p /var/log/xpanel
    step "初始化面板配置"
    config_after_install

    # Etckeeper compatibility
    step "注册系统服务并启动"
    if [ -d "/etc/.git" ]; then
        if [ -f "/etc/.gitignore" ]; then
            if ! grep -q "xpanel/xpanel.db" "/etc/.gitignore"; then
                echo "" >> "/etc/.gitignore"
                echo "xpanel/xpanel.db" >> "/etc/.gitignore"
                echo -e "${green}已将 xpanel.db 加入 /etc/.gitignore (etckeeper 兼容)${plain}"
            fi
        else
            echo "xpanel/xpanel.db" > "/etc/.gitignore"
            echo -e "${green}已创建 /etc/.gitignore 并加入 xpanel.db (etckeeper 兼容)${plain}"
        fi
    fi

    if [[ $release == "alpine" ]]; then
        curl -4fLRo /etc/init.d/xpanel https://raw.githubusercontent.com/YCJE/XPanel/main/xpanel.rc
        if [[ $? -ne 0 ]]; then
            echo -e "${red}xpanel.rc (OpenRC) 下载失败${plain}"
            exit 1
        fi
        chmod +x /etc/init.d/xpanel
        rc-update add xpanel
        rc-service xpanel start
    else
        # Install systemd service file
        service_installed=false

        if [ -f "xpanel.service" ]; then
            echo -e "${green}发现安装包内自带 xpanel.service，正在安装...${plain}"
            cp -f xpanel.service ${xpanel_service}/ > /dev/null 2>&1
            if [[ $? -eq 0 ]]; then
                service_installed=true
            fi
        fi

        if [ "$service_installed" = false ]; then
            case "${release}" in
                ubuntu | debian | armbian)
                    if [ -f "xpanel.service.debian" ]; then
                        echo -e "${green}发现安装包内自带 xpanel.service.debian，正在安装...${plain}"
                        cp -f xpanel.service.debian ${xpanel_service}/xpanel.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
                arch | manjaro | parch)
                    if [ -f "xpanel.service.arch" ]; then
                        echo -e "${green}发现安装包内自带 xpanel.service.arch，正在安装...${plain}"
                        cp -f xpanel.service.arch ${xpanel_service}/xpanel.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
                *)
                    if [ -f "xpanel.service.rhel" ]; then
                        echo -e "${green}发现安装包内自带 xpanel.service.rhel，正在安装...${plain}"
                        cp -f xpanel.service.rhel ${xpanel_service}/xpanel.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
            esac
        fi

        # If service file not found in tar.gz, download from GitHub
        if [ "$service_installed" = false ]; then
            echo -e "${yellow}安装包内未找到服务文件，从 GitHub 下载...${plain}"
            case "${release}" in
                ubuntu | debian | armbian)
                    curl -4fLRo ${xpanel_service}/xpanel.service https://raw.githubusercontent.com/YCJE/XPanel/main/xpanel.service.debian > /dev/null 2>&1
                    ;;
                arch | manjaro | parch)
                    curl -4fLRo ${xpanel_service}/xpanel.service https://raw.githubusercontent.com/YCJE/XPanel/main/xpanel.service.arch > /dev/null 2>&1
                    ;;
                *)
                    curl -4fLRo ${xpanel_service}/xpanel.service https://raw.githubusercontent.com/YCJE/XPanel/main/xpanel.service.rhel > /dev/null 2>&1
                    ;;
            esac

            if [[ $? -ne 0 ]]; then
                echo -e "${red}从 GitHub 安装服务文件失败${plain}"
                exit 1
            fi
            service_installed=true
        fi

        if [ "$service_installed" = true ]; then
            ok "systemd 服务单元已就绪"
            chown root:root ${xpanel_service}/xpanel.service > /dev/null 2>&1
            chmod 644 ${xpanel_service}/xpanel.service > /dev/null 2>&1
            systemctl daemon-reload
            systemctl enable xpanel > /dev/null 2>&1 && ok "已设置开机自启"
            systemctl start xpanel
        else
            echo -e "${red}服务文件安装失败${plain}"
            exit 1
        fi
    fi

    echo -e "${green}XPanel ${tag_version}${plain} 安装完成，服务已启动！"
    echo -e ""
    echo -e "┌───────────────────────────────────────────────────────┐
│  ${blue}xpanel 管理命令（子命令）:${plain}              │
│                                                       │
│  ${blue}xpanel${plain}              - 管理脚本（交互菜单）              │
│  ${blue}xpanel start${plain}        - 启动面板                          │
│  ${blue}xpanel stop${plain}         - 停止面板                          │
│  ${blue}xpanel restart${plain}      - 重启面板                          │
│  ${blue}xpanel status${plain}       - 查看运行状态                      │
│  ${blue}xpanel settings${plain}     - 查看当前配置                      │
│  ${blue}xpanel enable${plain}       - 设置开机自启                      │
│  ${blue}xpanel disable${plain}      - 取消开机自启                      │
│  ${blue}xpanel log${plain}          - 查看面板日志                      │
│  ${blue}xpanel banlog${plain}       - 查看 Fail2ban 封禁日志            │
│  ${blue}xpanel update${plain}       - 更新面板                          │
│  ${blue}xpanel legacy${plain}       - 旧版命令                          │
│  ${blue}xpanel install${plain}      - 安装                              │
│  ${blue}xpanel uninstall${plain}    - 卸载                              │
└───────────────────────────────────────────────────────┘"
}

echo -e "${cyan}═══════════════════════════════════════════"
echo -e "         XPanel 一键安装脚本"
echo -e "     Xray 代理 + gost 转发 一体化面板"
echo -e "═══════════════════════════════════════════${plain}"
install_base || { fail "系统依赖安装失败，请检查软件源配置"; exit 1; }
install_xpanel $1
