#!/bin/bash
# ============================================
# Computing Power - cpstart WSL2 启动脚本
# 在 WSL2 环境中启动 cpstart + containerd
# ============================================
set -euo pipefail

# 配置
CPSTART_BIN="${CPSTART_BIN:-/opt/cp-agent/bin/cpstart}"
CPSTART_CONFIG="${CPSTART_CONFIG:-/opt/cp-agent/configs/cpstart.yaml}"
CONTAINERD_SOCKET="/run/containerd/containerd.sock"

echo "========================================"
echo "  Computing Power - cpstart (WSL2)"
echo "========================================"
echo ""

# Step 1: 检查 containerd
echo "[1/3] 检查 containerd..."

# 如果 containerd 未运行，启动它
if [ ! -S "$CONTAINERD_SOCKET" ]; then
    echo "  containerd 未运行，正在启动..."

    # 尝试 systemd 方式
    if command -v systemctl &>/dev/null && systemctl is-enabled containerd &>/dev/null 2>&1; then
        echo "  使用 systemctl 启动 containerd..."
        sudo systemctl start containerd
    else
        echo "  使用 containerd 命令直接启动..."
        sudo containerd &
        # 等待 containerd 启动
        for i in $(seq 1 10); do
            if [ -S "$CONTAINERD_SOCKET" ]; then
                break
            fi
            sleep 1
        done
    fi
fi

# 验证 containerd 连通性
if ! ctr version &>/dev/null; then
    echo "  [ERROR] containerd 无法连接。请手动检查:" >&2
    echo "    sudo systemctl status containerd" >&2
    echo "    或: wsl --shutdown; wsl -d <distro>" >&2
    exit 1
fi
echo "  [OK] containerd 运行中"

# Step 2: 检查 cpstart 二进制
echo "[2/3] 检查 cpstart..."
if [ ! -f "$CPSTART_BIN" ]; then
    echo "  [ERROR] cpstart 二进制未找到: $CPSTART_BIN" >&2
    echo "  请将二进制文件复制到 WSL2:"
    echo "    cp bin/cpstart-linux \\\\wsl$\\$(lsb_release -is 2>/dev/null || echo 'Ubuntu-24.04')$CPSTART_BIN"
    exit 1
fi
chmod +x "$CPSTART_BIN"
echo "  [OK] cpstart: $CPSTART_BIN"

# 检查配置文件
if [ ! -f "$CPSTART_CONFIG" ]; then
    echo "  [WARN] 配置文件未找到: $CPSTART_CONFIG" >&2
    echo "  将使用默认配置启动（首次运行会进入设置向导）" >&2
fi

# Step 3: 启动 cpstart
echo "[3/3] 启动 cpstart..."
echo ""
echo "  Web UI 地址: http://localhost:8080"
echo "  （WSL2 自动转发端口到 Windows）"
echo ""

# 如果配置了 auto_open=true，但在 WSL2 中无浏览器，设置环境变量让 cpstart 检测
export CP_WSL2=1

exec "$CPSTART_BIN" --config "$CPSTART_CONFIG"