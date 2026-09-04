# 项目稳健性优化分析报告

> 生成日期：2026-09-01
> 目标：提升系统稳定性 + 降低用户技术门槛（单程序前端全控制）

---

## 目录

1. [关键 Bug & 稳定隐患](#一关键-bug--稳定隐患)
2. [前端控制力不足（用户需手动/CLI/SSH）](#二前端控制力不足用户需手动clissh)
3. [前端功能缺陷](#三前端功能缺陷)
4. [部署与更新短板](#四部署与更新短板)
5. [可观测性缺失](#五可观测性缺失)
6. [优先级汇总](#六优先级汇总)

---

## 一、关键 Bug & 稳定隐患

### 1.1 调度引擎并发竞态（P0）✅ 已修复

**问题：** `scheduleOnce()` 可从多个 goroutine 并发调用（定期 ticker + `ScheduleNow()` 每次调用启动新 goroutine），但**没有互斥锁保护**。两次并发调用可将同一 unit 分配给两个不同节点。

**文件：** `scheduler/internal/scheduler/engine.go` — `scheduleOnce()` 方法

**修复方案：**
- 新增 `scheduleMu sync.Mutex` 保护整个 `scheduleOnce()` 执行
- `ScheduleNow()` 改为通过 `wakeCh` 通道通知调度循环，不再启动新 goroutine
- 避免 `assignUnit()` 中 TOCTOU 竞态（先 GetUnit 再 SaveUnit）

**现状：** ✅ 已完成

---

### 1.2 孤立 Unit（节点断连后单元永久卡死）（P0）✅ 已修复

**问题：** 节点断连后，已分配给该节点的 unit 永远停留在 `Assigned` 或 `Running` 状态。`scheduleOnce()` 只取 `Pending` 和 `Failed` 状态的 unit，不会重新分配已卡住的 unit。Workflow 下游 stage 永远无法开始。

**文件：** `scheduler/internal/scheduler/engine.go` — `reclaimStaleUnits()`

**修复方案：**
- 新增 `reclaimStaleUnits()` 方法，在每次调度周期开始时扫描 `Assigned`/`Running` 但节点已 `Offline` 的 unit
- 被回收的 unit 标记为 `Failed`（"node offline"），走正常 retry 机制重新分配
- 配合 `scheduleMu` 互斥锁，保证并发安全

**现状：** ✅ 已完成

---

### 1.3 主备脑裂（无 fencing 机制）（P0）

**问题：** Standby 使用简单计数器（`failures >= maxFailures`）触发切换，**不检查主节点是否真死 vs 网络分区**。分区期间两个调度器独立运行，恢复后无合并机制。

**文件：** `scheduler/internal/server/standby.go` — `healthCheck()` 和 `promote()`

**修复方案：**
- 引入租约/分布式锁（etcd/consul 或基于 BoltDB 的文件锁）
- 或至少实现存活性探测的退避 + 确认机制

**估算工作量：** 2-3 天

---

### 1.4 数据库损坏无检测无恢复（P1）

**问题：** BoltDB 文件损坏时：
- 打开失败 → `log.Fatalf` 直接崩溃
- 无 `db.Check()` 完整性校验
- 无自动回滚到上一个 checkpoint
- `mustJSON()` 静默吞掉 JSON 编/解码错误，写入损坏数据

**文件：**
- `scheduler/internal/store/store.go:686` — `mustJSON()` 静默吞错误
- `scheduler/cmd/scheduler/main.go` — DB 打开失败直接退出

**修复方案：**
- 启动时运行 `db.Check()` 检查完整性
- 检测到损坏时自动从最新 checkpoint 恢复
- `mustJSON()` 改为返回错误而非写入损坏数据

**估算工作量：** 2 天

---

### 1.5 Dashboard `onlineCount` 永远为 0（P1）

**问题：** `DashboardPage.vue` 第 27 行定义了 `const onlineCount = ref(0)`，但**从未更新**，始终显示为 0。

**文件：** `agent/ui/src/views/DashboardPage.vue:27`

**影响：** 用户看到的在线节点数永远是 0，严重误导。

**修复方案：** 在 `fetchData()` 中计算 `nodes.filter(n => n.status === 'online').length`

**估算工作量：** 0.5 天

---

### 1.6 前端轮询错误静默吞掉（P2）

**问题：** `usePolling.ts` 第 25-26 行 `catch` 块为空，所有轮询请求错误被静默吞掉。用户无法感知网络问题或后端故障。

**文件：** `agent/ui/src/utils/usePolling.ts:25-26`

**修复方案：** 至少 `console.warn`，可选显示 `ElMessage` 提示

**估算工作量：** 0.5 天

---

## 二、前端控制力不足（用户需手动/CLI/SSH）

### 2.1 节点管理缺失（P0）

**用户无法从前端：**
| 操作 | 当前方式 | 影响 |
|------|---------|------|
| 取消注册/移除节点 | 无（protobuf 有 `UnregisterNode`） | 节点永驻集群 |
| 屏蔽/解屏蔽节点 | 无（protobuf 有 `block_list`） | 恶意节点无法隔离 |
| 暂停/恢复节点 | 无（protobuf 有 `NODE_STATUS_SUSPENDED`） | 无法临时停用节点 |
| 查看节点终端/SSH | 无 | 需 SSH 手动登录 |

**文件：** `agent/ui/src/views/NodesPage.vue`、`NodeDetailPage.vue`

**修复方案：**
- 节点列表增加右键菜单或操作列（取消注册、屏蔽、暂停）
- 节点详情页添加操作按钮

**估算工作量：** 2 天

---

### 2.2 作业输出与日志查看（P0）

**问题：** protobuf 中 `Unit` 有 `output` 字段（`common.proto:157`），但前端**完全不展示作业输出**。用户无法看到容器 stdout/stderr。

**文件：** `agent/ui/src/views/JobDetailPage.vue`

**影响：** 作业失败后用户不知道原因，需手动 SSH 到节点查日志。

**修复方案：**
- JobDetailPage 添加"输出"标签页，显示 unit 的 stdout
- 支持日志下载

**估算工作量：** 1 天

---

### 2.3 失败作业重试与重新提交（P0）

**问题：** 作业失败后，前端没有"重试"按钮或"重新提交"功能。用户只能从头再次填写表单。

**文件：** `agent/ui/src/views/JobDetailPage.vue`

**修复方案：**
- 在 JobDetailPage 添加"重试"按钮（将失败 unit 重置为 Pending）
- 添加"克隆并重新提交"功能（预填原始作业参数）

**估算工作量：** 1 天

---

### 2.4 证书管理（P1）

**问题：** protobuf 定义了 `IssueCertificate`、`RenewCertificate`、`RevokeCertificate`，但前端没有任何证书管理界面。

**文件：** `agent/ui/src/views/SettingsPage.vue`、`api/proto/v1/scheduler.proto:32-34`

**修复方案：**
- SettingsPage 添加证书管理卡片
- 显示证书过期时间、签发状态
- 续期/吊销按钮

**估算工作量：** 1.5 天

---

### 2.5 作业提交功能不足（P1）

**问题：** 前端作业提交表单缺少以下 protobuf 已支持的字段：

| 字段 | 位置 | 影响 |
|------|------|------|
| GPU 请求（`GPURequest`） | `common.proto:91-98` | 无法提交 GPU 作业 |
| 拆分策略（`SplitStrategy`） | `common.proto:133-139` | 无法提交分布式作业 |
| 超时时间（`max_duration_ms`） | `common.proto:184` | 作业可能无限运行 |
| 重试次数（`max_retries`） | `common.proto:183` | 失败后不自动重试 |
| 失败策略（`failure_policy`） | `common.proto:182` | 无法控制失败行为 |
| 多阶段工作流 | - | 只能提交单 stage 作业 |

**文件：** `agent/ui/src/views/JobSubmitDialog.vue`

**修复方案：** 在提交表单中逐步添加这些字段

**估算工作量：** 2 天

---

### 2.6 邀请码创建缺少参数（P2）

**问题：** protobuf 中 `CreateInviteCodeRequest` 有 `expires_at` 和 `max_uses` 字段，但前端发送 `{}` 空对象。

**文件：** `agent/ui/src/views/InvitePage.vue`

**修复方案：** 添加过期时间和最大使用次数输入

**估算工作量：** 0.5 天

---

### 2.7 设置编辑不完整（P2）

**问题：** 用户从前端无法更改：
- Agent 名称（需重新运行设置向导）
- 调度器地址（需重新运行设置向导）
- 数据目录（只读显示）

**文件：** `agent/ui/src/views/SettingsPage.vue`

**修复方案：** 在 SettingsPage 添加这些字段的直接编辑功能

**估算工作量：** 1 天

---

## 三、前端功能缺陷

### 3.1 无状态管理 = 页面间状态丢失（P2）

**问题：** 没有 Vuex/Pinia 状态管理，每个页面独立 fetch 数据。页面切换后缓存丢失，每次都要重新加载。

**修复方案：** 引入 Pinia，缓存节点列表、作业列表等公共数据

**估算工作量：** 2 天

---

### 3.2 无实时推送（仅轮询）（P2）

**问题：** 所有数据更新使用轮询（Dashboard 5s、Jobs 10s、JobDetail 5s、Nodes 10s、NodeDetail 8s），无 WebSocket/gRPC 流。延迟高，服务端负载大。

**修复方案：**
- 使用 `WatchJob` gRPC 流替代作业轮询
- 或引入 WebSocket 网关

**估算工作量：** 3 天

---

### 3.3 无分页加载（P2）

**问题：** 作业列表和节点列表一次性加载全部数据。protobuf 已定义 `page_size`、`page_token` 但前端未使用。

**文件：** `agent/ui/src/views/JobsPage.vue`、`NodesPage.vue`

**修复方案：** 后端已支持分页参数，前端添加分页组件

**估算工作量：** 1 天

---

### 3.4 无确认对话框（P2）

**问题：** 只有取消作业有确认对话框，其他破坏性操作（吊销信任、修改设置会重启 agent）没有确认。

**修复方案：** 添加 Element Plus `ElMessageBox.confirm` 统一确认

**估算工作量：** 0.5 天

---

### 3.5 无作业删除功能（P2）

**问题：** 只能取消作业，不能删除/清理已完成或已取消的作业。作业列表会无限增长。

**修复方案：** 添加删除按钮（protobuf 需先添加 `DeleteJob` RPC）

**估算工作量：** 1 天（含 proto 修改）

---

## 四、部署与更新短板

### 4.1 更新 URL 硬编码占位符（P1）

**问题：** 更新清单 URL 硬编码为 `https://update.computing-power.local/v1/releases/manifest.json`，实际无此域名。用户无法收到更新。

**文件：** `pkg/updater/updater.go` — manifest URL 常量

**修复方案：** 改为可配置项，默认值可从部署脚本注入

**估算工作量：** 0.5 天

---

### 4.2 无健康检查/状态页面（P1）

**问题：** 调度器无 HTTP 健康检查端点（HTTP 服务器默认关闭），无 Prometheus 指标。运维人员无法监控系统状态。

**文件：** `scheduler/internal/config/config.go:25` — `Server.HTTP.Enabled` 从未读取

**修复方案：**
- 启用 HTTP 服务器（默认监听 `:9091`）
- 添加 `/health`、`/metrics` 端点
- 添加 Prometheus 指标：节点数、作业数、调度延迟

**估算工作量：** 2 天

---

### 4.3 自动更新状态不可见（P2）

**问题：** 前端有 updater 开关，但用户无法查看更新状态（当前版本、最新版本、更新进度、上次检查时间）。

**文件：** `agent/ui/src/views/SettingsPage.vue`

**修复方案：**
- SettingsPage 添加版本信息卡片
- 显示当前版本、最新版本、检查更新按钮
- 更新进度条

**估算工作量：** 1 天

---

## 五、可观测性缺失

### 5.1 无结构化日志（P2）

**问题：** 全项目使用 `log.Printf` 而非结构化日志（zap/zerolog）。无法按级别过滤、无法 JSON 输出、无法接入日志收集系统。

**影响：** 故障排查困难，生产环境无法有效监控

**修复方案：** 引入 `go.uber.org/zap`，支持 JSON 输出、日志级别、 caller 追踪

**估算工作量：** 3 天

---

### 5.2 mTLS 身份提取但未使用（P2）

**问题：** `GetAuthenticatedNodeID()` 已实现（`mtls.go`），但从未在任何 handler 中调用。mTLS 客户端证书的身份信息未被用于鉴权。

**文件：** `scheduler/internal/server/mtls.go` — `GetAuthenticatedNodeID()`

**修复方案：** 在敏感 RPC（`CancelJob`、`RevokeTrust` 等）中校验调用者身份

**估算工作量：** 1 天

---

### 5.3 事件推送不可靠（P3）

**问题：** 事件总线是纯内存的，channel buffer 64，慢消费者静默丢事件。`WatchJob` 流可能悄无声息地漏掉事件。

**文件：** `scheduler/internal/events/bus.go`

**修复方案：** 添加有界阻塞或背压机制

**估算工作量：** 1 天

---

## 六、优先级汇总

| 优先级 | 项目 | 类别 | 影响 | 预估工作量 | 状态 |
|--------|------|------|------|-----------|------|
| **P0** | 调度引擎并发竞态 | 稳定隐患 | 同一 unit 被多次分配 | 0.5 天 | ✅ 已修复 |
| **P0** | 孤立 Unit 永久卡死 | 稳定隐患 | 节点断连后作业无法恢复 | 1 天 | ✅ 已修复 |
| **P0** | 主备脑裂（无 fencing） | 稳定隐患 | 双调度器数据不一致 | 2-3 天 | 待修复 |
| **P0** | 节点管理（移除/屏蔽/暂停） | 前端控制力 | 用户无法管理集群节点 | 2 天 | ✅ 已修复(注销) |
| **P0** | 作业输出与日志查看 | 前端控制力 | 用户不知作业运行结果 | 1 天 | ✅ 已修复 |
| **P0** | 失败作业重试与重新提交 | 前端控制力 | 失败后需手动重填表单 | 1 天 | ✅ 已修复 |
| **P0** | registry RLock 下修改节点对象 | 数据竞争 | 并发读写导致未定义行为 | 0.5 天 | ✅ 已修复 |
| **P0** | CancelJob Stage 状态丢失 | 稳定隐患 | 取消作业时 stage 状态变更被覆盖 | 0.5 天 | ✅ 已修复 |
| **P1** | 数据库损坏无检测无恢复 | 稳定隐患 | 数据损坏时直接崩溃 | 2 天 | 待修复 |
| **P1** | Dashboard onlineCount 永远为 0 | 关键 Bug | 显示错误数据 | 0.5 天 | 待修复 |
| **P1** | 证书管理界面 | 前端控制力 | 无法管理证书 | 1.5 天 | 待修复 |
| **P1** | 作业提交功能不足（GPU/拆分策略/超时） | 前端控制力 | 高级功能无法使用 | 2 天 | 待修复 |
| **P1** | 更新 URL 硬编码占位符 | 部署短板 | 自动更新不可用 | 0.5 天 | 待修复 |
| **P1** | 健康检查与状态页面 | 可观测性 | 无法监控系统状态 | 2 天 | 待修复 |
| **P2** | 前端轮询错误静默吞掉 | 前端缺陷 | 用户不知网络故障 | 0.5 天 | 待修复 |
| **P2** | 设置编辑不完整 | 前端控制力 | 需重跑设置向导改配置 | 1 天 | 待修复 |
| **P2** | 邀请码创建参数缺失 | 前端控制力 | 无法设置过期/使用次数 | 0.5 天 | 待修复 |
| **P2** | 无状态管理 | 前端缺陷 | 页面切换缓存丢失 | 2 天 | 待修复 |
| **P2** | 无实时推送（仅轮询） | 前端缺陷 | 高延迟、高负载 | 3 天 | 待修复 |
| **P2** | 无分页加载 | 前端缺陷 | 大数据量卡顿 | 1 天 | 待修复 |
| **P2** | 无确认对话框 | 前端缺陷 | 误操作风险 | 0.5 天 | 待修复 |
| **P2** | 无作业删除功能 | 前端控制力 | 列表无限增长 | 1 天 | 待修复 |
| **P2** | 自动更新状态不可见 | 前端控制力 | 用户不知更新状态 | 1 天 | 待修复 |
| **P2** | 无结构化日志 | 可观测性 | 排查困难 | 3 天 | 待修复 |
| **P2** | mTLS 身份未使用 | 安全隐患 | 未校验调用者身份 | 1 天 | 待修复 |
| **P3** | 事件推送不可靠 | 稳定隐患 | 静默丢事件 | 1 天 | 待修复 |

---

## 核心结论

### P0 修复状态（截至 2026-09-01）

| P0 问题 | 状态 |
|---------|------|
| 调度引擎并发竞态 | ✅ 已修复（`scheduleMu` 互斥锁 + `wakeCh` 通道） |
| 孤立 Unit 卡死 | ✅ 已修复（`reclaimStaleUnits()` 自动回收） |
| registry RLock 数据竞争 | ✅ 已修复（返回节点副本） |
| CancelJob Stage 状态丢失 | ✅ 已修复（改用 `SaveJob()` 持久化） |
| 节点管理缺失 | ✅ 已修复（注销节点 REST + 前端按钮） |
| 作业输出不可见 | ✅ 已修复（单元输出弹窗） |
| 失败作业无法重试 | ✅ 已修复（重试 + 克隆重新提交） |
| 注销节点未清理 Unit | ✅ 已修复（`reclaimNodeUnits()` + 立即调度） |
| WAL 写入错误静默忽略 | ✅ 已修复（`walWrite()` 记录错误日志） |
| 工作流上游失败不传播下游 | ✅ 已修复（`cancelDownstreamStages()` BFS 级联 Skipped） |
| Checkpointer 误删备机未回放 WAL | ✅ 已修复（基于备机 ack 序号的安全清理） |
| 主备脑裂 | ⏳ 待修复（架构级变更，需引入分布式锁/租约） |

### 关键优化方向

1. **稳定第一**：并发竞态、孤立 unit、WAL 安全清理已修复；下一步解决脑裂、数据库损坏检测
2. **前端全控制**：节点注销、日志查看、失败重试已补齐；下一步证书管理、作业高级字段
3. **可观测性**：健康检查端点、指标暴露、结构化日志
4. **部署完善**：更新 URL 可配置、更新状态可见