# Computing Power - 去中心化个人算力共享平台
# 构建入口

GO := go
VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "\
	-X computing-power/pkg/version.Version=$(VERSION) \
	-X computing-power/pkg/version.GitCommit=$(GIT_COMMIT) \
	-X computing-power/pkg/version.BuildTime=$(BUILD_TIME) \
	-s -w"

PLATFORMS := linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64
BIN_DIR := bin

.PHONY: proto build build-all test test-coverage lint clean \
	dev-scheduler dev-agent \
	build-scheduler build-agent build-cli build-ui build-cpstart build-cpstart-linux dev-cpstart \
	run-scheduler run-agent

# ========== Protobuf 生成 ==========
# 注：protoc 生成需要安装 protoc + protoc-gen-go，当前使用手工编写的 Go 类型
proto:
	@echo ">>> Generating protobuf code..."
	@echo ">>> (当前使用手工编写的 Go 类型，无需 protoc)"

# ========== 构建 ==========
build: build-scheduler build-agent build-cli

build-scheduler:
	cd scheduler && $(GO) build $(LDFLAGS) -o ../$(BIN_DIR)/scheduler ./cmd/scheduler

build-agent:
	cd agent && $(GO) build $(LDFLAGS) -o ../$(BIN_DIR)/agent ./cmd/agent

build-cli:
	cd cli && $(GO) build $(LDFLAGS) -o ../$(BIN_DIR)/cpcli ./cmd/cpcli

build-ui:
	cd agent/ui && npm install && npm run build

build-cpstart: build-ui
	cd agent && $(GO) build $(LDFLAGS) -o ../$(BIN_DIR)/cpstart ./cmd/cpstart

build-cpstart-linux: build-ui
	cd agent && GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o ../$(BIN_DIR)/cpstart-linux ./cmd/cpstart

dev-cpstart:
	cd agent && $(GO) run ./cmd/cpstart --config ./configs/cpstart.yaml

# ========== 全平台交叉编译 ==========
build-all:
	@$(foreach platform,$(PLATFORMS), \
		echo ">>> Building $(platform)..."; \
		GOOS=$(word 1,$(subst /, ,$(platform))) \
		GOARCH=$(word 2,$(subst /, ,$(platform))) \
		$(GO) build $(LDFLAGS) \
			-o $(BIN_DIR)/$(subst /,-,$(platform))-scheduler \
			./scheduler/cmd/scheduler; \
		$(GO) build $(LDFLAGS) \
			-o $(BIN_DIR)/$(subst /,-,$(platform))-agent \
			./agent/cmd/agent; \
		$(GO) build $(LDFLAGS) \
			-o $(BIN_DIR)/$(subst /,-,$(platform))-cpcli \
			./cli/cmd/cpcli; \
		$(GO) build $(LDFLAGS) \
			-o $(BIN_DIR)/$(subst /,-,$(platform))-cpstart \
			./agent/cmd/cpstart; \
	)

# ========== 打包分发 ==========
DIST_VERSION ?= $(VERSION)

.PHONY: dist

dist: build-all
	@echo ">>> Packaging distribution $(DIST_VERSION)..."
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		platform_flat=$$(echo $$platform | tr / -); \
		echo ">>> Packaging $$platform_flat..."; \
		bash scripts/package.sh $(DIST_VERSION) $$platform_flat $(BIN_DIR); \
	done
	@echo ">>> Generating manifest..."
	bash scripts/gen-manifest.sh $(DIST_VERSION)
	@echo ">>> Generating installers..."
	bash scripts/gen-installers.sh $(DIST_VERSION)
	@echo ">>> Distribution ready: dist/$(DIST_VERSION)/"

# ========== 测试 ==========
test:
	cd pkg && $(GO) test -v -race -count=1 ./...
	cd scheduler && $(GO) test -v -race -count=1 ./...
	cd agent && $(GO) test -v -race -count=1 ./...
	cd cli && $(GO) test -v -race -count=1 ./...

test-coverage:
	mkdir -p coverage
	cd pkg && $(GO) test -coverprofile=../../coverage/pkg.out ./...
	cd scheduler && $(GO) test -coverprofile=../../coverage/scheduler.out ./...
	cd agent && $(GO) test -coverprofile=../../coverage/agent.out ./...
	cd cli && $(GO) test -coverprofile=../../coverage/cli.out ./...

# ========== 开发模式 ==========
run-scheduler:
	cd scheduler && $(GO) run ./cmd/scheduler --config ./configs/scheduler.yaml

run-agent:
	cd agent && $(GO) run ./cmd/agent --config ./configs/agent.yaml

dev-scheduler: run-scheduler
dev-agent: run-agent

# ========== 清理 ==========
clean:
	rm -rf $(BIN_DIR) coverage/
	cd proto && $(GO) clean
	cd pkg && $(GO) clean
	cd scheduler && $(GO) clean
	cd agent && $(GO) clean
	cd cli && $(GO) clean

# ========== 工具 ==========
tidy:
	cd proto && $(GO) mod tidy
	cd pkg && $(GO) mod tidy
	cd scheduler && $(GO) mod tidy
	cd agent && $(GO) mod tidy
	cd cli && $(GO) mod tidy