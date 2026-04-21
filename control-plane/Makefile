# control-plane: Go binary that manages Firecracker VM lifecycle + PTY bridge.

BINARY ?= learn-control-plane
OUT_DIR ?= ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION)"

.PHONY: all build test fmt vet tidy clean install

all: build

build: tidy
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(OUT_DIR)/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -count=1 ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(OUT_DIR)

# Install onto the host: build + copy to /usr/local/bin. Requires sudo.
install: build
	sudo install -m 0755 $(OUT_DIR)/$(BINARY) /usr/local/bin/$(BINARY)

# --- dev targets ---

# Run the full curl test suite against a fresh dev DB. Requires a running
# Postgres via docker-compose (see infra/docker-compose.dev.yml).
.PHONY: test-curl
test-curl:
	bash tests/run.sh

# Run `goose up` using the provided binary (if installed). Falls back to
# the built-in migration runner via a dry-start of the server.
.PHONY: migrate
migrate:
	@if command -v goose >/dev/null; then \
		goose -dir internal/db/migrations postgres "$(DATABASE_URL)" up; \
	else \
		echo "goose not installed; server migrates on startup"; \
	fi

# Regenerate sqlc typed queries. Requires sqlc to be installed.
.PHONY: sqlc
sqlc:
	sqlc generate

# Boot a real Firecracker VM end to end (kernel + rootfs + jailer + vsock
# handshake). Linux + /dev/kvm + root-ish privileges required. On macOS the
# test body skips itself; the binary still compiles cleanly. Point the
# LEARN_IT_* env vars at your kernel/rootfs/token artefacts. See
# docs/firecracker-boot.md.
.PHONY: integration-vm
integration-vm:
	go test -tags=integration_firecracker -v ./tests/integration/...
