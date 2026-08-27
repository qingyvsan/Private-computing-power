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
  resources?: { max_cpu_cores?: number; max_memory_mb?: number }
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
  id: string
  name: string
  memory_mb: number
  memory_used_mb: number
  cores: number
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