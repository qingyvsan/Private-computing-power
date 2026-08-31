/// <reference types="vite/client" />

// API 基础路径
const API_BASE = '/api/v1'

// 通用响应类型
export interface ApiResponse<T> {
  data?: T
  error?: string
}

// 请求封装
async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${url}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const body: ApiResponse<T> = await res.json()
  if (body.error) {
    throw new Error(body.error)
  }
  return body.data as T
}

// ========== 设置向导 API ==========

export interface SetupStatus {
  configured: boolean
  agent_status: string
  node_id: string
}

export interface SetupConfig {
  scheduler?: { address?: string }
  agent?: { name?: string; data_dir?: string }
  resources?: { max_cpu_cores?: number; max_memory_mb?: number; report_gpu?: boolean }
  invite_code?: string
}

export function getSetupStatus(): Promise<SetupStatus> {
  return request<SetupStatus>('/setup/status')
}

export function postSetupConfig(config: SetupConfig): Promise<any> {
  return request('/setup/config', {
    method: 'POST',
    body: JSON.stringify(config),
  })
}

export function getSetupCheck(): Promise<Record<string, boolean>> {
  return request<Record<string, boolean>>('/setup/check')
}

export interface SetupCheckResult {
  scheduler: boolean
  containerd: boolean
  gpu: boolean
  os: string
  wsl_available: boolean
  [key: string]: any
}

// ========== 本地状态 API ==========

export interface LocalStatus {
  node_id: string
  agent_name: string
  agent_status: string
  scheduler: string
}

export function getLocalStatus(): Promise<LocalStatus> {
  return request<LocalStatus>('/status')
}

// ========== 设置 API ==========

export interface SettingsConfig {
  agent_name: string
  scheduler: string
  max_cpu_cores: number
  max_memory_mb: number
  report_gpu: boolean
  node_id: string
  agent_status: string
  data_dir: string
}

export interface ResourceSettings {
  max_cpu_cores: number
  max_memory_mb: number
  report_gpu: boolean
}

export function getSettings(): Promise<SettingsConfig> {
  return request<SettingsConfig>('/settings')
}

export function updateResourceSettings(settings: ResourceSettings): Promise<any> {
  return request('/settings/resources', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
}

// ========== 节点 API ==========

export interface Node {
  id: string
  name: string
  status: string
  version: string
  overlay_ip: string
  phi_value: number
  current_tasks: number
  max_tasks: number
  reputation: number
  resources?: NodeResources
}

export interface NodeResources {
  cpu_cores: number
  cpu_usage: number
  memory_bytes: number
  memory_used: number
  disk_bytes: number
  disk_used: number
  gpus?: GPUDevice[]
}

export interface GPUDevice {
  uuid: string
  model: string
  memory_total_mb: number
  memory_used_mb: number
  memory_available_mb: number
  compute_util: number
}

export function listNodes(): Promise<Node[]> {
  return request<Node[]>('/nodes')
}

export function getNode(id: string): Promise<Node> {
  return request<Node>(`/nodes/${id}`)
}

// ========== 作业 API ==========

export interface Job {
  id: string
  name: string
  type: string
  status: string
  owner_id: string
  image: string
  created_at: number
  updated_at: number
  stages?: Stage[]
}

export interface Stage {
  id: string
  name: string
  status: string
  units?: Unit[]
}

export interface Unit {
  id: string
  job_id: string
  stage_id: string
  status: string
  assigned_node: string
  retry_count: number
  exit_code: number
  error_message: string
  started_at: number
  completed_at: number
}

export interface JobsListResponse {
  jobs: Job[]
  total_count: number
}

export function listJobs(owner?: string): Promise<JobsListResponse> {
  const params = owner ? `?owner=${owner}` : ''
  return request<JobsListResponse>(`/jobs${params}`)
}

export function getJob(id: string): Promise<Job> {
  return request<Job>(`/jobs/${id}`)
}

export function submitJob(job: any): Promise<any> {
  return request('/jobs', {
    method: 'POST',
    body: JSON.stringify({ job }),
  })
}

export function cancelJob(id: string): Promise<any> {
  return request(`/jobs/${id}/cancel`, { method: 'POST' })
}

// ========== 信任 API ==========

export interface TrustEdge {
  from_node: string
  to_node: string
  expires_at: number
}

export function getTrustGraph(): Promise<TrustEdge[]> {
  return request<TrustEdge[]>('/trust-graph')
}

export function declareTrust(targetNodeID: string): Promise<any> {
  return request('/trust/declare', {
    method: 'POST',
    body: JSON.stringify({ target_node_id: targetNodeID }),
  })
}

export function revokeTrust(targetNodeID: string): Promise<any> {
  return request('/trust/revoke', {
    method: 'POST',
    body: JSON.stringify({ target_node_id: targetNodeID }),
  })
}

// ========== 邀请码 API ==========

export function createInviteCode(): Promise<any> {
  return request('/invite-codes', { method: 'POST', body: '{}' })
}

export function redeemInviteCode(code: string, nodeID: string): Promise<any> {
  return request('/invite-codes/redeem', {
    method: 'POST',
    body: JSON.stringify({ code, node_id: nodeID }),
  })
}

// ========== WSL2 自动配置 API ==========

export interface WSL2StepState {
  name: string
  status: string
  log?: string
}

export interface WSL2Status {
  running: boolean
  steps: WSL2StepState[]
  error?: string
}

export function startWSL2Setup(): Promise<any> {
  return request('/setup/wsl2/start', { method: 'POST' })
}

export function getWSL2Status(): Promise<WSL2Status> {
  return request<WSL2Status>('/setup/wsl2/status')
}