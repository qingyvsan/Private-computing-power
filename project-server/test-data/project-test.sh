#!/bin/bash
# Computing Power 项目文件测试脚本
# 从 Windows Python 服务下载项目并测试容器执行

PROJECT_SERVER="http://192.168.1.248:8081"
PROJECT_ID="$1"

if [ -z "$PROJECT_ID" ]; then
    echo "用法: $0 <project_id>"
    echo "可用项目:"
    curl -s "$PROJECT_SERVER/api/v1/projects" | python3 -m json.tool 2>/dev/null || echo "无法连接项目服务器"
    exit 1
fi

echo "=== 1. 获取项目元数据 ==="
curl -s "$PROJECT_SERVER/api/v1/projects/$PROJECT_ID" | python3 -m json.tool

echo ""
echo "=== 2. 下载项目文件 ==="
PROJECT_DIR="$HOME/cp-project-$PROJECT_ID"
rm -rf "$PROJECT_DIR"
mkdir -p "$PROJECT_DIR"
curl -s -o "$PROJECT_DIR/project.zip" "$PROJECT_SERVER/api/v1/projects/$PROJECT_ID/download"
unzip -o -q "$PROJECT_DIR/project.zip" -d "$PROJECT_DIR/workspace"
echo "项目文件:"
ls -la "$PROJECT_DIR/workspace/"

echo ""
echo "=== 3. 通过 Colima 容器执行 ==="
META=$(curl -s "$PROJECT_SERVER/api/v1/projects/$PROJECT_ID")
STARTUP_CMD=$(echo "$META" | python3 -c "import sys,json; print(json.load(sys.stdin).get('startup_command',''))" 2>/dev/null)
BASE_IMAGE=$(echo "$META" | python3 -c "import sys,json; print(json.load(sys.stdin).get('base_image','alpine:latest'))" 2>/dev/null)

echo "启动命令: $STARTUP_CMD"
echo "基础镜像: $BASE_IMAGE"
echo ""

# 通过 colima VM 内 nerdctl 拉取镜像
echo "正在拉取镜像 $BASE_IMAGE ..."
colima ssh -- sudo nerdctl pull "$BASE_IMAGE" 2>&1 || { echo "镜像拉取失败"; exit 1; }

echo ""
echo "--- 容器输出 ---"
colima ssh -- sudo nerdctl run --rm \
    -v "$HOME/cp-project-$PROJECT_ID/workspace:/workspace:ro" \
    "$BASE_IMAGE" \
    sh -c "$STARTUP_CMD"

echo ""
echo "=== 完成 ==="
