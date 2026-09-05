# Private Computing Power — 去中心化私域算力集群

> **Turn idle home/personal compute into a unified, self-sovereign cluster — operated entirely from your browser.**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![gRPC](https://img.shields.io/badge/gRPC-protocol-00ADD8)](https://grpc.io/)
[![Vue](https://img.shields.io/badge/Vue-3.x-4FC08D?logo=vue.js)](https://vuejs.org/)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Private Computing Power pools your idle CPU, GPU, memory, and disk across home/personal devices into a **self-controlled distributed cluster**. Think of it as a personal, privacy-first alternative to cloud computing — your compute, your rules.

---

## 🎯 Why This Exists

| Problem                                         | Solution                                                     |
| ----------------------------------------------- | ------------------------------------------------------------ |
| Cloud GPU costs are prohibitive for individuals | Aggregate idle home GPUs into a free cluster                 |
| Centralized compute = data privacy risk         | Your data never leaves your trusted network                  |
| Distributed systems are hard to set up          | One-click `cpstart` + browser-based management               |
| Trusting strangers with your hardware           | ECDSA-signed trust graph — only trusted peers can schedule jobs |

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    CENTRAL SCHEDULER (gRPC :9090)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │ Resource │  │  Trust   │  │  Health  │  │   Scheduling  │  │
│  │ Tracker  │  │  Graph   │  │  Monitor │  │   Engine      │  │
│  └──────────┘  └──────────┘  └──────────┘  └───────────────┘  │
│                                                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │  WAL +   │  │  Auto    │  │  Invite  │                     │
│  │ Hot-Standby│ │  Updater │  │  Code    │                     │
│  └──────────┘  └──────────┘  └──────────┘                     │
└──────────────┬──────────────────────────────────────────────────┘
               │
    ┌──────────┼──────────┬──────────────┐
    ▼          ▼          ▼              ▼
┌────────┐ ┌────────┐ ┌────────┐   ┌────────┐
│ Node A │ │ Node B │ │ Node C │ … │ Node N │  (thousands)
│ 8C GPU │ │ 16C CPU│ │ 32G RAM│   │ 4C GPU │
└────────┘ └────────┘ └────────┘   └────────┘
    │          │          │              │
    └──────────┴──────────┴──────────────┘
         Nebula Overlay Network + NAT Traversal
```

---

## 🔑 Key Features

### 1. Distributed Task Model (3-Layer)

```
Job "Train ML Model"
 ├── Stage "Data Preprocessing"
 │    ├── Unit: chunk 1/4
 │    ├── Unit: chunk 2/4
 │    ├── Unit: chunk 3/4
 │    └── Unit: chunk 4/4
 ├── Stage "Model Training"
 │    └── Unit: full training job
 └── Stage "Evaluation"
      └── Unit: run eval script
```

**4 Split Strategies:**

- `by_file` — split by file list
- `by_range` — split by numeric range
- `by_n` — split into N equal parts
- `by_custom` — user-defined split function

**3 Job Types:**

- `Singular` — single unit, single node
- `Aggregate` — multiple independent units, parallel
- `Workflow` — DAG of stages with dependencies (Kahn's algorithm topological sort)

### 2. Pipeline Scheduling Engine

```
Filter → Score → Assign
  │        │        │
  │        │        └── Concurrent dispatch + retry
  │        │
  │        └── 4-dimensional weighted scoring:
  │            • ResourceMatch (CPU/GPU/RAM match)
  │            • NetworkQuality (latency, bandwidth)
  │            • Reputation (historical success rate)
  │            • Load (current utilization)
  │
  └── 3 filters:
       • Resource (enough CPU/GPU/RAM?)
       • Trust (is this node trusted?)
       • Health (is the node alive?)
```

### 3. φ-Accrual Adaptive Failure Detection

Instead of simple heartbeats (which produce false positives under network jitter), φ-accrual analyzes the **distribution of historical heartbeat intervals** to compute a suspicion level φ:

- **φ = 1** → ~10% false positive probability
- **φ = 4** → ~0.1% false positive probability (used as default threshold)
- **φ = 8** → ~0.0001% false positive probability

This means the system adapts to network conditions — a node on flaky WiFi won't be falsely marked as dead.

### 4. ECDSA Trust Graph

```
       ┌──────────────────────┐
       │   Trust Relationship  │
       │  A ──signs──▶ B      │
       │  (ECDSA P-256)       │
       └──────────────────────┘
       
       Job scheduling: A → B → C → D
       BFS path verification (depth limit: 10)
       Only trusted transitive paths allowed
       Expired edges auto-cleaned
```

### 5. High Availability

| Feature           | Spec                                  |
| ----------------- | ------------------------------------- |
| **WAL Journal**   | JSONL + CRC32 checksum, auto-rotation |
| **Hot Standby**   | Dual-node active/passive              |
| **Failover**      | ≤ 30 seconds                          |
| **RPO**           | ≤ 5 seconds (data loss window)        |
| **Auto Recovery** | WAL replay on restart                 |

### 6. GPU Sharing with HAMi

```
┌─────────────────────────────────┐
│         Physical GPU            │
│  ┌──────┐ ┌──────┐ ┌──────┐   │
│  │ 2GB  │ │ 4GB  │ │ 2GB  │   │  ← HAMi Standalone
│  │ Job1 │ │ Job2 │ │ Job3 │   │     memory isolation
│  └──────┘ └──────┘ └──────┘   │
└─────────────────────────────────┘
```

### 7. Auto-Update System

```
Check → Download → Verify (SHA256) → Atomic Replace → Rollback on Failure
```

---

## 🚀 Quick Start

### One Command

```bash
cpstart
```

This opens `http://127.0.0.1:8080` in your browser — fill in identity, invite code, and resource sharing preferences, and you're in the cluster.

### Manual Setup

```bash
# Build from source
git clone https://github.com/qingyvsan/Private-computing-power.git
cd Private-computing-power
make build

# Start scheduler
./bin/scheduler --config config.yaml

# Start node agent
./bin/agent --scheduler-addr localhost:9090 --config config.yaml
```

### Cross-Platform Build

```bash
make dist VERSION=v0.1.0
# Produces packages for:
#   linux/amd64, linux/arm64
#   windows/amd64
#   darwin/amd64, darwin/arm64
```

---

## 🛡️ Security

| Layer              | Mechanism                                 |
| ------------------ | ----------------------------------------- |
| **Transport**      | mTLS (mutual TLS) on all gRPC connections |
| **Identity**       | ECDSA P-256 key pairs                     |
| **Authorization**  | Trust graph (BFS path verification)       |
| **Network**        | Nebula Overlay — encrypted mesh           |
| **Access Control** | Invite-code registration                  |

---

## 🛠️ Tech Stack

| Component    | Technology                  | Why                                      |
| ------------ | --------------------------- | ---------------------------------------- |
| **Language** | Go 1.26+                    | Single binary, no runtime dependency     |
| **RPC**      | gRPC + Protobuf             | Strong typing, streaming, bidirectional  |
| **Storage**  | BoltDB                      | Embedded, zero-config, ACID transactions |
| **Frontend** | Vue 3 + Vite + Element Plus | Reactive UI, small bundle                |
| **CLI**      | Cobra                       | POSIX-compliant, auto-completion         |
| **Overlay**  | Nebula + Lighthouse         | NAT traversal, encrypted P2P mesh        |
| **GPU**      | HAMi Standalone             | GPU memory isolation at container level  |
| **Config**   | YAML                        | Human-readable, version-controlled       |

---

## 📁 Project Structure

```
├── scheduler/          # Central scheduler (gRPC server)
├── agent/              # Distributed node agent
├── cli/                # CLI + cpstart launcher
├── api/proto/v1/       # Protobuf service definitions
├── pkg/                # Shared libraries
│   ├── trust/          # ECDSA trust graph
│   ├── detection/      # φ-accrual failure detector
│   ├── scheduler/      # Pipeline scheduling engine
│   └── wal/            # Write-ahead log
├── web/                # Vue 3 dashboard
├── scripts/            # Build + packaging
├── deploy/             # Deployment configs
└── test/fixtures/      # Integration test data
```

---

## 📊 Progress

| Phase  | Feature                     | Status |
| ------ | --------------------------- | ------ |
| P0     | Project skeleton + proto    | ✅      |
| P1     | Registration + heartbeat    | ✅      |
| P2     | φ-accrual failure detection | ✅      |
| P3     | Task model (Job/Stage/Unit) | ✅      |
| P4     | Pipeline scheduling engine  | ✅      |
| P5     | Web console                 | ✅      |
| P6     | Container execution         | ✅      |
| P7     | GPU sharing (HAMi)          | ✅      |
| P8     | Nebula overlay network      | ✅      |
| P9     | Trust graph                 | ✅      |
| P10    | WAL + hot standby           | ✅      |
| P11    | Packaging + auto-update     | ✅      |
| Future | Auto task splitting         | ⬜      |

---

## 🎓 Design Decisions

### Why Go?

A single statically-linked binary means zero runtime dependencies. Users drop one file and run it. Go's goroutine model is a natural fit for the scheduler's concurrent operations (thousands of agent connections, each with streaming gRPC).

### Why BoltDB instead of PostgreSQL?

BoltDB is an embedded database — no separate process, no configuration, no network. The scheduler's state is a single file. For a self-hosted tool targeting home users, this is the right tradeoff: ACID guarantees without operational complexity.

### Why φ-accrual instead of simple timeout?

Simple heartbeat timeouts produce false positives under network jitter. φ-accrual computes a statistical suspicion level from historical intervals, dramatically reducing false failure detections. This is the same algorithm used by Cassandra and Akka.

---

## 📄 License

MIT — see [LICENSE](LICENSE) for details.

*Built to prove that distributed computing doesn't need to be complex — or centralized.*
