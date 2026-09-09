# microvm-terminal root Makefile. Thin wrappers around the dev stack and the
# three Go modules.

SHELL := /bin/bash

COMPOSE := docker compose --env-file .env.local -f infra/docker-compose.dev.yml
PSQL_URL ?= $$DATABASE_URL

.PHONY: help
help:
	@echo "Targets:"
	@echo "  dev-up        Bring up Postgres in the background"
	@echo "  dev-down      Stop and remove the dev stack"
	@echo "  dev-logs      Follow logs for the dev stack"
	@echo "  psql          Open a psql shell against DATABASE_URL"
	@echo "  build         go build ./... across Go modules"
	@echo "  test          go vet + go test across Go modules"
	@echo "  run-mock      Run the control plane with the mock launcher (no KVM needed)"
	@echo "  integration-vm  Boot one real Firecracker VM end-to-end (Linux + KVM only)"

.env.local:
	@test -f .env.local || (cp .env.local.example .env.local && echo "Created .env.local from example. Edit as needed.")

.PHONY: dev-up dev-down dev-logs psql
dev-up: .env.local
	$(COMPOSE) up -d

dev-down:
	$(COMPOSE) down

dev-logs:
	$(COMPOSE) logs -f

psql: .env.local
	@set -a; source .env.local; set +a; psql "$(PSQL_URL)"

.PHONY: build
build:
	cd control-plane && go build ./...
	$(MAKE) -C guest-agent build
	cd proxy && go build ./...

.PHONY: test
test:
	cd control-plane && go vet ./... && go test ./...
	cd guest-agent && GOOS=linux GOARCH=amd64 go vet ./... && go test ./...
	cd proxy && go vet ./... && go test ./...

# Local run without a hypervisor: sessions are in-memory stubs with no
# console, but login, the pages and the sessions API all work.
.PHONY: run-mock
run-mock: .env.local
	@set -a; source .env.local; set +a; \
	  mkdir -p .dev && test -s .dev/password || echo dev-password-1 > .dev/password; \
	  cd control-plane && USE_MOCK_LAUNCHER=true SECURE_COOKIES=false \
	    PASSWORD_FILE=../.dev/password COOKIE_SECRET_FILE=../.dev/cookie-secret \
	    go run ./cmd/microvm-terminal -config /dev/null

# Real Firecracker VM end-to-end boot test. Linux + /dev/kvm + root required.
# See docs/firecracker-boot.md for the IT_* env vars.
.PHONY: integration-vm
integration-vm:
	cd control-plane && go test -tags=integration_firecracker -v ./tests/integration/...
