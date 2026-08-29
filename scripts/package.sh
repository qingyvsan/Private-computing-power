#!/bin/bash
# scripts/package.sh — 创建分层分发包
# 用法: scripts/package.sh <version> <platform> <bin_dir>
# 示例: scripts/package.sh 1.0.0 linux-amd64 ./bin

set -euo pipefail

VERSION="${1:?version required}"
PLATFORM="${2:?platform required (e.g., linux-amd64)}"
BIN_DIR="${3:-bin}"

DIST_DIR="dist/${VERSION}/${PLATFORM}"

# 根据平台确定二进制扩展名
case "$PLATFORM" in
    windows-*) EXT=".exe" ;;
    *) EXT="" ;;
esac

echo ">>> Packaging ${PLATFORM} version ${VERSION}..."

# 创建目录结构
mkdir -p "${DIST_DIR}/core/bin"
mkdir -p "${DIST_DIR}/core/configs"
mkdir -p "${DIST_DIR}/runtime/bin"
mkdir -p "${DIST_DIR}/gpu/lib"

# ---------- core 层 ----------
cp "${BIN_DIR}/${PLATFORM}-cpstart${EXT}" "${DIST_DIR}/core/bin/cpstart${EXT}" 2>/dev/null || \
    echo "  [skip] cpstart not found for ${PLATFORM}"
cp "${BIN_DIR}/${PLATFORM}-agent${EXT}" "${DIST_DIR}/core/bin/agent${EXT}" 2>/dev/null || \
    echo "  [skip] agent not found for ${PLATFORM}"
echo "${VERSION}" > "${DIST_DIR}/core/VERSION"

# 复制配置模板（如果存在）
if [ -f "deploy/agent.yaml.tmpl" ]; then
    cp "deploy/agent.yaml.tmpl" "${DIST_DIR}/core/configs/agent.yaml"
fi

# 打包 core
(cd "${DIST_DIR}" && tar czf "core.tar.gz" core/)

# ---------- runtime 层（占位） ----------
# 包含 containerd 运行时组件（P4）
echo "  [placeholder] runtime layer"
(cd "${DIST_DIR}" && tar czf "runtime.tar.gz" runtime/ 2>/dev/null)

# ---------- GPU 层（占位） ----------
# 包含 HAMi 库文件（P5）
echo "  [placeholder] gpu layer"
(cd "${DIST_DIR}" && tar czf "gpu.tar.gz" gpu/ 2>/dev/null)

# ---------- 校验和 ----------
cd "${DIST_DIR}"
sha256sum core.tar.gz runtime.tar.gz gpu.tar.gz > checksums.txt 2>/dev/null

# 显示包大小
echo ">>> Package sizes for ${PLATFORM}:"
ls -lh core.tar.gz runtime.tar.gz gpu.tar.gz 2>/dev/null

echo ">>> Done: ${DIST_DIR}"