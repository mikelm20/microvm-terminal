# control-plane: Go binary that manages Firecracker VM lifecycle + the
# terminal WebSocket.

BINARY ?= microvm-terminal
CMD ?= learn-control-plane
OUT_DIR ?= ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION)"

.PHONY: all build test fmt vet tidy clean install integration-vm

all: build

build:
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(OUT_DIR)/$(BINARY) ./cmd/$(CMD)

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

# Boot a real Firecracker VM end to end (kernel + rootfs + jailer + vsock
# handshake + resize). Linux + /dev/kvm + root required. On macOS the test
# skips itself; the binary still compiles. Point the IT_* env vars at your
# kernel/rootfs/token artefacts. See docs/firecracker-boot.md.
integration-vm:
	go test -tags=integration_firecracker -v ./tests/integration/...
