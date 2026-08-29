# scripts/install.ps1 — Windows 一键安装 Computing Power Agent
# 用法: powershell -ExecutionPolicy Bypass -File install.ps1
#       $env:INVITE_CODE = "xxxx"; powershell -ExecutionPolicy Bypass -File install.ps1

param(
    [string]$Version = "latest",
    [string]$BaseUrl = "https://update.computing-power.local/v1",
    [string]$InstallDir = "$env:ProgramFiles\ComputingPower",
    [string]$DataDir = "$env:ProgramData\ComputingPower",
    [string]$InviteCode = ""
)

$ErrorActionPreference = "Stop"

function Write-Info  { Write-Host "[INFO] $args" -ForegroundColor Green }
function Write-Warn  { Write-Host "[WARN] $args" -ForegroundColor Yellow }
function Write-Error { Write-Host "[ERROR] $args" -ForegroundColor Red }

# 检测平台
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$platform = "windows-$arch"
Write-Info "Detected platform: $platform"

# 获取最新版本
if ($Version -eq "latest") {
    Write-Info "Fetching latest version..."
    $manifestUrl = "$BaseUrl/manifest.json"
    $manifest = Invoke-RestMethod -Uri $manifestUrl
    $Version = $manifest.version
    Write-Info "Latest version: $Version"
}

# 下载 core 包
$coreUrl = "$BaseUrl/releases/$Version/$platform/core.tar.gz"
$tmpDir = "$env:TEMP\cp-agent-install"
$coreTarball = "$tmpDir\core.tar.gz"

Write-Info "Downloading core package from $coreUrl..."
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
Invoke-WebRequest -Uri $coreUrl -OutFile $coreTarball

# 解压（使用 tar，Windows 10 1803+ 内置）
Write-Info "Extracting to $InstallDir..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
tar xzf $coreTarball -C $InstallDir

# 写入配置
$configDir = "$DataDir\configs"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
$configPath = "$configDir\cpstart.yaml"

if (-not (Test-Path $configPath)) {
    $configContent = @"
agent:
  name: "$env:COMPUTERNAME"
  data_dir: "$DataDir\data"
scheduler:
  address: "8.138.108.183:9090"
resources:
  max_cpu_cores: 0
  max_memory_mb: 0
  report_gpu: true
console:
  port: 8080
  auto_open: true
"@
    if ($InviteCode) {
        $configContent += "`ninvite_code: `"$InviteCode`""
    }
    Set-Content -Path $configPath -Value $configContent
    Write-Info "Config written to $configPath"
} else {
    Write-Warn "Config exists, skipping"
}

# 注册 Windows 服务
$binaryPath = "$InstallDir\bin\cpstart.exe"
if (Get-Service "ComputingPowerAgent" -ErrorAction SilentlyContinue) {
    Write-Warn "Service ComputingPowerAgent already exists, restarting..."
    Restart-Service "ComputingPowerAgent" -ErrorAction SilentlyContinue
} else {
    Write-Info "Registering Windows service..."
    New-Service -Name "ComputingPowerAgent" `
        -BinaryPathName "`"$binaryPath`" --config `"$configPath`"" `
        -DisplayName "Computing Power Agent" `
        -Description "Private Computing Power node agent" `
        -StartupType Automatic
    Start-Service -Name "ComputingPowerAgent"
    Write-Info "Service ComputingPowerAgent started"
}

# 清理
Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue

Write-Info "Installation complete!"
Write-Info "Agent installed at: $InstallDir"