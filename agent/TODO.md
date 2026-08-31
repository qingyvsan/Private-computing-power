# 项目未实现功能与桩代码报告

> 生成日期：2026-08-31
> 范围：全项目（agent / scheduler / cli / proto / pkg）

---

## 一、安全

### 1.1 mTLS 通信加密（P6）

**状态：** 暂未实现，当前使用不安全的明文连接

| 位置 | 文件 | 行号 |
|------|------|------|
| Agent → Scheduler | `agent/internal/core/agent.go` | 103 |
| CLI → Scheduler | `cli/internal/client/client.go` | 31 |

```go
grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO(P6): 使用 mTLS
```

两个位置使用相同的 `insecure.NewCredentials()`。计划接入 mTLS，需要：
- 证书签发与管理（CA 基础设施）
- 客户端与服务端双向证书验证
- 证书热更新机制

---

## 二、容器运行时

### 2.1 Windows 容器环境支持

**状态：** ✅ 已完成 WSL2 方案

通过 WSL2 自动配置引擎，在 Windows 上自动安装/配置 WSL2 Ubuntu 发行版 + containerd + NVIDIA 容器支持。

**实现方式：** `agent/internal/cpstart/wsl2/automator.go` — 异步 6 步自动化流程：
1. 检测 WSL2 状态
2. 检查/安装 Ubuntu 发行版（`wsl --install` → 失败回退 `wsl --import`）
3. 安装 containerd
4. 配置 containerd（SystemdCgroup、启动脚本、wsl.conf）
5. 安装 NVIDIA 容器工具包（有 GPU 时）
6. 验证环境（`ctr version` + hello-world 测试）

**前端集成：** `StepWindows.vue` — 一键配置按钮 + 实时进度轮询（2s 间隔）

**注意：** 容器运行时栈其余部分仍为 Linux-only。WSL2 方案通过透明代理转发 containerd 调用到 WSL2 内部，上层代码无需改动。

---

## 三、Agent 核心

### 3.1 容器输出收集（P4）

**状态：** 桩代码，始终返回 nil

**文件：** `agent/internal/executor/executor.go`，第 233-236 行

```go
// collectOutput 收集容器输出（占位，P4 后续可扩展）
func collectOutput(containerID string) []byte {
    // TODO(P4): 通过 containerd task.IO 或日志 API 收集 stdout/stderr
    return nil
}
```

**影响：** `monitorContainer()` 在第 142 行调用此函数，但结果始终为空。作业完成后用户无法查看容器标准输出/错误。

### 3.2 CPU 资源限制未生效（P4）

**状态：** 配置已保存，但实际未限制

**文件：** `agent/internal/cpstart/agent/runner.go`，第 167-171 行

```go
func (r *Runner) applyResourceLimits(ac *config.Config) {
    if r.cfg.Resources.MaxCPUCores > 0 {
        // TODO(P4): 在 agent 启动时限制 CPU 使用
        _ = ac
    }
}
```

**影响：** 用户在 Web 设置页面保存的 `max_cpu_cores` 值写入配置文件但未生效。`max_memory_mb` 同样未传递到 agent 配置（`ToAgentConfig()` 未包含 `MaxMemoryMB`）。

### 3.3 心跳间隔动态调整（P3）

**状态：** 代码已写但被注释掉

**文件：** `agent/internal/heartbeat/reporter.go`，第 140-145 行

```go
// TODO(P3): 根据服务器建议调整心跳间隔
// if resp.HeartbeatInterval != "" {
//     if newInterval, err := time.ParseDuration(resp.HeartbeatInterval); err == nil {
//         r.interval = newInterval
//     }
// }
```

**影响：** 服务器可以在心跳响应中建议新的间隔，但客户端忽略此字段。Proto 中 `HeartbeatInterval` 字段已定义，功能逻辑已编写，仅需取消注释并测试。

### 3.4 启动时间追踪

**状态：** 硬编码为 0

**文件：** `agent/internal/cpstart/server/handlers.go`，第 194 行

```go
"uptime_ms": 0, // TODO: 跟踪启动时间
```

**影响：** `GET /api/v1/status` 返回的 `uptime_ms` 始终为 0。需要记录 Agent 启动时间戳并计算差值。

---

## 四、调度器

### 4.1 Nebula 覆盖网络默认关闭

**状态：** 功能可选，默认不启用

**文件：** `agent/internal/nebula/manager.go`，第 57、103 行

```go
return fmt.Errorf("nebula is not enabled")
```

**影响：** NAT 穿透和节点间隧道（P6 功能）默认不启用。需要用户在配置中显式开启 `nebula.enabled`。

### 4.2 HAMi GPU 虚拟化默认关闭

**状态：** 功能可选，默认不启用

**文件：** `agent/internal/container/hami.go`，第 81 行

```go
return nil, fmt.Errorf("hami: not enabled")
```

**影响：** GPU 虚拟化与分片功能默认关闭。需要在配置中启用 `hami.enabled`。

### 4.3 自动更新器默认关闭

**状态：** 功能可选，默认不启用

**文件：** `pkg/updater/updater.go`，第 67 行

```go
log.Printf("updater: disabled")
return
```

**影响：** P10 打包分发与自动更新功能默认关闭。需要在配置中启用 `updater.enabled`。

---

## 五、API 与前端

### 5.1 YAML 作业提交不支持

**状态：** 明确拒绝

**文件：** `agent/internal/cpstart/server/handlers.go`，第 269 行

```go
writeError(w, http.StatusBadRequest, "YAML submission not yet supported via REST API; use JSON SubmitJobRequest")
```

**影响：** 客户端仅能通过 JSON 格式提交作业。YAML 格式的 `Content-Type` 请求被拒绝（尽管 YAML 解析逻辑的入口已存在）。

### 5.2 WebSocket/SSE 实时更新

**状态：** 未实现（非显式 TODO，但前端架构中缺失）

**影响：** 当前前端通过请求-响应模式获取数据，无实时推送。用户需要手动刷新页面才能看到作业状态变化、节点上下线等。可在后续版本中引入 WebSocket 或 Server-Sent Events。

---

## 六、优先级汇总

| 优先级 | 项目 | 影响范围 | 预估工作量 |
|--------|------|----------|-----------|
| **P3** | 心跳间隔动态调整 | 调度器负载优化 | 1 小时 |
| **P4** | 容器输出收集 | 用户无法查看作业日志 | 1-2 天 |
| **P4** | CPU/内存资源限制 | 资源配置不生效 | 2-3 天 |
| **P6** | mTLS 通信加密 | 生产环境安全 | 3-5 天 |
| **P6** | Nebula 覆盖网络启用 | 节点间隧道通信 | 配置即可 |
| **—** | 启动时间追踪 | 状态显示 | 30 分钟 |
| **—** | YAML 作业提交 | API 完整度 | 1 天 |
| **—** | 实时更新（WebSocket） | 用户体验 | 3-5 天 |

---

## 七、变更文件清单

### 已修复（本轮）

| 文件 | 变更 |
|------|------|
| `agent/internal/cpstart/wsl2/automator.go` | **新增** WSL2 自动配置引擎（6 步异步流程，支持 `wsl --import` 免管理员安装） |
| `agent/ui/src/components/setup/StepWindows.vue` | **新增** 一键配置 WSL2 环境（实时进度轮询） |
| `scripts/setup-wsl2.ps1` | **新增** WSL2 环境手动安装脚本 |
| `scripts/start-wsl2-cpstart.sh` | **新增** WSL2 内部 cpstart 启动脚本 |
| `agent/internal/cpstart/config/config.go` | 添加 `json` tag，修复 GPU/资源设置 JSON 解析丢失 |
| `agent/internal/cpstart/server/handlers.go` | 新增 WSL2 API 路由 + `data_dir` 返回；修复 ReportGPU 合并 bug |
| `agent/internal/cpstart/agent/runner.go` | 修复 `close of closed channel` panic（Stop/Start 竞态） |
| `agent/ui/src/components/setup/StepIdentity.vue` | 新增数据目录输入框 |
| `agent/ui/src/components/setup/SetupWizard.vue` | 集成 WSL2 步骤 + 数据目录传递 |
| `agent/ui/src/api/client.ts` | 新增 WSL2 API 类型与函数；更新 `SetupConfig`/`SettingsConfig` 接口 |
| `agent/ui/src/views/SettingsPage.vue` | 数据目录卡片显示实际路径 |

### 待修复

| 文件 | 待办项 |
|------|--------|
| `agent/internal/executor/executor.go:235` | 实现 `collectOutput()` |
| `agent/internal/executor/executor.go:190` | 新增 `startTime` 追踪替代硬编码 0 |
| `agent/internal/cpstart/agent/runner.go:169` | 实现 `applyResourceLimits()` |
| `agent/internal/cpstart/config/config.go` | `ToAgentConfig()` 传递 `MaxMemoryMB` |
| `agent/internal/heartbeat/reporter.go:140` | 取消注释心跳间隔调整代码 |
| `agent/internal/core/agent.go:103` | 接入 mTLS |
| `cli/internal/client/client.go:31` | 接入 mTLS |
| `agent/internal/container/containerd.go` | 新增 Windows 运行时实现 |
| `agent/internal/container/gpu.go` | Windows 路径支持 |