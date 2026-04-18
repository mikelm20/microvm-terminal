# Built as a static Linux/amd64 binary; baked into the VM rootfs by vm-image/build-rootfs.sh.

BINARY ?= learn-guest-agent
OUT_DIR ?= ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION)"

.PHONY: all build clean

all: build

build:
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(OUT_DIR)/$(BINARY) ./cmd/$(BINARY)

clean:
	rm -rf $(OUT_DIR)
