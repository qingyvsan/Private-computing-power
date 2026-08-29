#!/bin/bash
# scripts/gen-installers.sh — 生成内嵌版本号的安装脚本
# 用法: scripts/gen-installers.sh <version>
# 输出: dist/<version>/install.sh, dist/<version>/install.ps1

set -euo pipefail

VERSION="${1:?version required}"
DIST_DIR="dist/${VERSION}"

mkdir -p "${DIST_DIR}"

# 生成 install.sh
sed "s|VERSION=latest|VERSION=${VERSION}|g" scripts/install.sh > "${DIST_DIR}/install.sh"
chmod +x "${DIST_DIR}/install.sh"

# 生成 install.ps1
sed "s|param(\$Version = \"latest\"|param(\$Version = \"${VERSION}\"|" scripts/install.ps1 > "${DIST_DIR}/install.ps1"

echo ">>> Installers written to ${DIST_DIR}/"