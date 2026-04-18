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
