export APP_NAME           ?= solana-validator-failover
export APP_VERSION        ?= dev
export SRC_DIR            ?= .
export BUILD_DIR          ?= $(SRC_DIR)/bin
export BUILD_OS_ARCH_LIST ?= $(shell $(GO) env GOHOSTOS)-$(shell $(GO) env GOHOSTARCH)
export CI                 ?= false
export GO                 ?= go

.DEFAULT_GOAL := build
.PHONY: build build-all smoke-test test prepare-version check-go-version dev hot-reload gh-release demo demo-down gif

dev:
	docker compose stop
	docker compose rm --force
	APP_NAME=$(APP_NAME) APP_VERSION=$(APP_VERSION) BUILD_OS_ARCH_LIST=$(BUILD_OS_ARCH_LIST) CI=$(CI) docker compose up --build --remove-orphans dev

gh-release:
	./scripts/gh-release.sh

hot-reload:
	@echo "running with hotreload..."
	@air -c .air.conf

check-go-version:
	@GO="$(GO)" sh scripts/check-go-version.sh

prepare-version: check-go-version
	@printf '%s' "$(APP_VERSION)" > pkg/constants/app.version

test: prepare-version
	$(GO) test -race ./...

build:
	BUILD_DIR="$(BUILD_DIR)" APP_NAME="$(APP_NAME)" APP_VERSION="$(APP_VERSION)" BUILD_OS_ARCH_LIST="$(BUILD_OS_ARCH_LIST)" PACKAGE=false ./scripts/build.sh

build-all:
	BUILD_DIR="$(BUILD_DIR)" APP_NAME="$(APP_NAME)" APP_VERSION="$(APP_VERSION)" BUILD_OS_ARCH_LIST="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64" PACKAGE=true ./scripts/build.sh

smoke-test: prepare-version
	@mkdir -p "$(BUILD_DIR)/.smoke"
	CGO_ENABLED=0 $(GO) build -mod=readonly -trimpath -ldflags='-s -w' -o "$(BUILD_DIR)/.smoke/$(APP_NAME)" .
	@"$(BUILD_DIR)/.smoke/$(APP_NAME)" --version | grep -F -- "version $(APP_VERSION)"
	@"$(BUILD_DIR)/.smoke/$(APP_NAME)" --help >/dev/null

# ── Demo / GIF generation ──────────────────────────────────────────────────────

demo:
	go run ./integration/prepare-history
	docker compose -f integration/docker-compose.demo.yml up --build -d
	@echo "mock-solana running on localhost:8899 (chicago=active)"
	@echo "run 'make demo-down' to stop"

demo-down:
	docker compose -f integration/docker-compose.demo.yml down

gif:
	@which vhs >/dev/null 2>&1 || (echo "vhs not found — install from https://github.com/charmbracelet/vhs" && exit 1)
	vhs integration/tapes/failover-passive-to-active.tape
	vhs integration/tapes/failover-active-to-passive.tape
