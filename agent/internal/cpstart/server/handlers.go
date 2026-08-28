package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	pb "computing-power/proto/v1"

	"computing-power/agent/internal/container"
	"computing-power/agent/internal/cpstart/agent"
	cpstartcfg "computing-power/agent/internal/cpstart/config"
)

// Handler REST API 处理器
type Handler struct {
	bridge *Bridge
	runner *agent.Runner
	cfg    *cpstartcfg.Config
}

// NewHandler 创建 REST 处理器
func NewHandler(bridge *Bridge, runner *agent.Runner, cfg *cpstartcfg.Config) *Handler {
	return &Handler{
		bridge: bridge,
		runner: runner,
		cfg:    cfg,
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

	// 集群节点
	mux.HandleFunc("GET /api/v1/nodes", h.listNodes)
	mux.HandleFunc("GET /api/v1/nodes/{id}", h.getNode)

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
		"configured": configured,
		"agent_status": h.runner.Status().String(),
		"node_id":    h.runner.NodeID(),
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
		"status":   "configured",
		"node_id":  h.runner.NodeID(),
		"name":     h.cfg.Agent.Name,
		"address":  h.cfg.Scheduler.Address,
	})
}

func (h *Handler) setupCheck(w http.ResponseWriter, r *http.Request) {
	checks := map[string]interface{}{
		"scheduler":  false,
		"containerd": false,
		"gpu":        false,
	}

	// 检查调度器连通性
	client, err := h.bridge.Client()
	if err == nil {
		_, err = client.ListNodes(r.Context(), &pb.ListNodesRequest{})
		checks["scheduler"] = err == nil
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
		"node_id":     h.runner.NodeID(),
		"agent_name":  h.cfg.Agent.Name,
		"agent_status": h.runner.Status().String(),
		"scheduler":   h.cfg.Scheduler.Address,
		"uptime_ms":   0, // TODO: 跟踪启动时间
	})
}

func (h *Handler) localResources(w http.ResponseWriter, r *http.Request) {
	res := h.runner.LocalResources()
	writeOK(w, res)
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
		// YAML 提交 — 需要解析为 Job
		// 简化处理：客户端发送 JSON 格式的 SubmitJobRequest
		writeError(w, http.StatusBadRequest, "YAML submission not yet supported via REST API; use JSON SubmitJobRequest")
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "parse request: "+err.Error())
		return
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