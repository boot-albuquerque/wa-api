.PHONY: build test lint vet clean coverage docker

# Default Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt
GOMOD := $(GOCMD) mod
BINARY := wa-api

# Lint
LINT := golangci-lint
LINT_ARGS := run ./internal/...

# Coverage output
COVERAGE_OUT := coverage.out
COVERAGE_HTML := coverage.html

# Docker
DOCKER_IMAGE := disparazaap-wa-api
DOCKER_TAG := latest

##@ Build

build: ## Build the binary
	$(GOBUILD) -o $(BINARY) ./src/cmd/core

docker: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

##@ Test

test: ## Run unit tests with race detection
	$(GOTEST) -race -count=1 ./internal/...

test-verbose: ## Run unit tests with verbose output
	$(GOTEST) -race -count=1 -v ./internal/...

coverage: ## Run tests and generate coverage report
	$(GOTEST) -race -count=1 -coverprofile=$(COVERAGE_OUT) ./internal/...
	$(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -5

coverage-html: coverage ## Generate HTML coverage report
	$(GOCMD) tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

coverage-domain: ## Show domain + application coverage
	$(GOTEST) -race -count=1 -coverprofile=$(COVERAGE_OUT) ./internal/domain/... ./internal/application/usecase/...
	$(GOCMD) tool cover -func=$(COVERAGE_OUT) | grep -E "^total:|domain|usecase"

##@ Quality

lint: ## Run golangci-lint on internal packages
	$(LINT) run ./... ./cmd/...

vet: ## Run go vet
	$(GOVET) ./...

fmt: ## Format code
	$(GOFMT) ./...

tidy: ## Tidy module dependencies
	$(GOMOD) tidy

check: build vet test lint ## Run all quality checks (build + vet + test + lint)

##@ Utilities

clean: ## Remove build artifacts
	rm -f $(BINARY) $(COVERAGE_OUT) $(COVERAGE_HTML)

stats: ## Print project statistics
	@echo "=== Root Files ==="
	@echo "root .go files: $$(ls *.go 2>/dev/null | wc -l)"
	@echo "root LOC:       $$(cat *.go 2>/dev/null | wc -l)"
	@echo "cmd/wa-api/main.go: $$(wc -l < cmd/wa-api/main.go) LOC"
	@echo ""
	@echo "=== Internal ==="
	@echo "Files: $$(find internal -name '*.go' | wc -l)"
	@echo "LOC:   $$(find internal -name '*.go' -exec cat {} + | wc -l)"
	@echo ""
	@echo "domain:      $$(find internal/domain -name '*.go' | wc -l) files"
	@echo "port:        $$(find internal/application/port -name '*.go' | wc -l) files"
	@echo "usecase:     $$(find internal/application/usecase -name '*.go' | wc -l) files"
	@echo "infra:       $$(find internal/infrastructure -name '*.go' | wc -l) files"
	@echo "handlers:    $$(find internal/interfaces/http/handlers -name '*.go' | wc -l) files"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
