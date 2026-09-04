#!/usr/bin/env python3
"""
Computing Power 项目文件在线服务

用于 Windows ↔ macOS 节点通信测试：
- 本地 Windows 客户端上传项目文件（zip 包）
- macOS Agent 通过 HTTP 下载项目并在容器内执行

用法：
    python server.py [--port 8080] [--dir ./projects]
"""
import argparse
import json
import os
import socket
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse

VERSION = "1.0.0"
STORAGE_DIR = None  # 项目文件存储目录


def get_all_ips():
    """获取本机所有 IPv4 地址"""
    ips = set()
    try:
        # 通过 UDP 连接获取本机外网地址（不实际发送数据）
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            s.connect(("8.8.8.8", 80))
            ips.add(s.getsockname()[0])
        finally:
            s.close()
    except Exception:
        pass
    try:
        hostname = socket.gethostname()
        for info in socket.getaddrinfo(hostname, None, socket.AF_INET):
            ips.add(info[4][0])
    except Exception:
        pass
    return sorted(ips)


def list_projects():
    """列出所有已上传的项目"""
    if not os.path.isdir(STORAGE_DIR):
        return []
    projects = []
    for name in os.listdir(STORAGE_DIR):
        meta_path = os.path.join(STORAGE_DIR, name, "meta.json")
        zip_path = os.path.join(STORAGE_DIR, name, "project.zip")
        if os.path.isfile(meta_path) and os.path.isfile(zip_path):
            try:
                with open(meta_path, "r", encoding="utf-8") as f:
                    meta = json.load(f)
                meta["project_id"] = name
                meta["download_url"] = f"/api/v1/projects/{name}/download"
                projects.append(meta)
            except Exception:
                continue
    projects.sort(key=lambda p: p.get("created_at", 0), reverse=True)
    return projects


class ProjectHandler(BaseHTTPRequestHandler):
    server_version = f"CPProjectServer/{VERSION}"

    # ---------- 响应辅助 ----------
    def _json(self, status, obj):
        body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _text(self, status, text, ctype="text/plain; charset=utf-8"):
        body = text.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_file(self, path, download_name=None, ctype="application/octet-stream"):
        size = os.path.getsize(path)
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        if download_name:
            self.send_header(
                "Content-Disposition",
                f'attachment; filename="{download_name}"',
            )
        self.send_header("Content-Length", str(size))
        self.end_headers()
        with open(path, "rb") as f:
            while True:
                chunk = f.read(64 * 1024)
                if not chunk:
                    break
                self.wfile.write(chunk)

    # ---------- 路由 ----------
    def do_GET(self):
        path = urlparse(self.path).path
        if path in ("/", "/index.html"):
            self._index()
        elif path == "/health":
            self._json(200, {"status": "ok", "version": VERSION})
        elif path == "/api/v1/projects":
            self._json(200, {"projects": list_projects()})
        elif path.startswith("/api/v1/projects/"):
            parts = path[len("/api/v1/projects/"):].split("/")
            project_id = parts[0] if parts else ""
            if len(parts) == 1:
                self._project_meta(project_id)
            elif len(parts) == 2 and parts[1] == "download":
                self._download(project_id)
            elif len(parts) == 2 and parts[1] == "status":
                self._project_meta(project_id)
            else:
                self._json(404, {"error": "not found"})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        path = urlparse(self.path).path
        if path == "/api/v1/projects/upload":
            self._upload()
        else:
            self._json(404, {"error": "not found"})

    # ---------- 页面 ----------
    def _index(self):
        html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>Computing Power 项目文件服务</title>
<style>
  body {{ font-family: -apple-system, Segoe UI, sans-serif; margin: 40px auto; max-width: 800px; color: #333; }}
  h1 {{ font-size: 22px; }}
  .card {{ border: 1px solid #ddd; border-radius: 8px; padding: 20px; margin: 16px 0; }}
  .btn {{ background: #409eff; color: #fff; border: none; padding: 10px 20px; border-radius: 6px; cursor: pointer; font-size: 14px; }}
  .btn:disabled {{ background: #a0cfff; cursor: not-allowed; }}
  table {{ width: 100%; border-collapse: collapse; margin-top: 16px; }}
  th, td {{ border: 1px solid #eee; padding: 8px 12px; text-align: left; font-size: 14px; }}
  code {{ background: #f4f4f5; padding: 2px 6px; border-radius: 4px; }}
</style>
</head>
<body>
<h1>☁️ Computing Power 项目文件服务</h1>
<p>服务版本 {VERSION} · 本机 IP: <code>{', '.join(get_all_ips())}</code> · 端口 <code>{self.server.server_address[1]}</code></p>

<div class="card">
  <h3>上传项目</h3>
  <p>上传 zip 包（包含容器内执行的代码/脚本），并指定启动命令和基础镜像。</p>
  <input type="file" id="file" accept=".zip">
  <div style="margin: 12px 0;">
    <label>启动命令：</label><br>
    <input id="cmd" placeholder="sh /workspace/main.sh" style="width: 100%; padding: 8px; margin-top: 6px; box-sizing: border-box;">
  </div>
  <div style="margin: 12px 0;">
    <label>基础镜像：</label><br>
    <input id="img" value="alpine:latest" style="width: 100%; padding: 8px; margin-top: 6px; box-sizing: border-box;">
  </div>
  <button class="btn" id="uploadBtn" onclick="doUpload()">上传</button>
  <span id="msg" style="margin-left: 12px; color: #67c23a;"></span>
</div>

<div class="card">
  <h3>已上传项目</h3>
  <table id="tbl">
    <thead><tr><th>项目ID</th><th>文件名</th><th>大小</th><th>启动命令</th><th>操作</th></tr></thead>
    <tbody></tbody>
  </table>
</div>

<script>
async function refresh() {{
  const r = await fetch('/api/v1/projects');
  const d = await r.json();
  const tb = document.querySelector('#tbl tbody');
  tb.innerHTML = '';
  for (const p of d.projects) {{
    const tr = document.createElement('tr');
    const size = p.size >= 1048576 ? (p.size/1048576).toFixed(1)+'MB' : Math.ceil(p.size/1024)+'KB';
    tr.innerHTML = `<td><code>${{p.project_id}}</code></td><td>${{p.file_name||'-'}}</td><td>${{size}}</td><td>${{p.startup_command||'-'}}</td>
      <td><a href="${{p.download_url}}">下载</a></td>`;
    tb.appendChild(tr);
  }}
}}
async function doUpload() {{
  const f = document.getElementById('file').files[0];
  const cmd = document.getElementById('cmd').value;
  const img = document.getElementById('img').value;
  const msg = document.getElementById('msg');
  if (!f) {{ msg.textContent='请选择 zip 文件'; msg.style.color='#f56c6c'; return; }}
  if (!cmd) {{ msg.textContent='请填写启动命令'; msg.style.color='#f56c6c'; return; }}
  const fd = new FormData();
  fd.append('file', f);
  fd.append('startup_command', cmd);
  fd.append('base_image', img);
  const btn = document.getElementById('uploadBtn');
  btn.disabled = true; msg.textContent = '上传中...'; msg.style.color='#e6a23c';
  try {{
    const r = await fetch('/api/v1/projects/upload', {{ method:'POST', body: fd }});
    const d = await r.json();
    if (r.ok) {{ msg.textContent = '上传成功: '+d.project_id; msg.style.color='#67c23a'; refresh(); }}
    else {{ msg.textContent = '上传失败: '+(d.error||r.status); msg.style.color='#f56c6c'; }}
  }} catch (e) {{ msg.textContent = '上传失败: '+e; msg.style.color='#f56c6c'; }}
  btn.disabled = false;
}}
refresh();
</script>
</body>
</html>"""
        body = html.encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    # ---------- API 实现 ----------
    def _upload(self):
        # 解析 multipart/form-data
        content_type = self.headers.get("Content-Type", "")
        if "multipart/form-data" not in content_type:
            return self._json(400, {"error": "expect multipart/form-data"})

        boundary = content_type.split("boundary=")[1].strip().strip('"').encode()
        length = int(self.headers.get("Content-Length", 0))
        if length > 500 * 1024 * 1024:
            return self._json(413, {"error": "file too large (max 500MB)"})

        body = self.rfile.read(length)

        parts = body.split(b"--" + boundary)
        fields = {}
        file_data = None
        file_name = None

        for part in parts:
            part = part.strip(b"\r\n")
            if not part or part == b"--":
                continue
            # 头部与内容分隔
            sep_idx = part.find(b"\r\n\r\n")
            if sep_idx == -1:
                continue
            headers_raw = part[:sep_idx].decode("latin-1")
            content = part[sep_idx + 4:]
            # 去除尾部 \r\n
            if content.endswith(b"\r\n"):
                content = content[:-2]

            ctype = None
            is_file = False
            for line in headers_raw.split("\r\n"):
                if line.lower().startswith("content-type:"):
                    ctype = line.split(":", 1)[1].strip()
            if ctype:
                is_file = True
            elif "filename=" in headers_raw:
                is_file = True

            if is_file:
                # 文件字段
                for line in headers_raw.split("\r\n"):
                    if line.lower().startswith("content-disposition:"):
                        for seg in line.split(";"):
                            seg = seg.strip()
                            if seg.startswith("filename="):
                                file_name = seg.split("=", 1)[1].strip().strip('"')
                file_data = content
            else:
                for line in headers_raw.split("\r\n"):
                    if line.lower().startswith("content-disposition:"):
                        for seg in line.split(";"):
                            seg = seg.strip()
                            if seg.startswith("name="):
                                fname = seg.split("=", 1)[1].strip().strip('"')
                                fields[fname] = content.decode("utf-8", errors="replace")
                                break

        if file_data is None:
            return self._json(400, {"error": "file required (field name: file)"})

        startup_command = fields.get("startup_command", "")
        base_image = fields.get("base_image", "alpine:latest")
        if not startup_command:
            return self._json(400, {"error": "startup_command is required"})

        project_id = "proj-%d" % (int(time.time() * 1000))
        project_dir = os.path.join(STORAGE_DIR, project_id)
        os.makedirs(project_dir, exist_ok=True)

        zip_path = os.path.join(project_dir, "project.zip")
        with open(zip_path, "wb") as f:
            f.write(file_data)

        meta = {
            "project_id": project_id,
            "startup_command": startup_command,
            "base_image": base_image,
            "file_name": file_name,
            "size": len(file_data),
            "created_at": int(time.time() * 1000),
        }
        with open(os.path.join(project_dir, "meta.json"), "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=2)

        print(f"[upload] {project_id} <- {file_name} ({len(file_data)} bytes)")
        self._json(200, meta)

    def _project_meta(self, project_id):
        meta_path = os.path.join(STORAGE_DIR, project_id, "meta.json")
        if not os.path.isfile(meta_path):
            return self._json(404, {"error": "project not found"})
        with open(meta_path, "r", encoding="utf-8") as f:
            meta = json.load(f)
        meta["project_id"] = project_id
        meta["download_url"] = f"/api/v1/projects/{project_id}/download"
        self._json(200, meta)

    def _download(self, project_id):
        # 防止路径穿越
        if project_id in ("", ".", "..") or "/" in project_id or "\\" in project_id:
            return self._json(400, {"error": "invalid project_id"})
        zip_path = os.path.join(STORAGE_DIR, project_id, "project.zip")
        if not os.path.isfile(zip_path):
            return self._json(404, {"error": "project not found"})
        self._send_file(zip_path, f"{project_id}.zip", "application/zip")

    def log_message(self, fmt, *args):
        sys.stderr.write("[%s] %s\n" % (time.strftime("%H:%M:%S"), fmt % args))


def main():
    parser = argparse.ArgumentParser(description="Computing Power 项目文件在线服务")
    parser.add_argument("--port", type=int, default=8080, help="监听端口 (default: 8080)")
    parser.add_argument("--dir", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "projects"), help="项目存储目录")
    parser.add_argument("--host", default="0.0.0.0", help="监听地址 (default: 0.0.0.0)")
    args = parser.parse_args()

    global STORAGE_DIR
    STORAGE_DIR = args.dir
    os.makedirs(STORAGE_DIR, exist_ok=True)

    httpd = ThreadingHTTPServer((args.host, args.port), ProjectHandler)

    print("=" * 60)
    print(" Computing Power 项目文件在线服务")
    print("=" * 60)
    print(f"  监听地址 : {args.host}:{args.port}")
    print(f"  存储目录 : {os.path.abspath(STORAGE_DIR)}")
    print(f"  本机 IP  : {', '.join(get_all_ips())}")
    print()
    print("  上传项目 : curl -F 'file=@proj.zip' -F 'startup_command=sh /workspace/main.sh' \\")
    print(f"              -F 'base_image=alpine:latest' http://<本机IP>:{args.port}/api/v1/projects/upload")
    print(f"  下载项目 : http://<本机IP>:{args.port}/api/v1/projects/{{project_id}}/download")
    print(f"  项目列表 : http://<本机IP>:{args.port}/api/v1/projects")
    print(f"  网页界面 : http://<本机IP>:{args.port}/")
    print("=" * 60)
    print()

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[server] 已停止")
        httpd.server_close()


if __name__ == "__main__":
    main()
