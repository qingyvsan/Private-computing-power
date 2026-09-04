package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/container"
	"computing-power/agent/internal/cpstart/agent"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
	"computing-power/agent/internal/cpstart/macos"
	"computing-power/agent/internal/cpstart/wsl2"
	"gopkg.in/yaml.v3"
)

// Handler REST API 处理器
type Handler struct {
	bridge    *Bridge
	runner    *agent.Runner
	cfg       *cpstartcfg.Config
	wsl2      *wsl2.Automator
	macos     *macos.Automator
	startTime time.Time
}

// NewHandler 创建 REST 处理器
func NewHandler(bridge *Bridge, runner *agent.Runner, cfg *cpstartcfg.Config) *Handler {
	return &Handler{
		bridge: bridge,
		runner: runner,
		cfg:    cfg,
		wsl2: wsl2.New(wsl2.AutomatorConfig{
			DistroName:       cfg.WSL2.DistroName,
			InstallPath:      cfg.WSL2.InstallPath,
			ContainerdSocket: cfg.WSL2.ContainerdSocket,
		}),
		macos:     macos.New(),
		startTime: time.Now(),
	}
}

// RegisterRoutes 注册路由到 ServeMux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// 设置向导
	mux.HandleFunc("GET /api/v1/setup/status", h.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/config", h.setupConfig)
	mux.HandleFunc("GET /api/v1/setup/check", h.setupCheck)

	// 本地状态
	mux.HandleFunc("GET /api/v1/status", h.localStatus)
	mux.HandleFunc("GET /api/v1/status/resources", h.localResources)

	// 设置（资源限制等）
	mux.HandleFunc("GET /api/v1/settings", h.getSettings)
	mux.HandleFunc("PUT /api/v1/settings/resources", h.updateResources)
	mux.HandleFunc("PUT /api/v1/settings/features", h.updateFeatures)

	// 集群节点
	mux.HandleFunc("GET /api/v1/nodes", h.listNodes)
	mux.HandleFunc("GET /api/v1/nodes/{id}", h.getNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/unregister", h.unregisterNode)

	// 作业
	mux.HandleFunc("GET /api/v1/jobs", h.listJobs)
	mux.HandleFunc("GET /api/v1/jobs/{id}", h.getJob)
	mux.HandleFunc("POST /api/v1/jobs", h.submitJob)
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", h.cancelJob)

	// 信任
	mux.HandleFunc("GET /api/v1/trust-graph", h.getTrustGraph)
	mux.HandleFunc("POST /api/v1/trust/declare", h.declareTrust)
	mux.HandleFunc("POST /api/v1/trust/revoke", h.revokeTrust)

	// 邀请码
	mux.HandleFunc("POST /api/v1/invite-codes", h.createInvite)
	mux.HandleFunc("POST /api/v1/invite-codes/redeem", h.redeemInvite)

	// WSL2 自动配置
	mux.HandleFunc("POST /api/v1/setup/wsl2/start", h.startWSL2)
	mux.HandleFunc("GET /api/v1/setup/wsl2/status", h.wsl2Status)
	mux.HandleFunc("GET /api/v1/setup/wsl2/config", h.wsl2Config)

	// macOS 容器运行时自动配置
	mux.HandleFunc("POST /api/v1/setup/macos/start", h.startMacOS)
	mux.HandleFunc("GET /api/v1/setup/macos/status", h.macosStatus)

	mux.HandleFunc("POST /api/v1/projects/upload", h.uploadProject)
	mux.HandleFunc("GET /api/v1/projects/{id}/download", h.downloadProject)
	mux.HandleFunc("GET /api/v1/projects/{id}/status", h.projectStatus)

}

// ========== JSON 响应工具 ==========

type jsonResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, resp jsonResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, jsonResponse{Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonResponse{Error: msg})
}

// ========== 设置向导 ==========

func (h *Handler) setupStatus(w http.ResponseWriter, r *http.Request) {
	configPath := h.cfg.ConfigPath()
	_, err := cpstartcfg.Load(configPath)
	configured := err == nil

	writeOK(w, map[string]interface{}{
		"configured":   configured,
		"agent_status": h.runner.Status().String(),
		"node_id":      h.runner.NodeID(),
	})
}

func (h *Handler) setupConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var incoming cpstartcfg.Config
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeError(w, http.StatusBadRequest, "parse config: "+err.Error())
		return
	}

	// 合并到当前配置（只覆盖非零值）
	if incoming.Scheduler.Address != "" {
		h.cfg.Scheduler.Address = incoming.Scheduler.Address
	}
	if incoming.Agent.Name != "" {
		h.cfg.Agent.Name = incoming.Agent.Name
	}
	if incoming.Agent.DataDir != "" {
		h.cfg.Agent.DataDir = incoming.Agent.DataDir
	}
	if incoming.InviteCode != "" {
		h.cfg.InviteCode = incoming.InviteCode
	}
	if incoming.Resources.MaxCPUCores > 0 {
		h.cfg.Resources.MaxCPUCores = incoming.Resources.MaxCPUCores
	}
	if incoming.Resources.MaxMemoryMB > 0 {
		h.cfg.Resources.MaxMemoryMB = incoming.Resources.MaxMemoryMB
	}
	// ReportGPU 始终应用（零值 false 表示关闭 GPU 上报）
	if incoming.Resources.ReportGPU {
		h.cfg.Resources.ReportGPU = true
	} else {
		h.cfg.Resources.ReportGPU = false
	}

	// WSL2 配置
	if incoming.WSL2.InstallPath != "" {
		h.cfg.WSL2.InstallPath = incoming.WSL2.InstallPath
	}
	if incoming.WSL2.DistroName != "" {
		h.cfg.WSL2.DistroName = incoming.WSL2.DistroName
	}
	if incoming.WSL2.ContainerdSocket != "" {
		h.cfg.WSL2.ContainerdSocket = incoming.WSL2.ContainerdSocket
	}

	// 保存配置
	if err := h.cfg.Save(h.cfg.ConfigPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}

	// 重启 Agent
	h.runner.Stop()
	if err := h.runner.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "start agent: "+err.Error())
		return
	}

	writeOK(w, map[string]string{
		"status":  "configured",
		"node_id": h.runner.NodeID(),
		"name":    h.cfg.Agent.Name,
		"address": h.cfg.Scheduler.Address,
	})
}

func (h *Handler) setupCheck(w http.ResponseWriter, r *http.Request) {
	checks := map[string]interface{}{
		"scheduler":         false,
		"containerd":        false,
		"gpu":               false,
		"os":                runtime.GOOS,
		"wsl_available":     false,
		"container_backend": "none",
		"brew_available":    false,
	}

	// 检查调度器连通性
	client, err := h.bridge.Client()
	if err == nil {
		_, err = client.ListNodes(r.Context(), &pb.ListNodesRequest{})
		checks["scheduler"] = err == nil
	}

	// 检查容器运行时可用性
	if runtime.GOOS == "windows" {
		// Windows 上检查 wsl.exe 是否可用（WSL2 容器后端）
		if _, err := exec.LookPath("wsl.exe"); err == nil {
			checks["wsl_available"] = true
		}
		checks["containerd"] = false // Windows 原生不支持 containerd
		checks["container_backend"] = "wsl2"
	} else if runtime.GOOS == "darwin" {
		// macOS 上检测容器后端（Colima / Docker Desktop / OrbStack）
		backend := container.DetectBackend()
		checks["container_backend"] = backend.Type
		if backend.Type != "none" {
			checks["containerd"] = true
			checks["containerd_socket"] = backend.Socket
		}
		// 检测 Homebrew
		if _, err := exec.LookPath("brew"); err == nil {
			checks["brew_available"] = true
		}
	} else {
		// Linux 上检查 containerd socket
		if _, err := os.Stat("/run/containerd/containerd.sock"); err == nil {
			checks["containerd"] = true
			checks["container_backend"] = "containerd"
		}
	}

	// 检查 GPU 可用性
	if h.cfg.Resources.ReportGPU {
		gpus, err := container.DiscoverGPUs()
		checks["gpu"] = err == nil && len(gpus) > 0
	}

	writeOK(w, checks)
}

// ========== 本地状态 ==========

func (h *Handler) localStatus(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]interface{}{
		"node_id":      h.runner.NodeID(),
		"agent_name":   h.cfg.Agent.Name,
		"agent_status": h.runner.Status().String(),
		"scheduler":    h.cfg.Scheduler.Address,
		"uptime_ms":    time.Since(h.startTime).Milliseconds(),
	})
}

func (h *Handler) localResources(w http.ResponseWriter, r *http.Request) {
	res := h.runner.LocalResources()
	writeOK(w, res)
}

// ========== 设置（资源限制等） ==========

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]interface{}{
		"agent_name":      h.cfg.Agent.Name,
		"scheduler":       h.cfg.Scheduler.Address,
		"max_cpu_cores":   h.cfg.Resources.MaxCPUCores,
		"max_memory_mb":   h.cfg.Resources.MaxMemoryMB,
		"report_gpu":      h.cfg.Resources.ReportGPU,
		"node_id":         h.runner.NodeID(),
		"agent_status":    h.runner.Status().String(),
		"data_dir":        h.cfg.Agent.DataDir,
		"nebula_enabled":  h.cfg.Nebula.Enabled,
		"hami_enabled":    h.cfg.HAMI.Enabled,
		"updater_enabled": h.cfg.Updater.Enabled,
	})
}

func (h *Handler) updateResources(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var incoming struct {
		MaxCPUCores float64 `json:"max_cpu_cores"`
		MaxMemoryMB int64   `json:"max_memory_mb"`
		ReportGPU   bool    `json:"report_gpu"`
	}
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	// 始终应用所有值（包括 0，表示无限制）
	h.cfg.Resources.MaxCPUCores = incoming.MaxCPUCores
	h.cfg.Resources.MaxMemoryMB = incoming.MaxMemoryMB
	h.cfg.Resources.ReportGPU = incoming.ReportGPU

	// 保存配置
	if err := h.cfg.Save(h.cfg.ConfigPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}

	// 重启 Agent 使配置生效
	h.runner.Stop()
	if err := h.runner.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "start agent: "+err.Error())
		return
	}

	writeOK(w, map[string]interface{}{
		"status":        "updated",
		"max_cpu_cores": h.cfg.Resources.MaxCPUCores,
		"max_memory_mb": h.cfg.Resources.MaxMemoryMB,
		"report_gpu":    h.cfg.Resources.ReportGPU,
	})
}

func (h *Handler) updateFeatures(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var incoming struct {
		NebulaEnabled  bool `json:"nebula_enabled"`
		HAMIEnabled    bool `json:"hami_enabled"`
		UpdaterEnabled bool `json:"updater_enabled"`
	}
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	// Always apply all values
	h.cfg.Nebula.Enabled = incoming.NebulaEnabled
	h.cfg.HAMI.Enabled = incoming.HAMIEnabled
	h.cfg.Updater.Enabled = incoming.UpdaterEnabled

	// Save config
	if err := h.cfg.Save(h.cfg.ConfigPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}

	// Restart agent to apply new feature toggles
	h.runner.Stop()
	if err := h.runner.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "start agent: "+err.Error())
		return
	}

	writeOK(w, map[string]interface{}{
		"status":          "updated",
		"nebula_enabled":  h.cfg.Nebula.Enabled,
		"hami_enabled":    h.cfg.HAMI.Enabled,
		"updater_enabled": h.cfg.Updater.Enabled,
	})
}

// ========== 集群节点 ==========

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.ListNodesResponse, error) {
		return c.ListNodes(r.Context(), &pb.ListNodesRequest{})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp.Nodes)
}

func (h *Handler) getNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.GetNodeResponse, error) {
		return c.GetNode(r.Context(), &pb.GetNodeRequest{NodeID: id})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp.Node)
}

// unregisterNode 注销节点
func (h *Handler) unregisterNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	// 解析可选 reason
	var reqBody struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.UnregisterNodeResponse, error) {
		return c.UnregisterNode(r.Context(), &pb.UnregisterNodeRequest{
			NodeID: id,
			Reason: reqBody.Reason,
		})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

// ========== 作业 ==========

func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner")
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.ListJobsResponse, error) {
		return c.ListJobs(r.Context(), &pb.ListJobsRequest{NodeID: ownerID})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{
		"jobs":        resp.Jobs,
		"total_count": resp.TotalCount,
	})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.GetJobResponse, error) {
		return c.GetJob(r.Context(), &pb.GetJobRequest{JobID: id})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp.Job)
}

func (h *Handler) submitJob(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	// 尝试解析为 SubmitJobRequest
	var req pb.SubmitJobRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "x-yaml") {
		// YAML 提交 — 解析 YAML 格式的 SubmitJobRequest
		if err := yaml.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "parse YAML request: "+err.Error())
			return
		}
	}

	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	// 自动填充 OwnerID：默认以本地节点作为作业所有者。
	// 调度器据此判断"是否分配到自身节点"（allow_self_assignment=false 时不分配给自己）。
	if req.Job != nil && req.Job.OwnerID == "" {
		req.Job.OwnerID = h.runner.NodeID()
	}

	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.SubmitJobResponse, error) {
		return c.SubmitJob(r.Context(), &req)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.CancelJobResponse, error) {
		return c.CancelJob(r.Context(), &pb.CancelJobRequest{
			JobID:  id,
			NodeID: h.runner.NodeID(),
		})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

// ========== 信任 ==========

func (h *Handler) getTrustGraph(w http.ResponseWriter, r *http.Request) {
	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.GetTrustGraphResponse, error) {
		return c.GetTrustGraph(r.Context(), &pb.GetTrustGraphRequest{})
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp.Edges)
}

func (h *Handler) declareTrust(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req pb.DeclareTrustRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.DeclareTrustResponse, error) {
		return c.DeclareTrust(r.Context(), &req)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

func (h *Handler) revokeTrust(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req pb.RevokeTrustRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.RevokeTrustResponse, error) {
		return c.RevokeTrust(r.Context(), &req)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

// ========== 邀请码 ==========

func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req pb.CreateInviteCodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.CreateInviteCodeResponse, error) {
		return c.CreateInviteCode(r.Context(), &req)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

func (h *Handler) redeemInvite(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req pb.RedeemInviteCodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
	}

	resp, err := Unary(h.bridge, func(c pb.SchedulerServiceClient) (*pb.RedeemInviteCodeResponse, error) {
		return c.RedeemInviteCode(r.Context(), &req)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, resp)
}

// ========== WSL2 自动配置 ==========

func (h *Handler) startWSL2(w http.ResponseWriter, r *http.Request) {
	// 解析可选 body 中的 install_path / distro_name 覆盖（UI 传入的安装目录）
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		var overrides struct {
			InstallPath string `json:"install_path"`
			DistroName  string `json:"distro_name"`
		}
		if json.Unmarshal(body, &overrides) == nil {
			if overrides.InstallPath != "" || overrides.DistroName != "" {
				h.wsl2.SetConfig(wsl2.AutomatorConfig{
					InstallPath: overrides.InstallPath,
					DistroName:  overrides.DistroName,
				})
			}
		}
	}
	if err := h.wsl2.Start(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeOK(w, map[string]string{"status": "started"})
}

func (h *Handler) wsl2Status(w http.ResponseWriter, r *http.Request) {
	writeOK(w, h.wsl2.Status())
}

func (h *Handler) wsl2Config(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]interface{}{
		"install_path":      h.cfg.WSL2.InstallPath,
		"distro_name":       h.cfg.WSL2.DistroName,
		"containerd_socket": h.cfg.WSL2.ContainerdSocket,
	})
}

// WSL2Automator 返回 Handler 持有的 WSL2 自动配置器实例
func (h *Handler) WSL2Automator() *wsl2.Automator {
	return h.wsl2
}

// ========== macOS 容器运行时自动配置 ==========

func (h *Handler) startMacOS(w http.ResponseWriter, r *http.Request) {
	if err := h.macos.Start(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeOK(w, map[string]string{"status": "started"})
}

func (h *Handler) macosStatus(w http.ResponseWriter, r *http.Request) {
	writeOK(w, h.macos.Status())
}

// ========== 项目文件 ==========

type projectMeta struct {
	ProjectID      string `json:"project_id,omitempty"`
	StartupCommand string `json:"startup_command,omitempty"`
	BaseImage      string `json:"base_image,omitempty"`
	FileName       string `json:"file_name,omitempty"`
	Size           int64  `json:"size,omitempty"`
	CreatedAt      int64  `json:"created_at,omitempty"`
}

func (h *Handler) uploadProject(w http.ResponseWriter, r *http.Request) {
	// 限制上传大小 500MB
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "parse form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required: "+err.Error())
		return
	}
	defer file.Close()

	startupCommand := r.FormValue("startup_command")
	baseImage := r.FormValue("base_image")
	if startupCommand == "" {
		writeError(w, http.StatusBadRequest, "startup_command is required")
		return
	}
	if baseImage == "" {
		baseImage = "alpine:latest"
	}

	projectID := fmt.Sprintf("proj-%d", time.Now().UnixNano())
	projectDir := filepath.Join(h.cfg.Agent.DataDir, "projects", projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "create project dir: "+err.Error())
		return
	}

	// 保存 zip 文件
	zipPath := filepath.Join(projectDir, "project.zip")
	dst, err := os.Create(zipPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create file: "+err.Error())
		return
	}
	defer dst.Close()

	size, err := io.Copy(dst, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save file: "+err.Error())
		return
	}

	// 保存元数据
	meta := projectMeta{
		ProjectID:      projectID,
		StartupCommand: startupCommand,
		BaseImage:      baseImage,
		FileName:       header.Filename,
		Size:           size,
		CreatedAt:      time.Now().UnixMilli(),
	}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(projectDir, "meta.json"), metaData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "save meta: "+err.Error())
		return
	}

	log.Printf("cpstart: project %s uploaded (%s, %d bytes)", projectID, header.Filename, size)
	writeOK(w, meta)
}

func (h *Handler) downloadProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	zipPath := filepath.Join(h.cfg.Agent.DataDir, "projects", projectID, "project.zip")
	if _, err := os.Stat(zipPath); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, projectID))
	http.ServeFile(w, r, zipPath)
}

func (h *Handler) projectStatus(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	projectDir := filepath.Join(h.cfg.Agent.DataDir, "projects", projectID)
	metaPath := filepath.Join(projectDir, "meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var meta projectMeta
	json.Unmarshal(metaData, &meta)
	writeOK(w, meta)
}
