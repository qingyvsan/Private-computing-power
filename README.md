# Private Computing Power · 私域算力

> 去中心化私域算力集群 —— 将家庭/个人闲置算力（CPU / GPU / 内存 / 磁盘）汇聚成统一、自主可控的计算集群。

把身边闲置的算力用起来：集中调度、统一编排，算力始终掌握在自己人手里，全程通过浏览器即可操作。

---

## 核心特性

- **中央调度 + 分布式节点**：调度器作为无界面控制面（gRPC），节点 Agent 分布式接入
- **三层任务模型**：`Job → Stage → Unit`，支持单体 / 聚合 / 工作流，4 种拆分策略（by_file / by_range / by_n / by_custom）
- **φ-accrual 故障检测**：自适应超时判断节点健康，误判率可控
- **信任图**：有向信任关系 + ECDSA 签名，仅信任节点间可调度
- **Web 控制台**：用户主机一键启动（`cpstart`），浏览器完成首次配置向导后加入集群
- **GPU 共享**：HAMi Standalone 模式显存隔离
- **Overlay 网络**：Nebula + Lighthouse NAT 穿透，节点间 P2P 通信
- **邀请码注册**：邀请制接入私域成员，可选管理员密钥
- **WAL 热备**：双节点故障切换 ≤ 30s，RPO ≤ 5s
- **自动更新**：内建更新器，定时检查 + 下载 + 校验 + 原子替换 + 备份回滚
- **跨平台客户端**：5 平台分发包（linux/amd64+arm64, windows/amd64, darwin/amd64+arm64）

## 快速开始（统一使用流程）

```
① 中央服务器：部署调度器（headless，仅 gRPC :9090）
② 打包分发：`make dist VERSION=v0.1.0` 产出 5 平台分发包
③ 用户安装：一键安装脚本自动下载 + 解压 + 注册系统服务
④ 首次配置向导：连接调度器 / 节点身份+邀请码 / 共享资源 / 环境检测
⑤ 加入集群：节点注册上线，控制台开始操作
```

### 部署调度器

```bash
# 构建
go build -o bin/scheduler ./scheduler/cmd/scheduler

# 运行（监听 gRPC :9090）
./bin/scheduler --config ./scheduler/configs/scheduler.yaml
```

### 安装 Agent（一键安装）

```bash
# Linux/macOS
curl -fsSL https://update.computing-power.local/install.sh | bash
INVITE_CODE=xxxx bash scripts/install.sh

# Windows
powershell -ExecutionPolicy Bypass -File install.ps1
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
├── pkg/                     # 共享库（phidetector / trustgraph / taskmodel / resource / wal / certutil / updater ...）
├── scheduler/               # 中央调度器（store / registry / server）
├── agent/                   # 节点 Agent（heartbeat / container / core）
├── cli/                     # cpcli 命令行 + cpstart 一键启动
├── web/                     # Web 控制台前端（P3.5）
├── scripts/                 # 打包分发 + 安装脚本
├── deploy/                  # 部署工具（scheduler systemd + Nebula Lighthouse）
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

## 构建

```bash
# 构建三个二进制
go build -o bin/scheduler ./scheduler/cmd/scheduler
go build -o bin/agent      ./agent/cmd/agent
go build -o bin/cpcli      ./cli/cmd/cpcli

# 构建一键启动器（含内嵌前端）
make build-cpstart

# 全平台交叉编译 + 打包分发
make dist VERSION=v0.1.0

# 测试
go test -race ./pkg/... ./scheduler/... ./agent/... ./cli/...
```

## 路线图

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 项目骨架 + gRPC 握手 | ✅ 已完成 |
| P1 | 注册与心跳强化 + φ 故障检测 | ✅ 已完成 |
| P2 | 任务模型落库（BoltDB） | ✅ 已完成 |
| P3 | 调度引擎（过滤/评分/分配） | ✅ 已完成 |
| P3.5 | 用户主机 Web 控制台 + 一键启动 | ✅ 已完成 |
| P4 | Agent 容器执行 | ✅ 已完成 |
| P5 | GPU 管理（HAMi） | ✅ 已完成 |
| P6 | Nebula Overlay 网络 | ✅ 已完成 |
| P7 | 信任图 | ✅ 已完成 |
| P8 | WAL 热备 | ✅ 已完成 |
| P9 | 邀请码 | ✅ 已完成 |
| P10 | 打包分发 + 自动更新 + 安装脚本 | ✅ 已完成 |
| 远期 | 自动任务拆分 | ⬜ 规划中 |

## 许可证

尚未指定。
