  #!/usr/bin/env python3
  import http.server, json, os, socket, sys, time
  from datetime import datetime

  HOST = "0.0.0.0"
  PORT = int(os.environ.get("TEST_PORT", "8080"))
  NODE_NAME = os.environ.get("NODE_NAME", socket.gethostname())

  class Handler(http.server.BaseHTTPRequestHandler):
      def _json(self, data, status=200):
          body = json.dumps(data, indent=2).encode()
          self.send_response(status)
          self.send_header("Content-Type", "application/json")
          self.end_headers()
          self.wfile.write(body)
      def do_GET(self):
          if self.path in ("/", "/health"):
              self._json({"status": "ok", "node": NODE_NAME, "time": datetime.now().isoformat()})
          elif self.path == "/info":
              self._json({"node": NODE_NAME, "hostname": socket.gethostname(), "platform": sys.platform, "python": sys.version})
          else:
              self._json({"error": "not found"}, 404)

  def main():
      server = http.server.HTTPServer((HOST, PORT), Handler)
      print(f"[{NODE_NAME}] Server on http://{HOST}:{PORT}")
      sys.stdout.flush()
      server.serve_forever()
  if __name__ == "__main__":
      main()
