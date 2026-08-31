<#
.SYNOPSIS
    Computing Power - WSL2 环境自动配置脚本
.DESCRIPTION
    在 Windows 上配置 WSL2 环境，安装 containerd 和 NVIDIA Container Toolkit，
    为 cpstart 提供容器运行时支持。
.NOTES
    需要管理员权限运行部分操作。建议以管理员身份打开 PowerShell 执行。
#>

$ErrorActionPreference = "Stop"
$DistroName = "Ubuntu-24.04"
$CpAgentDir = "/opt/cp-agent"
$CpAgentBinDir = "/opt/cp-agent/bin"
$CpAgentConfigDir = "/opt/cp-agent/configs"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Computing Power - WSL2 环境配置" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# ========== 检查管理员权限 ==========
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "[!] 建议以管理员身份运行此脚本，否则部分操作可能失败。" -ForegroundColor Yellow
    Write-Host "    继续运行，但遇到错误时请尝试以管理员身份重新启动。" -ForegroundColor Yellow
    Write-Host ""
}

# ========== Step 1: 检查 WSL2 ==========
Write-Host "[1/6] 检查 WSL2 状态..." -ForegroundColor Green

$wslInstalled = $false
try {
    $wslVersion = wsl --version 2>&1
    if ($LASTEXITCODE -eq 0) {
        $wslInstalled = $true
        Write-Host "  [OK] WSL 已安装: $($wslVersion -split "`n" | Select-Object -First 1)" -ForegroundColor Green
    }
} catch {}

if (-not $wslInstalled) {
    Write-Host "  [!] WSL 未安装，正在安装..." -ForegroundColor Yellow
    Write-Host "  执行: wsl --install -d $DistroName"
    Write-Host "  (安装完成后系统可能需要重启)" -ForegroundColor Yellow
    wsl --install -d $DistroName
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  [ERROR] WSL 安装失败。请手动安装:" -ForegroundColor Red
        Write-Host "    wsl --install -d $DistroName" -ForegroundColor Red
        exit 1
    }
    Write-Host "  [OK] WSL 安装完成" -ForegroundColor Green
}

# ========== Step 2: 设置 WSL2 默认版本 ==========
Write-Host "[2/6] 设置 WSL2 为默认版本..." -ForegroundColor Green
try {
    wsl --set-default-version 2
    Write-Host "  [OK] WSL2 已设为默认" -ForegroundColor Green
} catch {
    Write-Host "  [OK] WSL2 已启用" -ForegroundColor Green
}

# ========== Step 3: 检查/安装分发版 ==========
Write-Host "[3/6] 检查 WSL 分发版..." -ForegroundColor Green

$distroExists = $false
try {
    $distros = wsl --list --verbose 2>&1
    if ($distros -match $DistroName) {
        $distroExists = $true
        Write-Host "  [OK] 分发版 $DistroName 已存在" -ForegroundColor Green
    }
} catch {}

if (-not $distroExists) {
    Write-Host "  正在安装 $DistroName ..." -ForegroundColor Yellow
    Write-Host "  执行: wsl --install -d $DistroName"
    wsl --install -d $DistroName
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  [ERROR] 安装失败。请手动安装:" -ForegroundColor Red
        Write-Host "    wsl --install -d $DistroName" -ForegroundColor Red
        exit 1
    }
    Write-Host "  [OK] $DistroName 安装完成" -ForegroundColor Green
}

# ========== Step 4: 在 WSL2 中安装 containerd ==========
Write-Host "[4/6] 在 WSL2 中安装 containerd..." -ForegroundColor Green

$wslSetupScript = @'
#!/bin/bash
set -euo pipefail

echo "  [WSL] 更新软件包列表..."
apt-get update -qq

echo "  [WSL] 安装依赖..."
apt-get install -y -qq ca-certificates curl gnupg lsb-release

# 检查 containerd 是否已安装
if command -v containerd &> /dev/null; then
    echo "  [OK] containerd 已安装: $(containerd --version)"
else
    echo "  [WSL] 安装 containerd..."
    # 添加 Docker 官方源
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update -qq
    apt-get install -y -qq containerd

    # 配置 containerd 使用 systemd cgroup
    mkdir -p /etc/containerd
    containerd config default | tee /etc/containerd/config.toml > /dev/null
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

    echo "  [OK] containerd 安装完成"
fi

# 创建 cp-agent 目录
mkdir -p /opt/cp-agent/bin
mkdir -p /opt/cp-agent/configs
mkdir -p /opt/cp-agent/data
echo "  [OK] 目录结构已创建: /opt/cp-agent/"

# 启用 systemd 支持（WSL2 + Ubuntu 24.04+）
WSL_CONF="/etc/wsl.conf"
if [ -f "$WSL_CONF" ] && grep -q "systemd=true" "$WSL_CONF" 2>/dev/null; then
    echo "  [OK] systemd 已启用"
else
    echo "  [WSL] 启用 systemd..."
    echo -e "[boot]\nsystemd=true" > "$WSL_CONF"
    echo "  [WARN] systemd 已配置。需要重启 WSL2 后生效。" >&2
fi

# 启动 containerd（立即生效）
if systemctl is-active containerd &>/dev/null; then
    echo "  [OK] containerd 运行中"
else
    systemctl start containerd 2>/dev/null || containerd &
    echo "  [WSL] containerd 已启动"
fi

# 验证 containerd
if ctr version &>/dev/null; then
    echo "  [OK] containerd 连通性正常"
else
    echo "  [WARN] containerd 未响应，请稍后检查: sudo systemctl status containerd" >&2
fi
'@

# 在 WSL2 中执行安装脚本
$wslOutput = $wslSetupScript | wsl -d $DistroName -e bash -e 2>&1
$wslExitCode = $LASTEXITCODE
$wslOutput | ForEach-Object { Write-Host "  $_" }

if ($wslExitCode -ne 0) {
    Write-Host "  [WARN] WSL 安装遇到问题，但可能部分完成。" -ForegroundColor Yellow
} else {
    Write-Host "  [OK] containerd 安装完成" -ForegroundColor Green
}

# ========== Step 5: 安装 NVIDIA Container Toolkit（条件执行） ==========
Write-Host "[5/6] 检查 GPU 并安装 NVIDIA 支持..." -ForegroundColor Green

$hasNvidia = $false
try {
    $nvidiaSmi = & "nvidia-smi" 2>&1
    if ($LASTEXITCODE -eq 0) {
        $hasNvidia = $true
        Write-Host "  [OK] 检测到 NVIDIA GPU" -ForegroundColor Green
    }
} catch {
    Write-Host "  [SKIP] 未检测到 NVIDIA GPU，跳过 GPU 配置" -ForegroundColor Yellow
}

if ($hasNvidia) {
    $wslNvidiaScript = @'
#!/bin/bash
set -euo pipefail

# 检查 nvidia-smi 是否在 WSL2 中可用
if command -v nvidia-smi &> /dev/null && nvidia-smi &>/dev/null; then
    echo "  [OK] WSL2 中 nvidia-smi 可用"
else
    echo "  [WARN] WSL2 中 nvidia-smi 不可用。请确保:" >&2
    echo "        1. Windows 已安装最新 NVIDIA 驱动程序（版本 470+）" >&2
    echo "        2. 重启 WSL2: wsl --shutdown" >&2
fi

# 检查 nvidia-container-toolkit 是否已安装
if command -v nvidia-ctk &> /dev/null; then
    echo "  [OK] nvidia-container-toolkit 已安装"
else
    echo "  [WSL] 安装 nvidia-container-toolkit..."
    curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
    distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
    curl -s -L https://nvidia.github.io/libnvidia-container/$distribution/libnvidia-container.list | \
        sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
        tee /etc/apt/sources.list.d/nvidia-container-toolkit.list > /dev/null
    apt-get update -qq
    apt-get install -y -qq nvidia-container-toolkit

    # 配置 containerd 运行时
    nvidia-ctk runtime configure --runtime=containerd
    nvidia-ctk config --set nvidia-container-cli.no-cgroups --in-place

    # 重启 containerd
    systemctl restart containerd 2>/dev/null || pkill containerd && containerd &

    echo "  [OK] nvidia-container-toolkit 安装完成"
fi

# 验证 GPU 容器支持
echo "  [WSL] 验证 GPU 支持..."
if ctr run --rm --gpus 0 docker.io/nvidia/cuda:12.6.0-base-ubuntu22.00 nvidia-smi nvidia-smi 2>/dev/null; then
    echo "  [OK] GPU 容器支持正常"
else
    echo "  [WARN] GPU 容器测试跳过（可能在 WSL2 中需要额外配置）" >&2
fi
'@

    $wslNvidiaOutput = $wslNvidiaScript | wsl -d $DistroName -e bash -e 2>&1
    $wslNvidiaExitCode = $LASTEXITCODE
    $wslNvidiaOutput | ForEach-Object { Write-Host "  $_" }
}

# ========== Step 6: 输出摘要 ==========
Write-Host "[6/6] 配置完成" -ForegroundColor Green
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  WSL2 环境配置完成" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "下一步操作：" -ForegroundColor Cyan
Write-Host ""
Write-Host "  1. 如果 systemd 配置有更新，重启 WSL2:" -ForegroundColor White
Write-Host "     wsl --shutdown"
Write-Host "     wsl -d $DistroName"
Write-Host ""
Write-Host "  2. 构建 cpstart Linux 版本:" -ForegroundColor White
Write-Host "     cd <project_root>"
Write-Host "     make build-cpstart-linux"
Write-Host ""
Write-Host "  3. 将二进制文件复制到 WSL2:" -ForegroundColor White
Write-Host "     wsl -d $DistroName -e bash -c 'mkdir -p $CpAgentBinDir'"
Write-Host "     cp bin/cpstart-linux \\\\wsl$\\$DistroName$CpAgentBinDir\\cpstart"
Write-Host "     wsl -d $DistroName -e bash -c 'chmod +x $CpAgentBinDir/cpstart'"
Write-Host ""
Write-Host "  4. 复制配置文件:" -ForegroundColor White
Write-Host "     cp agent/configs/cpstart.yaml \\\\wsl$\\$DistroName$CpAgentConfigDir\\"
Write-Host ""
Write-Host "  5. 在 WSL2 中启动 cpstart:" -ForegroundColor White
Write-Host "     wsl -d $DistroName"
Write-Host "     $CpAgentBinDir/cpstart --config $CpAgentConfigDir/cpstart.yaml"
Write-Host ""
Write-Host "  6. 从 Windows 浏览器访问:" -ForegroundColor White
Write-Host "     http://localhost:8080"
Write-Host ""
Write-Host "  详细说明请参考 docs/wsl2-setup.md" -ForegroundColor Cyan
Write-Host ""