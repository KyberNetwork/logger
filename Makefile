GOLANGCI_LINT ?= golangci-lint
# Keep in sync with golangci-lint-version in .github/workflows/ci.yaml.
GOLANGCI_LINT_VERSION := 2.15.0

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: lint-version ## Format the code with gofumpt
	$(GOLANGCI_LINT) fmt

.PHONY: lint
lint: lint-version ## Run the linters and check formatting, as CI does
	$(GOLANGCI_LINT) run --timeout=10m

.PHONY: test
test: ## Run the tests, as CI does
	go test -race -vet=off ./...

.PHONY: bench
bench: ## Run the benchmarks
	go test -run '^$$' -bench . -benchmem

.PHONY: lint-version
lint-version:
	@$(GOLANGCI_LINT) version --short 2>/dev/null | grep -qx '$(GOLANGCI_LINT_VERSION)' || \
		echo "warning: CI uses golangci-lint v$(GOLANGCI_LINT_VERSION), which may format and lint differently;" \
			"see https://golangci-lint.run/welcome/install/" >&2
