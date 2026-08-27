package server

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"computing-power/agent/internal/cpstart/agent"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
)

//go:embed all:ui/dist
var uiFS embed.FS

// HTTPServer 本地控制台 HTTP 服务器
type HTTPServer struct {
	server *http.Server
	handler *Handler
}

// NewHTTPServer 创建 HTTP 服务器
func NewHTTPServer(cfg *cpstartcfg.Config, bridge *Bridge, runner *agent.Runner) (*HTTPServer, error) {
	handler := NewHandler(bridge, runner, cfg)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 静态文件服务
	staticHandler, err := newStaticHandler()
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /", staticHandler)

	// 应用中间件
	var h http.Handler = mux
	h = loggingMiddleware(h)
	h = recoveryMiddleware(h)
	h = localhostMiddleware(h)

	addr := "127.0.0.1"
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &HTTPServer{
		server:  httpServer,
		handler: handler,
	}, nil
}

// Start 启动 HTTP 服务器（非阻塞）
func (s *HTTPServer) Start() error {
	log.Printf("cpstart: console listening on %s", s.server.Addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("cpstart: http server error: %v", err)
		}
	}()
	return nil
}

// Stop 停止 HTTP 服务器
func (s *HTTPServer) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// newStaticHandler 创建内嵌静态文件处理器
func newStaticHandler() (http.Handler, error) {
	subFS, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(subFS)), nil
}

// ListenAddr 返回监听地址
func (s *HTTPServer) ListenAddr() string {
	return s.server.Addr
}