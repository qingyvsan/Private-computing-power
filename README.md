# Private Computing Power · 私域算力

> 去中心化个人算力共享平台 —— 将家庭/个人闲置算力（CPU / GPU / 内存 / 磁盘）汇聚成可自由交易的算力市场。

把身边闲置的算力用起来：算力提供者共享资源获得收益，算力需求者按需租用，全程通过浏览器即可操作。

---

## 核心特性

- **中央调度 + 分布式节点**：调度器作为无界面控制面（gRPC），节点 Agent 分布式接入
- **三层任务模型**：`Job → Stage → Unit`，支持单体 / 聚合 / 工作流，4 种拆分策略（by_file / by_range / by_n / by_custom）
- **φ-accrual 故障检测**：自适应超时判断节点健康，误判率可控
- **信任图**：有向信任关系 + ECDSA 签名，仅信任节点间可调度
- **Web 控制台**：用户主机一键启动（`cpstart`），浏览器完成首次配置向导后进入算力网络
- **GPU 共享**：HAMi Standalone 模式显存隔离（规划中）
- **Overlay 网络**：Nebula + Lighthouse NAT 穿透（规划中）
- **邀请码注册**：自由社区共享模型（规划中）

## 快速开始（统一使用流程）

```
① 中央服务器：部署调度器（headless，仅 gRPC :9090）
② 打包分发：客户端压缩包发给用户（cpstart + agent + 内嵌前端）
③ 用户启动：解压 → 双击 cpstart → 自动拉起服务 → 浏览器打开控制台
④ 首次配置向导：连接调度器 / 节点身份+邀请码 / 共享资源 / 环境检测
⑤ 进入算力网络：节点注册上线，控制台开始操作
```

### 部署调度器

```bash
# 构建
go build -o bin/scheduler ./scheduler/cmd/scheduler

# 运行（监听 gRPC :9090）
./bin/scheduler --config ./scheduler/configs/scheduler.yaml
```

### 用户主机一键启动（P3.5 起）

```bash
# 解压客户端包，双击 cpstart（Windows）/ ./cpstart（Linux/macOS）
# 浏览器自动打开 http://127.0.0.1:8080
# 按向导完成首次配置 → 节点上线 → 开始操作
```

## 目录结构

```
├── api/proto/v1/            # Protobuf 规范（common / scheduler）
├── proto/v1/                # Go 类型 + JSON codec
├── pkg/                     # 共享库（phidetector / trustgraph / taskmodel / resource / wal / certutil ...）
├── scheduler/               # 中央调度器（store / registry / server）
├── agent/                   # 节点 Agent（heartbeat / container / core）
├── cli/                     # cpcli 命令行 + cpstart 一键启动
├── web/                     # Web 控制台前端（P3.5）
└── test/fixtures/           # 作业 YAML 测试用例
```

## 技术栈

| 项 | 选型 |
|----|------|
| 语言 | Go 1.26+（单二进制） |
| 通信 | gRPC（控制流） |
| 存储 | BoltDB（嵌入式） |
| 界面 | Vue 3 + Vite + Element Plus（Web 控制台） |
| CLI | Cobra |
| 配置 | YAML |

## 开发

```bash
# 构建三个二进制
go build -o bin/scheduler ./scheduler/cmd/scheduler
go build -o bin/agent      ./agent/cmd/agent
go build -o bin/cpcli      ./cli/cmd/cpcli

# 测试
go test -race ./pkg/... ./scheduler/... ./agent/... ./cli/...
```

## 路线图

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 项目骨架 + gRPC 握手 | ✅ 已完成 |
| P1 | 注册与心跳强化 + φ 故障检测 | ⬜ |
| P2 | 任务模型落库（BoltDB） | ⬜ |
| P3 | 调度引擎（过滤/评分/分配） | ⬜ |
| P3.5 | 用户主机 Web 控制台 + 一键启动 | ⬜ |
| P4 | Agent 容器执行 | ⬜ |
| P5 | GPU 管理（HAMi） | ⬜ |
| P6 | Nebula Overlay 网络 | ⬜ |
| P7 | 信任图 | ⬜ |
| P8 | WAL 热备 | ⬜ |
| P9 | 邀请码 | ⬜ |
| P10 | 打包分发 | ⬜ |

## 许可证

尚未指定。
