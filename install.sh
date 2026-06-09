#!/bin/bash
# Block Area Bot 一键安装脚本
# 用法:
#   安装最新 beta 版本:
#     curl -fsSL https://raw.githubusercontent.com/PIKACHUIM/BlockAreaBot/main/install.sh | bash
#   指定版本安装:
#     curl -fsSL https://raw.githubusercontent.com/PIKACHUIM/BlockAreaBot/main/install.sh | bash -s -- --version 1.0.0
#     curl -fsSL https://raw.githubusercontent.com/PIKACHUIM/BlockAreaBot/main/install.sh | bash -s -- -v 1.0.0

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# GitHub 仓库地址
REPO="PIKACHUIM/BlockAreaBot"
BINARY_NAME="block"
SERVICE_NAME="block-area-bot"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/block-area-bot"
DATA_DIR="/var/lib/block-area-bot"
LOG_DIR="/var/log/block-area-bot"
GH_URL="github.524228.xyz"
# 用户指定的版本号（为空则自动获取 beta）
USER_VERSION=""

# 检查 root 权限
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        error "此脚本需要 root 权限运行，请使用 sudo"
    fi
}

# 检测系统架构
detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "不支持的架构: $ARCH"
            ;;
    esac
    info "检测到架构: $ARCH"
}

# 检测发行版
detect_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        DISTRO=$ID
    elif [ -f /etc/redhat-release ]; then
        DISTRO="rhel"
    else
        DISTRO="unknown"
    fi
    info "检测到发行版: $DISTRO"
}

# 检查依赖
check_deps() {
    local missing=""
    
    if ! command -v iptables &>/dev/null; then
        missing="$missing iptables"
    fi
    
    if ! command -v ipset &>/dev/null; then
        missing="$missing ipset"
    fi
    
    if [ -n "$missing" ]; then
        warn "缺少依赖:$missing"
        info "正在安装依赖..."
        
        case "$DISTRO" in
            ubuntu|debian)
                apt-get update -qq && apt-get install -y -qq iptables ipset
                ;;
            centos|rhel|fedora|rocky|alma)
                yum install -y iptables ipset 2>/dev/null || dnf install -y iptables ipset
                ;;
            arch|manjaro)
                pacman -Sy --noconfirm iptables ipset
                ;;
            *)
                error "无法自动安装依赖，请手动安装: iptables ipset"
                ;;
        esac
    fi
    
    info "依赖检查通过"
}

# 解析命令行参数
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -v|--version)
                USER_VERSION="$2"
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done
}

# 获取版本号（优先使用用户指定版本，否则拉取 beta release）
get_version() {
    if [ -n "$USER_VERSION" ]; then
        # 用户指定版本，去掉可能的 v 前缀
        VERSION=$(echo "$USER_VERSION" | sed 's/^v//')
        info "使用指定版本: v$VERSION"
    else
        # 默认拉取 beta release（tag 为 beta）
        info "正在获取 beta 版本信息..."
        VERSION=$(curl -fsSL "https://api.${GH_URL}/repos/${REPO}/releases/tags/beta" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
            # 如果没有 beta release，尝试获取最新 release
            warn "未找到 beta 版本，尝试获取最新正式版本..."
            VERSION=$(curl -fsSL "https://api.${GH_URL}/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        fi
        if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
            error "无法获取版本号，请使用 --version 手动指定"
        fi
        info "获取到版本: $VERSION"
    fi
    # 标准化：确保 VERSION 不含 v 前缀（用于拼接下载 URL）
    VERSION_TAG="$VERSION"
    VERSION=$(echo "$VERSION" | sed 's/^v//')
}

# 下载并安装
install_binary() {
    # 文件名格式与 GitHub Actions 构建产物一致: block-linux-<arch>.tar.gz
    local url="https://${GH_URL}/${REPO}/releases/download/${VERSION_TAG}/block-linux-${ARCH}.tar.gz"
    local tmp_dir=$(mktemp -d)
    
    info "正在下载: $url"
    curl -fsSL "$url" -o "${tmp_dir}/release.tar.gz" || error "下载失败"
    
    info "正在解压..."
    tar -xzf "${tmp_dir}/release.tar.gz" -C "${tmp_dir}"
    
    # 安装二进制文件
    install -Dm755 "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    info "二进制文件已安装到 ${INSTALL_DIR}/${BINARY_NAME}"
    
    # 安装 systemd service
    if [ -f "${tmp_dir}/dist/block-area-bot.service" ]; then
        install -Dm644 "${tmp_dir}/dist/block-area-bot.service" "/etc/systemd/system/${SERVICE_NAME}.service"
    fi
    
    # 安装默认配置
    if [ ! -f "${CONFIG_DIR}/config.json" ]; then
        mkdir -p "${CONFIG_DIR}"
        if [ -f "${tmp_dir}/dist/config.json" ]; then
            install -Dm644 "${tmp_dir}/dist/config.json" "${CONFIG_DIR}/config.json"
        else
            echo '{"repos":[],"rules":[],"crons":[],"next_id":1,"next_rule":1}' > "${CONFIG_DIR}/config.json"
        fi
    fi
    
    # 创建数据目录
    mkdir -p "${DATA_DIR}"
    mkdir -p "${LOG_DIR}"
    
    # 清理
    rm -rf "${tmp_dir}"
    
    # 重新加载 systemd
    systemctl daemon-reload
    
    info "安装完成!"
}

# 显示安装后信息
show_info() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo " Block Area Bot 安装成功!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo " 版本:    v${VERSION}"
    echo " 二进制:  ${INSTALL_DIR}/${BINARY_NAME}"
    echo " 配置:    ${CONFIG_DIR}/config.json"
    echo " 数据:    ${DATA_DIR}/"
    echo " 日志:    ${LOG_DIR}/"
    echo ""
    echo " 快速开始:"
    echo "   block repo add --type apnic:cn        # 添加中国 IP 段"
    echo "   block rule ban cn                     # 屏蔽中国 IP"
    echo "   block cron add cn 3d                  # 每 3 天更新"
    echo "   block enable && block start           # 启动服务"
    echo ""
    echo " 更多命令: block --help"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# 主流程
main() {
    info "Block Area Bot 安装脚本"
    echo ""
    
    parse_args "$@"
    check_root
    detect_arch
    detect_distro
    check_deps
    get_version
    install_binary
    show_info
}

main "$@"
