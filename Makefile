# Tools are pinned in tools.mod and built on first use.
GOTOOL := go tool -modfile=tools.mod

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# gofumpt, then golines: the order golangci-lint checks them in.
.PHONY: fmt
fmt: ## Format the code with gofumpt and golines
	$(GOTOOL) gofumpt -w .
	$(GOTOOL) golines -w .

.PHONY: lint
lint: ## Run the linters and check formatting, as CI does
	$(GOTOOL) golangci-lint run --timeout=10m

.PHONY: test
test: ## Run the tests, as CI does
	go test -race -vet=off ./...

.PHONY: bench
bench: ## Run the benchmarks
	go test -run '^$$' -bench . -benchmem
