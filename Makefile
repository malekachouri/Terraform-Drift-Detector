.PHONY: help up down logs build test tf-init tf-apply tf-plan tf-destroy drift scan scan-json server-scan demo clean

# Terraform runs in Docker by default; use your own with: make tf-apply TF=terraform
TF ?= docker compose run --rm terraform
DRIFTCTL ?= ./bin/driftctl
CONFIG ?= configs/driftctl.yaml

help: ## Show targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

up: ## Start LocalStack + drift-server (dashboard on :8080)
	docker compose up -d --build --wait

down: ## Stop everything and delete LocalStack data
	docker compose down -v

logs: ## Follow container logs
	docker compose logs -f

build: ## Build driftctl and drift-server into ./bin
	go build -o bin/driftctl ./cmd/driftctl
	go build -o bin/drift-server ./cmd/drift-server

test: ## Run unit tests
	go test ./...

tf-init: ## terraform init (state backend = S3 in LocalStack)
	$(TF) init -input=false

tf-apply: tf-init ## Create the demo infrastructure in LocalStack
	$(TF) apply -auto-approve -input=false

tf-plan: ## What Terraform itself thinks changed
	$(TF) plan -input=false

tf-destroy: ## Destroy the demo infrastructure
	$(TF) destroy -auto-approve -input=false

drift: ## Make out-of-band changes (simulate console clicks)
	./scripts/simulate-drift.sh all

scan: build ## Scan for drift from the CLI (exit 1 = drift)
	$(DRIFTCTL) scan --config $(CONFIG) --workspace localstack

scan-json: build ## Same, JSON output
	$(DRIFTCTL) scan --config $(CONFIG) --workspace localstack --output json

server-scan: ## Trigger a scan through the drift-server REST API
	@id=$$(curl -fsS localhost:8080/api/v1/workspaces | sed -E 's/.*"id":"([^"]+)".*/\1/'); \
	curl -fsS -X POST localhost:8080/api/v1/workspaces/$$id/scans | head -c 600; echo

demo: ## Full walkthrough: up -> apply -> scan -> drift -> scan
	./scripts/demo.sh

clean: ## Remove build output and local DB
	rm -rf bin/ driftctl.db driftctl.db-*
