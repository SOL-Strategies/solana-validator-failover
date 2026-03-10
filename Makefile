export APP_NAME           ?= solana-validator-failover
export APP_VERSION        ?= dev
export SRC_DIR            ?= .
export BUILD_DIR          ?= $(SRC_DIR)/bin
export BUILD_OS_ARCH_LIST ?= linux-amd64 # can use linux-amd64,darwin-arm64
export CI                 ?= false

dev:
	docker compose stop
	docker compose rm --force
	APP_NAME=$(APP_NAME) APP_VERSION=$(APP_VERSION) BUILD_OS_ARCH_LIST=$(BUILD_OS_ARCH_LIST) CI=$(CI) docker compose up --build --remove-orphans dev

build-compose:
	docker compose stop
	docker compose rm --force
	touch release-notes.md
	APP_NAME=$(APP_NAME) APP_VERSION=$(APP_VERSION) BUILD_OS_ARCH_LIST=$(BUILD_OS_ARCH_LIST) CI=$(CI) docker compose up --exit-code-from build --build --remove-orphans build

test-compose:
	docker compose stop
	docker compose rm --force
	APP_NAME=$(APP_NAME) APP_VERSION=$(APP_VERSION) BUILD_OS_ARCH_LIST=$(BUILD_OS_ARCH_LIST) CI=$(CI) docker compose up --exit-code-from test --build --remove-orphans test

gh-release:
	./scripts/gh-release.sh

hot-reload:
	@echo "running with hotreload..."
	@air -c .air.conf

test:
	echo -n "${APP_VERSION}" >"pkg/constants/app.version"
	go test ./...

build:
	BUILD_DIR=$(BUILD_DIR) APP_NAME=$(APP_NAME) APP_VERSION=$(APP_VERSION) BUILD_OS_ARCH_LIST=$(BUILD_OS_ARCH_LIST) CI=$(CI) ./scripts/build.sh

# ── Demo / GIF generation ──────────────────────────────────────────────────────

demo:
	docker compose -f integration/docker-compose.demo.yml up --build -d
	@echo "mock-solana running on localhost:8899 (chicago=active)"
	@echo "run 'make demo-down' to stop"

demo-down:
	docker compose -f integration/docker-compose.demo.yml down

gif:
	@which vhs >/dev/null 2>&1 || (echo "vhs not found — install from https://github.com/charmbracelet/vhs" && exit 1)
	vhs integration/tapes/failover-passive-to-active.tape
	vhs integration/tapes/failover-active-to-passive.tape
