# learn-platform root Makefile. Thin wrappers around the dev stack.
# Agent-Tooling owns this file. Other agents read, do not write.

SHELL := /bin/bash

COMPOSE := docker compose --env-file .env.local -f infra/docker-compose.dev.yml
PSQL_URL ?= $$LEARN_DATABASE_URL

.PHONY: help
help:
	@echo "Targets:"
	@echo "  dev-up        Bring up Postgres (and optional services) in the background"
	@echo "  dev-down      Stop and remove the dev stack"
	@echo "  dev-restart   Restart the dev stack"
	@echo "  dev-logs      Follow logs for the dev stack"
	@echo "  dev-ps        Show running dev services"
	@echo "  psql          Open a psql shell against LEARN_DATABASE_URL"
	@echo "  codegen       Regenerate shared codegen (voice types + Go apitypes)"
	@echo "  typecheck     pnpm typecheck across workspaces"
	@echo "  lint          pnpm lint across workspaces"
	@echo "  build         pnpm build across workspaces"
	@echo "  go-build      go build ./... across Go modules"
	@echo "  integration-vm  Boot one real Firecracker VM end-to-end (Linux + KVM only)"

.env.local:
	@test -f .env.local || (cp .env.local.example .env.local && echo "Created .env.local from example. Edit as needed.")

.PHONY: dev-up
dev-up: .env.local
	$(COMPOSE) up -d

.PHONY: dev-down
dev-down:
	$(COMPOSE) down

.PHONY: dev-restart
dev-restart:
	$(COMPOSE) restart

.PHONY: dev-logs
dev-logs:
	$(COMPOSE) logs -f

.PHONY: dev-ps
dev-ps:
	$(COMPOSE) ps

.PHONY: psql
psql: .env.local
	@set -a; source .env.local; set +a; psql "$(PSQL_URL)"

.PHONY: codegen
codegen:
	pnpm codegen

.PHONY: typecheck
typecheck:
	pnpm typecheck

.PHONY: lint
lint:
	pnpm lint

.PHONY: build
build:
	pnpm build

.PHONY: go-build
go-build:
	cd control-plane && go build ./...
	cd guest-agent && go build ./...
	@if [ -d vm-image/claude-wrap ]; then cd vm-image/claude-wrap && go build ./...; fi
	@if [ -d proxy ]; then cd proxy && go build ./...; fi

# Real Firecracker VM end-to-end boot test. Linux + /dev/kvm + root required.
# See docs/firecracker-boot.md for the LEARN_IT_* env vars and artefact
# prerequisites. Skips with a clear message on macOS.
.PHONY: integration-vm
integration-vm:
	cd control-plane && go test -tags=integration_firecracker -v ./tests/integration/...
