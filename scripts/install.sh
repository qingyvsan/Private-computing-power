#!/bin/bash
# scripts/install.sh — Linux/macOS 一键安装 Computing Power Agent
# 用法: curl -fsSL https://update.computing-power.local/install.sh | bash
#        INVITE_CODE=xxxx bash scripts/install.sh
#        VERSION=1.0.0 bash scripts/install.sh

set -euo pipefail

# ====== 配置 ======
VERSION="${VERSION:-latest}"
BASE_URL="${BASE_URL:-https://update.computing-power.local/v1}"
INSTALL_DIR="${INSTALL_DIR:-/opt/cp-agent}"
INVITE_CODE="${INVITE_CODE:-}"

# ====== 颜色 ======
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ====== 检测平台 ======
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        *)      error "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)            error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    echo "${os}-${arch}"
}

# ====== 下载 ======
download() {
    local url="$1" out="$2"
    if command -v curl &>/dev/null; then
        curl -fsSL -o "$out" "$url"
    elif command -v wget &>/dev/null; then
        wget -qO "$out" "$url"
    else
        error "curl or wget required"
        exit 1
    fi
}

# ====== 主流程 ======
main() {
    local platform
    platform=$(detect_platform)
    info "Detected platform: ${platform}"
    info "Install dir: ${INSTALL_DIR}"

    # 确定下载 URL
    local manifest_url="${BASE_URL}/manifest.json"
    local core_url

    if [ "$VERSION" = "latest" ]; then
        # 下载 manifest 获取最新版本
        local tmp_manifest
        tmp_manifest=$(mktemp)
        download "$manifest_url" "$tmp_manifest"
        VERSION=$(python3 -c "import json; print(json.load(open('${tmp_manifest}'))['version'])" 2>/dev/null || \
                  grep -o '"version"[^,]*' "$tmp_manifest" | head -1 | cut -d'"' -f4)
        rm -f "$tmp_manifest"
        info "Latest version: ${VERSION}"
    fi

    core_url="${BASE_URL}/releases/${VERSION}/${platform}/core.tar.gz"

    # 下载并解压
    local tmp_dir
    tmp_dir=$(mktemp -d)
    info "Downloading core package from ${core_url}..."

    download "$core_url" "${tmp_dir}/core.tar.gz"

    info "Extracting..."
    mkdir -p "${INSTALL_DIR}"
    tar xzf "${tmp_dir}/core.tar.gz" -C "${INSTALL_DIR}"

    # 写入配置
    local config_dir="${INSTALL_DIR}/configs"
    mkdir -p "${config_dir}"

    if [ ! -f "${config_dir}/cpstart.yaml" ]; then
        cat > "${config_dir}/cpstart.yaml" << EOF
agent:
  name: "$(hostname)"
  data_dir: "${INSTALL_DIR}/data"
scheduler:
  address: "8.138.108.183:9090"
resources:
  max_cpu_cores: 0
  max_memory_mb: 0
  report_gpu: true
console:
  port: 8080
  auto_open: true
EOF
        if [ -n "$INVITE_CODE" ]; then
            echo "invite_code: \"${INVITE_CODE}\"" >> "${config_dir}/cpstart.yaml"
        fi
        info "Config written to ${config_dir}/cpstart.yaml"
    else
        warn "Config exists, skipping"
    fi

    # 安装服务
    case "$(uname -s)" in
        Linux)
            install_linux
            ;;
        Darwin)
            install_darwin
            ;;
    esac

    rm -rf "${tmp_dir}"
    info "Installation complete!"
    info "Agent installed at: ${INSTALL_DIR}"
    info "Run: sudo systemctl start cp-agent  (Linux)"
    info "Run: sudo launchctl load /Library/LaunchDaemons/com.computing-power.agent.plist  (macOS)"
}

install_linux() {
    info "Installing systemd service..."
    local service_src="${INSTALL_DIR}/core/cp-agent.service"
    local service_dst="/etc/systemd/system/cp-agent.service"

    # 检查服务文件来源
    if [ ! -f "$service_src" ]; then
        # 可能在 core 子目录外
        service_src="${INSTALL_DIR}/cp-agent.service"
    fi

    if [ -f "$service_src" ]; then
        cp "$service_src" "$service_dst"
    else
        # 生成服务文件
        cat > "$service_dst" << SERVICEEOF
[Unit]
Description=Computing Power Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/bin/cpstart --config ${INSTALL_DIR}/configs/cpstart.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SERVICEEOF
    fi

    systemctl daemon-reload
    systemctl enable cp-agent
    systemctl start cp-agent
    info "Service cp-agent started"
}

install_darwin() {
    info "Installing launchd service..."
    local plist_dst="/Library/LaunchDaemons/com.computing-power.agent.plist"
    local plist_src="${INSTALL_DIR}/core/cp-agent.plist"

    if [ -f "$plist_src" ]; then
        cp "$plist_src" "$plist_dst"
    else
        warn "No plist template found, generating..."
        cat > "$plist_dst" << PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.computing-power.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/bin/cpstart</string>
        <string>--config</string>
        <string>${INSTALL_DIR}/configs/cpstart.yaml</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
PLISTEOF
    fi

    chown root:wheel "$plist_dst"
    launchctl load -w "$plist_dst"
    info "Service com.computing-power.agent loaded"
}

main "$@"