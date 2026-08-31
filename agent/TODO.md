# 项目未实现功能与桩代码报告

> 生成日期：2026-08-31
> 范围：全项目（agent / scheduler / cli / proto / pkg）

---

## 一、安全

### 1.1 mTLS 通信加密（P6）✅ 已完成

**状态：** 已实现，使用内部 gRPC CA 签发证书

**实现方式：**
- 调度器启动时从 `cfg.TLS.CACert/CAKey` 加载或创建 gRPC CA，生成服务器证书
- 服务器 TLS 使用 `tls.VerifyClientCertIfGiven`，允许首次注册连接
- 拦截器（一元 + 流式）对所有 RPC 强制客户端证书，`RegisterNode` 特殊放行
- Agent 首次连接无客户端证书 → 注册获取证书 → 保存到 `{data_dir}/grpc/` → 重新连接使用 mTLS
- 既存节点重启直接加载磁盘证书，完整 mTLS 连接
- CLI 通过 `--tls-cert`/`--tls-key`/`--ca-cert` 标志使用 mTLS
- cpstart 通过配置 `tls.ca_cert/cert/key` 启用 mTLS

**新增文件：**
| 文件 | 说明 |
|------|------|
| `scheduler/internal/ca/grpc.go` | gRPC mTLS CA 管理器 |
| `scheduler/internal/server/mtls.go` | mTLS 拦截器 + 节点身份提取 |

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

### 3.1 容器输出收集（P4）✅ 已完成

**状态：** 已实现，通过 containerd task.IO 缓冲捕获 stdout

**文件：** `agent/internal/container/containerd.go`

**实现方式：**
- 在 `Runtime` 接口新增 `GetContainerLogs(ctx, id) ([]byte, error)` 方法
- `containerdRuntime` 在 `StartContainer()` 中使用 `cio.WithStreams` 替代 `cio.WithStdio`，将 stdout/stderr 写入 `containerLogs` 缓冲
- `collectOutput` 改为 `Executor` 方法，调用 `e.runtime.GetContainerLogs()`
- 容器删除时自动清理日志缓冲区

**影响：** 作业完成后可通过 `UnitStatusReport.Output` 字段查看容器标准输出。

### 3.2 CPU/内存资源限制生效（P4）✅ 已完成

**状态：** 已实现，配置正确传递并应用于容器创建

**实现方式：**
- `agentcfg.Config.Resources` 新增 `MaxCPUCores`、`MaxMemoryMB` 字段
- `ToAgentConfig()` 新增传递这两个字段
- `applyResourceLimits()` 现在实际设置 agent 配置中的资源限制
- `Executor` 新增 `maxCPUCores`、`maxMemoryMB` 字段，在创建容器时传递给 `ContainerSpec.Resource`
- 内存值从 MB 自动转换为 Bytes（`maxMemoryMB * 1024 * 1024`）

**影响：** 用户在 Web 设置页面保存的 `max_cpu_cores` 和 `max_memory_mb` 值现在实际传递到容器运行时。

### 3.3 心跳间隔动态调整（P3）✅ 已完成

**状态：** 已取消注释并启用

**文件：** `agent/internal/heartbeat/reporter.go`，第 140-145 行

**影响：** 服务器可以在心跳响应中建议新的间隔，客户端现在会应用此字段。Proto 中 `HeartbeatInterval` 字段已定义，功能逻辑已取消注释。

### 3.4 启动时间追踪 ✅ 已完成

**状态：** 已实现，Handler 记录启动时间戳并计算差值

**文件：** `agent/internal/cpstart/server/handlers.go`

**影响：** `GET /api/v1/status` 返回的 `uptime_ms` 现在正确显示 Agent 运行时长。

---

## 四、调度器

### 4.1 Nebula 覆盖网络默认关闭

**状态：** 功能可选，默认不启用，可通过 Web 设置页面开关

**文件：** `agent/internal/nebula/manager.go`，第 57、103 行

```go
return fmt.Errorf("nebula is not enabled")
```

**影响：** NAT 穿透和节点间隧道（P6 功能）默认不启用。用户可在设置页面开启 "Nebula 覆盖网络" 开关，或通过配置 `nebula.enabled: true` 启用。

### 4.2 HAMi GPU 虚拟化默认关闭

**状态：** 功能可选，默认不启用，可通过 Web 设置页面开关

**文件：** `agent/internal/container/hami.go`，第 81 行

```go
return nil, fmt.Errorf("hami: not enabled")
```

**影响：** GPU 虚拟化与分片功能默认关闭。用户可在设置页面开启 "HAMI GPU 虚拟化" 开关，或通过配置 `hami.enabled: true` 启用。

### 4.3 自动更新器默认关闭

**状态：** 功能可选，默认不启用，可通过 Web 设置页面开关

**文件：** `pkg/updater/updater.go`，第 67 行

```go
log.Printf("updater: disabled")
return
```

**影响：** P10 打包分发与自动更新功能默认关闭。用户可在设置页面开启 "自动更新" 开关，或通过配置 `updater.enabled: true` 启用。

---

## 五、API 与前端

### 5.1 YAML 作业提交不支持

**状态：** 明确拒绝

**文件：** `agent/internal/cpstart/server/handlers.go`，第 269 行

```go
writeError(w, http.StatusBadRequest, "YAML submission not yet supported via REST API; use JSON SubmitJobRequest")
```

**影响：** 客户端仅能通过 JSON 格式提交作业。YAML 格式的 `Content-Type` 请求被拒绝（尽管 YAML 解析逻辑的入口已存在）。

### 5.2 实时数据更新 ✅ 已完成

**状态：** 已实现，采用轮询（Polling）方案

**实现方式：**
- 新建 `agent/ui/src/utils/usePolling.ts` — 通用 Vue composable，自动在 `onMounted` 时启动轮询，`onUnmounted` 时清理
- 各页面轮询间隔：
  - Dashboard：5s（节点列表 + 作业列表 + 本地状态）
  - JobsPage：10s（作业列表）
  - JobDetailPage：5s（运行中作业详情）
  - NodesPage：10s（节点列表）
  - NodeDetailPage：8s（节点详情）
- 轮询方案（而非 WebSocket/SSE）的原因：调度器已有 `WatchJob` gRPC 流式接口，但 cpstart 桥接层未暴露 SSE/WS 端点；轮询实现简单，5-10s 间隔对管理面场景足够

### 5.3 作业默认不自用 ✅ 已完成

**状态：** 已实现

**实现方式：**
- `Job` 结构体新增 `AllowSelfAssignment` 字段（默认 `false`）
- 调度器 `filterTrust` 在 `allowSelfAssignment=false` 时排除作业拥有者自身的节点
- 前端提交表单新增"允许分配到自己的节点"复选框

### 5.4 项目文件夹上传 ✅ 已完成

**状态：** 已实现，采用节点间 HTTP 下载方案

**流程：**
1. 用户通过 Web UI 上传 `.zip` 项目文件 + 填写启动命令 + 选择基础镜像
2. cpstart 存储到 `{data_dir}/projects/{project_id}/`，保存元数据
3. Job 携带 `project_id`、`startup_command`、`base_image` 提交到调度器
4. 调度器分配其他节点（默认不自用）
5. assign 命令中携带项目下载 URL（`http://{owner_overlay_ip}:8080/api/v1/projects/{project_id}/download`）
6. 被分配节点上的 Agent 下载项目 zip，解压到本地缓存，挂载到容器 `/workspace`
7. 容器使用 `base_image`，执行 `startup_command`

**新增/修改文件：**
- `agent/internal/cpstart/server/handlers.go` — 新增 `POST /projects/upload`、`GET /projects/{id}/download`、`GET /projects/{id}/status`
- `agent/internal/executor/executor.go` — 扩展 `AssignPayload`，新增 `downloadProject()` 方法（含 zip 解压和路径穿越防护）
- `scheduler/internal/scheduler/assigner.go` — `buildAssignCommand` 传递 project 信息，构造下载 URL
- `agent/ui/src/views/JobSubmitDialog.vue` — 新增"项目提交"模式（文件选择、启动命令、基础镜像选择）
- `agent/ui/src/api/client.ts` — 新增 `uploadProject()`、`getProjectStatus()` 接口

---

## 六、优先级汇总

| 优先级 | 项目 | 影响范围 | 预估工作量 |
|--------|------|----------|-----------|
| ~~**P3**~~ | ~~心跳间隔动态调整~~ | ✅ 已完成 | — |
| ~~**P4**~~ | ~~容器输出收集~~ | ✅ 已完成 | — |
| ~~**P4**~~ | ~~CPU/内存资源限制~~ | ✅ 已完成 | — |
| ~~**—**~~ | ~~启动时间追踪~~ | ✅ 已完成 | — |
| ~~**—**~~ | ~~实时数据更新（轮询）~~ | ✅ 已完成 | — |
| ~~**—**~~ | ~~作业默认不自用~~ | ✅ 已完成 | — |
| ~~**—**~~ | ~~项目文件夹上传~~ | ✅ 已完成 | — |
| ~~**P6**~~ | ~~mTLS 通信加密~~ | ✅ 已完成 | — |
| ~~**P6**~~ | ~~Nebula 覆盖网络启用~~ | ✅ 设置页面开关 | — |
| **—** | YAML 作业提交 | API 完整度 | 1 天 |

---

## 七、变更文件清单

### 已修复（本轮：安全与调度器完善）

| 文件 | 变更 |
|------|------|
| `scheduler/internal/ca/grpc.go` | **新增** gRPC mTLS CA 管理器 |
| `scheduler/internal/server/mtls.go` | **新增** mTLS 拦截器 + 节点身份提取 |
| `api/proto/v1/scheduler.proto` | `RegisterNodeResponse` 加 `grpc_certificate`、`grpc_private_key` 字段 |
| `proto/v1/scheduler.pb.go` | 对应 Go 结构体字段 |
| `scheduler/internal/config/config.go` | TLS 加 `Enabled` 字段 |
| `scheduler/cmd/scheduler/main.go` | 初始化 gRPC CA、生成服务器证书、启用 mTLS |
| `scheduler/internal/server/server.go` | `New()` 接收 `grpcCA`；`RegisterNode` 签发客户端证书 |
| `scheduler/internal/server/server_test.go` | 适配 `New()` 新增参数 |
| `agent/internal/config/config.go` | TLS 字段添加默认值 |
| `agent/internal/core/agent.go` | `Start()` 使用 mTLS 连接；`register()` 处理引导流程 |
| `cli/internal/client/client.go` | `New()` 支持 TLS 凭证 |
| `cli/internal/commands/root.go` | 加 `--tls-cert`/`--tls-key`/`--ca-cert` 全局标志 |
| `cli/internal/commands/node.go` | 传递 TLS 配置到 `client.New()` |
| `cli/internal/commands/register.go` | 同上 |
| `cli/internal/commands/trust.go` | 同上 |
| `cli/internal/commands/invite.go` | 同上 |
| `cli/internal/commands/job.go` | 同上 |
| `agent/internal/cpstart/config/config.go` | `Config` 加 `TLS` 段；`ToAgentConfig()` 映射 TLS 字段 |
| `agent/internal/cpstart/server/grpc.go` | `NewBridge` 接收 TLS 凭证；`connect()` 使用 TLS |
| `agent/cmd/cpstart/main.go` | 构建 TLS 凭证并传递给 Bridge |
| `scheduler/configs/scheduler.yaml` | 加 `tls.enabled: true` |
| `deploy/scheduler.yaml` | 加 `tls.enabled: true` |

| 文件 | 变更 |
|------|------|
| `agent/ui/src/utils/usePolling.ts` | **新增** 通用轮询 composable |
| `agent/ui/src/views/DashboardPage.vue` | 添加 5s 轮询自动刷新 |
| `agent/ui/src/views/JobsPage.vue` | 添加 10s 轮询自动刷新 |
| `agent/ui/src/views/JobDetailPage.vue` | 添加 5s 轮询（运行中作业） |
| `agent/ui/src/views/NodesPage.vue` | 添加 10s 轮询自动刷新 |
| `agent/ui/src/views/NodeDetailPage.vue` | 添加 8s 轮询自动刷新 |
| `proto/v1/common.pb.go` | Job 新增 `AllowSelfAssignment`、`ProjectID`、`StartupCommand`、`BaseImage` 字段 |
| `api/proto/v1/common.proto` | 对应 proto 字段定义 |
| `scheduler/internal/scheduler/filter.go` | `Filter()`/`filterTrust()` 支持 `allowSelfAssignment` 参数 |
| `scheduler/internal/scheduler/assigner.go` | `buildAssignCommand` 传递 project 信息；`nodePassesFilter` 支持 `allowSelfAssignment` |
| `scheduler/internal/server/server.go` | `SubmitJob` 设置 `AllowSelfAssignment` 默认值 |
| `scheduler/internal/scheduler/filter_test.go` | 测试用例适配新签名 |
| `scheduler/internal/scheduler/assigner_test.go` | 测试用例添加 `AllowSelfAssignment: true` |
| `agent/internal/cpstart/server/handlers.go` | 新增项目上传/下载/状态查询 3 个路由 |
| `agent/internal/cpstart/server/middleware.go` | 新增 `/api/v1/projects/` 路径例外（节点间下载） |
| `agent/internal/cpstart/server/server.go` | 监听地址从 `127.0.0.1` 改为 `0.0.0.0`（节点间访问） |
| `agent/internal/executor/executor.go` | 扩展 `AssignPayload`；新增 `downloadProject()` 方法（解压+路径穿越防护） |
| `agent/internal/executor/types.go` | 移除重复的 `AssignPayload` 和 `GPURequestPayload` 类型 |
| `agent/ui/src/views/JobSubmitDialog.vue` | 重写：新增"项目提交"模式（文件上传+启动命令+基础镜像） |
| `agent/ui/src/api/client.ts` | 新增 `uploadProject()`、`getProjectStatus()` 接口 |

### 待修复

| 文件 | 变更 |
|------|------|
| `agent/internal/cpstart/server/handlers.go` | 新增 `startTime` 追踪 + `uptime_ms` 计算；新增 `data_dir` 返回 |
| `agent/internal/cpstart/server/handlers.go` | 修复 ReportGPU 合并 bug |
| `agent/internal/heartbeat/reporter.go` | 取消注释心跳间隔动态调整代码 |
| `agent/internal/cpstart/agent/runner.go` | 实现 `applyResourceLimits()` 实际传递资源限制 |
| `agent/internal/cpstart/agent/runner.go` | 修复 `close of closed channel` panic（Stop/Start 竞态） |
| `agent/internal/cpstart/config/config.go` | 添加 `json` tag，修复 GPU/资源设置 JSON 解析丢失；`ToAgentConfig()` 传递 `MaxCPUCores`/`MaxMemoryMB` |
| `agent/internal/config/config.go` | 新增 `MaxCPUCores`/`MaxMemoryMB` 字段到 Resources |
| `agent/internal/core/agent.go` | `NewExecutor()` 调用传递资源限制参数 |
| `agent/internal/executor/executor.go` | `collectOutput()` 改为 `Executor` 方法，调用 `runtime.GetContainerLogs()`；容器 spec 使用配置的资源限制 |
| `agent/internal/container/runtime.go` | 接口新增 `GetContainerLogs(ctx, id) ([]byte, error)` |
| `agent/internal/container/containerd.go` | 实现日志缓冲捕获（`cio.WithStreams` 替代 `WithStdio`）；容器删除时清理日志；新增 `GetContainerLogs()` 方法 |
| `agent/internal/container/containerd.go` | 新增 Windows 运行时实现 |
| `agent/internal/container/gpu.go` | Windows 路径支持 |
| `agent/internal/cpstart/wsl2/automator.go` | **新增** WSL2 自动配置引擎（6 步异步流程，支持 `wsl --import` 免管理员安装） |
| `agent/ui/src/components/setup/StepWindows.vue` | **新增** 一键配置 WSL2 环境（实时进度轮询） |
| `scripts/setup-wsl2.ps1` | **新增** WSL2 环境手动安装脚本 |
| `scripts/start-wsl2-cpstart.sh` | **新增** WSL2 内部 cpstart 启动脚本 |
| `agent/ui/src/components/setup/StepIdentity.vue` | 新增数据目录输入框 |
| `agent/ui/src/components/setup/SetupWizard.vue` | 集成 WSL2 步骤 + 数据目录传递 |
| `agent/ui/src/api/client.ts` | 新增 WSL2 API 类型与函数；更新 `SetupConfig`/`SettingsConfig` 接口 |
| `agent/ui/src/views/SettingsPage.vue` | 数据目录卡片显示实际路径 |

