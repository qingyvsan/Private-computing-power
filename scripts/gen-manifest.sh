#!/bin/bash
# scripts/gen-manifest.sh — 生成更新清单 manifest.json
# 用法: scripts/gen-manifest.sh <version>
# 输出: dist/<version>/manifest.json

set -euo pipefail

VERSION="${1:?version required}"
DIST_DIR="dist/${VERSION}"

if [ ! -d "${DIST_DIR}" ]; then
    echo "Error: ${DIST_DIR} not found. Run scripts/package.sh first."
    exit 1
fi

echo ">>> Generating manifest for version ${VERSION}..."

# 构建 JSON
cat > "${DIST_DIR}/manifest.json" << EOF
{
  "version": "${VERSION}",
  "release_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "platforms": {
EOF

first_platform=true
for platform_dir in "${DIST_DIR}"/*/; do
    platform=$(basename "${platform_dir}")
    [ "$platform" = "manifest.json" ] && continue

    if [ -f "${platform_dir}/checksums.txt" ]; then
        # 读取校验和
        core_sha=$(awk '/core.tar.gz/ {print $1}' "${platform_dir}/checksums.txt" 2>/dev/null || echo "")
        core_size=$(stat -c%s "${platform_dir}/core.tar.gz" 2>/dev/null || stat -f%z "${platform_dir}/core.tar.gz" 2>/dev/null || echo "0")
        runtime_sha=$(awk '/runtime.tar.gz/ {print $1}' "${platform_dir}/checksums.txt" 2>/dev/null || echo "")
        runtime_size=$(stat -c%s "${platform_dir}/runtime.tar.gz" 2>/dev/null || stat -f%z "${platform_dir}/runtime.tar.gz" 2>/dev/null || echo "0")
        gpu_sha=$(awk '/gpu.tar.gz/ {print $1}' "${platform_dir}/checksums.txt" 2>/dev/null || echo "")
        gpu_size=$(stat -c%s "${platform_dir}/gpu.tar.gz" 2>/dev/null || stat -f%z "${platform_dir}/gpu.tar.gz" 2>/dev/null || echo "0")

        [ -z "$core_sha" ] && core_sha="0000000000000000000000000000000000000000000000000000000000000000"

        if [ "$first_platform" = true ]; then
            first_platform=false
        else
            echo "," >> "${DIST_DIR}/manifest.json"
        fi

        cat >> "${DIST_DIR}/manifest.json" << EOF
    "${platform}": {
      "core": {
        "url": "https://update.computing-power.local/v1/releases/${VERSION}/${platform}/core.tar.gz",
        "sha256": "${core_sha}",
        "size": ${core_size:-0}
      }$(if [ -n "${runtime_sha}" ]; then echo ",
      \"runtime\": {
        \"url\": \"https://update.computing-power.local/v1/releases/${VERSION}/${platform}/runtime.tar.gz\",
        \"sha256\": \"${runtime_sha}\",
        \"size\": ${runtime_size:-0}
      }"; fi)$(if [ -n "${gpu_sha}" ]; then echo ",
      \"gpu\": {
        \"url\": \"https://update.computing-power.local/v1/releases/${VERSION}/${platform}/gpu.tar.gz\",
        \"sha256\": \"${gpu_sha}\",
        \"size\": ${gpu_size:-0}
      }"; fi)
    }
EOF
    fi
done

echo "" >> "${DIST_DIR}/manifest.json"
echo "  }" >> "${DIST_DIR}/manifest.json"
echo "}" >> "${DIST_DIR}/manifest.json"

echo ">>> Manifest written to ${DIST_DIR}/manifest.json"
python3 -m json.tool "${DIST_DIR}/manifest.json" > /dev/null 2>&1 && echo ">>> Valid JSON" || echo ">>> WARNING: Invalid JSON"