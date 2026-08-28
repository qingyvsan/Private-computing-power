#!/bin/bash
# deploy.sh — 部署调度器到远程服务器
# 用法: ./deploy.sh <服务器IP>

set -euo pipefail

SERVER="${1:-8.138.108.183}"
echo "=== 部署调度器到 ${SERVER} ==="

# 1. 创建目录结构
echo "--- 创建目录结构 ---"
ssh root@"${SERVER}" "mkdir -p /opt/cp-scheduler/{bin,configs,data/backups}"

# 2. 上传二进制
echo "--- 上传二进制 ---"
scp bin/scheduler-linux-amd64 root@"${SERVER}":/opt/cp-scheduler/bin/scheduler
scp bin/cpcli-linux-amd64  root@"${SERVER}":/opt/cp-scheduler/bin/cpcli
ssh root@"${SERVER}" "chmod +x /opt/cp-scheduler/bin/scheduler /opt/cp-scheduler/bin/cpcli"

# 3. 上传配置
echo "--- 上传配置 ---"
scp deploy/scheduler.yaml root@"${SERVER}":/opt/cp-scheduler/configs/scheduler.yaml

# 4. 生成 CA 证书（首次部署）
echo "--- 生成 CA 证书 ---"
ssh root@"${SERVER}" << 'CAEOF'
  if [ ! -f /opt/cp-scheduler/configs/ca.pem ]; then
    /opt/cp-scheduler/bin/scheduler --version 2>/dev/null || true
    # 用 openssl 生成 CA 证书（用于后续 TLS/Nebula）
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
      -keyout /opt/cp-scheduler/configs/ca-key.pem \
      -out /opt/cp-scheduler/configs/ca.pem \
      -days 3650 -nodes -subj "/CN=ComputingPower Root CA/O=ComputingPower"
    echo "CA 证书已生成"
  else
    echo "CA 证书已存在，跳过"
  fi
CAEOF

# 5. 安装 systemd 服务
echo "--- 安装 systemd 服务 ---"
scp deploy/cp-scheduler.service root@"${SERVER}":/etc/systemd/system/cp-scheduler.service
ssh root@"${SERVER}" "systemctl daemon-reload"

# 6. 部署 Nebula Lighthouse
echo "--- 部署 Nebula Lighthouse ---"

# 6.1 下载 Nebula 二进制（如果不存在）
ssh root@"${SERVER}" << 'NEBULAEOF'
  if [ ! -f /opt/cp-scheduler/bin/nebula ]; then
    NEBULA_VERSION="1.9.4"
    NEBULA_URL="https://github.com/slackhq/nebula/releases/download/v${NEBULA_VERSION}/nebula-linux-amd64.tar.gz"
    echo "下载 Nebula ${NEBULA_VERSION}..."
    curl -sL "$NEBULA_URL" -o /tmp/nebula.tar.gz
    tar xzf /tmp/nebula.tar.gz -C /opt/cp-scheduler/bin/ nebula
    chmod +x /opt/cp-scheduler/bin/nebula
    rm -f /tmp/nebula.tar.gz
    echo "Nebula 二进制已安装: $(/opt/cp-scheduler/bin/nebula -version 2>&1 || true)"
  else
    echo "Nebula 二进制已存在，跳过下载"
  fi
NEBULAEOF

# 6.2 创建 Nebula 配置目录
ssh root@"${SERVER}" "mkdir -p /opt/cp-scheduler/configs/nebula"

# 6.3 生成 Lighthouse 配置（首次部署）
ssh root@"${SERVER}" << 'LHEOF'
  if [ ! -f /opt/cp-scheduler/configs/nebula/lighthouse.yml ]; then
    cat > /opt/cp-scheduler/configs/nebula/lighthouse.yml << 'YAMLEOF'
pki:
  ca: /opt/cp-scheduler/configs/nebula/ca.crt
  cert: /opt/cp-scheduler/configs/nebula/lighthouse.crt
  key: /opt/cp-scheduler/configs/nebula/lighthouse.key

lighthouse:
  am_lighthouse: true
  interval: 60

listen:
  host: 0.0.0.0
  port: 4242

punchy:
  punch: true
  respond: true

relay:
  am_relay: false

tun:
  disabled: true

firewall:
  outbound_action: accept
  inbound_action: drop
  default_action: drop
  conntrack:
    tcp_timeout: 12m
    udp_timeout: 3m
    default_timeout: 10m
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - port: any
      proto: icmp
      host: any
YAMLEOF
    echo "Lighthouse 配置已生成"
  else
    echo "Lighthouse 配置已存在，跳过"
  fi
LHEOF

# 6.4 生成 Lighthouse 证书（如果 CA 已存在）
ssh root@"${SERVER}" << 'CERTEOF'
  CA_CERT="/opt/cp-scheduler/configs/nebula/ca.crt"
  LIGHTHOUSE_CRT="/opt/cp-scheduler/configs/nebula/lighthouse.crt"
  if [ ! -f "$LIGHTHOUSE_CRT" ] && [ -f "$CA_CERT" ]; then
    # 使用调度器内置的 CA 签发 Lighthouse 证书
    # 调度器启动后会自动创建 CA，这里用 openssl 生成临时证书
    echo "Lighthouse 证书将由调度器 CA 自动签发，使用临时自签名证书占位"
    # 生成临时 ECDSA 密钥和自签名证书
    openssl ecparam -genkey -name prime256v1 -out /opt/cp-scheduler/configs/nebula/lighthouse.key
    openssl req -new -key /opt/cp-scheduler/configs/nebula/lighthouse.key \
      -x509 -days 3650 -out /opt/cp-scheduler/configs/nebula/lighthouse.crt \
      -subj "/CN=lighthouse/OU=ComputingPower"
    echo "Lighthouse 临时证书已生成"
  elif [ -f "$LIGHTHOUSE_CRT" ]; then
    echo "Lighthouse 证书已存在，跳过"
  else
    echo "CA 证书不存在，跳过 Lighthouse 证书生成（调度器首次启动后会自动创建）"
  fi
CERTEOF

# 6.5 安装 lighthouse systemd 服务
echo "--- 安装 lighthouse systemd 服务 ---"
scp deploy/nebula-lighthouse.service root@"${SERVER}":/etc/systemd/system/nebula-lighthouse.service
ssh root@"${SERVER}" "systemctl daemon-reload"

# 6.6 开放防火墙端口
echo "--- 开放防火墙端口 ---"
ssh root@"${SERVER}" "ufw allow 4242/udp 2>/dev/null || firewall-cmd --add-port=4242/udp --permanent 2>/dev/null || echo 'firewall not configured, skip'"

# 6.7 启动 Lighthouse
echo "--- 启动 Lighthouse ---"
ssh root@"${SERVER}" "systemctl enable nebula-lighthouse && systemctl restart nebula-lighthouse"

# 7. 启动调度器
echo "--- 启动调度器 ---"
ssh root@"${SERVER}" "systemctl enable cp-scheduler && systemctl restart cp-scheduler"

echo ""
echo "=== 部署完成 ==="
echo "检查状态: ssh root@${SERVER} 'systemctl status cp-scheduler'"
echo "查看日志: ssh root@${SERVER} 'journalctl -u cp-scheduler -f'"
echo "CLI 测试: ./bin/cpcli -s ${SERVER}:9090 node list"