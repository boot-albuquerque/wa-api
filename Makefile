.PHONY: build test lint lint-strict vet clean coverage coverage-gate coverage-report log-coverage-report docker check tidy fmt stats help

# Default Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt
GOMOD := $(GOCMD) mod
BINARY := wa-api

# Lint
LINT          := golangci-lint
LINT_TARGETS  := ./...
BASELINE_FILE := .golangci-baseline

# Coverage ratchet
COVERAGE_BASELINE_FILE := .coverage-baseline

# Log coverage ratchet (Fase 9). Estagio advisory: imprime, nao trava.
LOGCOV_BASELINE_FILE := .log-coverage-baseline

# Coverage output
COVERAGE_OUT := coverage.out
COVERAGE_HTML := coverage.html

# Docker
DOCKER_IMAGE := disparazaap-wa-api
DOCKER_TAG := latest

##@ Build

build: ## Build the binary
	$(GOBUILD) -o $(BINARY) ./cmd/core

docker: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

##@ Test

test: ## Run unit tests with race detection
	$(GOTEST) -race -count=1 ./...

test-verbose: ## Run unit tests with verbose output
	$(GOTEST) -race -count=1 -v ./...

coverage: ## Run tests and generate coverage report
	$(GOTEST) -race -count=1 -coverprofile=$(COVERAGE_OUT) ./...
	$(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -5

coverage-html: coverage ## Generate HTML coverage report
	$(GOCMD) tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

coverage-report: ## Cobertura por pacote com DEDUP DE BLOCOS + total que bate com go tool cover
	@$(GOTEST) -count=1 ./... -coverpkg=./... -coverprofile=$(COVERAGE_OUT) > /dev/null
	@$(GOCMD) run ./cmd/logcov -coverprofile=$(COVERAGE_OUT)
	@echo ""
	@echo "NOTA: o total acima usa deduplicacao de blocos por chave arquivo:range,"
	@echo "      retendo max(count). Somar as linhas cruas de um perfil gerado com"
	@echo "      -coverpkg=./... infla o denominador ~13x, porque o mesmo bloco"
	@echo "      aparece uma vez por pacote de teste que o executou. O total bate"
	@echo "      com 'go tool cover -func=$(COVERAGE_OUT) | tail -1' por construcao,"
	@echo "      e ha teste provando isso em cmd/logcov/main_test.go."
	@echo ""
	@echo "      A comparacao e' do PERCENTUAL, nao dos bytes da linha: go tool"
	@echo "      cover usa tabwriter sobre as colunas da propria listagem por"
	@echo "      funcao, entao a quantidade de TABs de padding difere. Confira com:"
	@echo "        diff <(make coverage-report | grep -oE '[0-9.]+%$$' | tail -1) \\\\"
	@echo "             <($(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -1 | grep -oE '[0-9.]+%$$')"

coverage-domain: ## Show domain + application coverage
	$(GOTEST) -race -count=1 -coverprofile=$(COVERAGE_OUT) ./pkg/domain/... ./pkg/application/usecase/...
	$(GOCMD) tool cover -func=$(COVERAGE_OUT) | grep -E "^total:|domain|usecase"

coverage-gate: ## Cobertura contra o piso declarado: falha se o numero CAIR
	@$(GOTEST) -count=1 ./... -coverpkg=./... -coverprofile=$(COVERAGE_OUT) > /dev/null
	@pct=$$($(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -1 | grep -oE '[0-9]+(\.[0-9]+)?%' | tr -d '%'); \
	 if [ -z "$$pct" ]; then \
	   echo "FALHA: nao consegui extrair a cobertura total de $(COVERAGE_OUT)."; \
	   echo "       O formato de 'go tool cover -func' mudou, ou o run abortou."; \
	   echo "       Este gate FALHA FECHADO de proposito: cobertura ausente nao e' cobertura ok."; \
	   exit 1; \
	 fi; \
	 cur=$$(echo "$$pct" | awk '{printf "%d", $$1*10 + 0.5}'); \
	 base=$$(grep -oE '^min_coverage=[0-9]+' $(COVERAGE_BASELINE_FILE) | grep -oE '[0-9]+'); \
	 if [ -z "$$base" ]; then \
	   echo "FALHA: $(COVERAGE_BASELINE_FILE) nao declara min_coverage=<N>. Gate FALHA FECHADO."; \
	   exit 1; \
	 fi; \
	 echo "coverage: $$cur decimos de % (piso declarado $$base decimos de %) — atual $$pct%"; \
	 if [ "$$cur" -lt "$$base" ]; then \
	   echo "FALHA: a cobertura caiu ($$pct% < piso declarado)."; \
	   echo "       Codigo novo sem teste, ou teste deletado. Cubra o codigo novo,"; \
	   echo "       ou justifique e ajuste min_coverage no PR."; \
	   exit 1; \
	 fi; \
	 if [ "$$cur" -gt "$$base" ]; then \
	   echo "ATENCAO: a cobertura subiu. Suba min_coverage para $$cur neste mesmo PR."; \
	 fi

##@ Quality

lint: ## Lint contra o baseline declarado: falha se o numero SUBIR
	@$(LINT) run --issues-exit-code 0 $(LINT_TARGETS) 2>&1 | tee .lint.out
	@found=$$(grep -oE '^[0-9]+ issues' .lint.out | grep -oE '^[0-9]+' | tail -1); \
	 if [ -z "$$found" ]; then \
	   echo "FALHA: nao consegui extrair a contagem de issues de .lint.out."; \
	   echo "       O formato do sumario do golangci-lint mudou, ou o run abortou."; \
	   echo "       Este gate FALHA FECHADO de proposito: contagem ausente nao e' zero."; \
	   exit 1; \
	 fi; \
	 gocyclo_lines=$$(grep -c 'cyclomatic complexity .* is high' .lint.out || true); \
	 if [ "$$gocyclo_lines" -eq 0 ]; then \
	   max=0; \
	 else \
	   max=$$(grep -oE 'cyclomatic complexity [0-9]+ of' .lint.out | grep -oE '[0-9]+' | sort -rn | head -1); \
	 fi; \
	 if [ -z "$$max" ]; then \
	   echo "FALHA: ha $$gocyclo_lines linha(s) de gocyclo em .lint.out mas nao consegui ler nenhuma complexidade."; \
	   echo "       O formato da mensagem do gocyclo mudou. Gate FALHA FECHADO."; \
	   exit 1; \
	 fi; \
	 base_max=$$(grep -oE '^max_complexity=[0-9]+' $(BASELINE_FILE) | grep -oE '[0-9]+'); \
	 base_count=$$(grep -oE '^count=[0-9]+' $(BASELINE_FILE) | grep -oE '[0-9]+'); \
	 if [ -z "$$base_max" ]; then \
	   echo "FALHA: $(BASELINE_FILE) nao declara max_complexity=<N>. Gate FALHA FECHADO."; \
	   exit 1; \
	 fi; \
	 echo "lint: complexidade maxima $$max (baseline $$base_max) | $$found issue(s) (informativo, baseline $$base_count)"; \
	 if [ "$$max" -gt "$$base_max" ]; then \
	   echo "FALHA: a maior funcao do repo piorou ($$max > $$base_max)."; \
	   echo "       A trava e' a complexidade maxima, nao a contagem: decompor uma funcao"; \
	   echo "       gigante em varias menores AUMENTA a contagem e MELHORA o repo."; \
	   echo "       Quebre a funcao, ou justifique e ajuste max_complexity no PR."; \
	   exit 1; \
	 fi; \
	 if [ "$$max" -lt "$$base_max" ]; then \
	   echo "ATENCAO: a complexidade maxima caiu ($$max < $$base_max). Baixe max_complexity para $$max neste mesmo PR."; \
	 fi; \
	 if [ "$$found" -ne "$$base_count" ]; then \
	   echo "NOTA: a contagem de issues mudou ($$base_count -> $$found). Informativo, nao trava. Atualize count no PR."; \
	 fi

lint-strict: ## Lint com tolerancia zero — vira o alvo `lint` quando max_complexity chegar a 10
	$(LINT) run $(LINT_TARGETS)

vet: ## Run go vet
	$(GOVET) ./...

fmt: ## Format code
	$(GOFMT) ./...

tidy: ## Tidy module dependencies
	$(GOMOD) tidy

log-coverage-report: ## Cobertura de log (METRIC.md): IMPRIME os numeros, nao falha (stage=advisory)
	@stage=$$(grep -oE '^stage=[a-z]+' $(LOGCOV_BASELINE_FILE) | cut -d= -f2); \
	 if [ -z "$$stage" ]; then \
	   echo "ATENCAO: $(LOGCOV_BASELINE_FILE) nao declara stage=<advisory|ratchet|floor>."; \
	   echo "         Seguindo como advisory; este alvo nao trava nesta fase."; \
	   stage=advisory; \
	 fi; \
	 echo "log-coverage: estagio do gate = $$stage (ADR-008)"; \
	 $(GOCMD) run ./cmd/logcov ./pkg || { \
	   echo "ATENCAO: logcov nao conseguiu medir. Estagio advisory: nao trava,"; \
	   echo "         mas a metrica esta' cega e isso precisa ser resolvido."; \
	   exit 0; \
	 }; \
	 echo ""; \
	 echo "--- $(LOGCOV_BASELINE_FILE) (impresso em toda execucao, por design) ---"; \
	 grep -E '^(stage|min_|max_)' $(LOGCOV_BASELINE_FILE); \
	 echo ""; \
	 echo "NOTA: estagio advisory — este alvo IMPRIME e nao falha. A trava entra"; \
	 echo "      quando stage virar ratchet, nas fases F10-F15."

check: build vet test lint coverage-gate log-coverage-report ## build + vet + test + lint + cobertura + cobertura de log

##@ Utilities

clean: ## Remove build artifacts
	rm -f $(BINARY) $(COVERAGE_OUT) $(COVERAGE_HTML) .lint.out

stats: ## Print project statistics
	@echo "=== Root Files ==="
	@echo "root .go files: $$(ls *.go 2>/dev/null | wc -l)"
	@echo "root LOC:       $$(cat *.go 2>/dev/null | wc -l)"
	@echo ""
	@echo "=== pkg ==="
	@echo "Files: $$(find pkg -name '*.go' | wc -l)"
	@echo "LOC:   $$(find pkg -name '*.go' -exec cat {} + | wc -l)"
	@echo ""
	@echo "domain:      $$(find pkg/domain -name '*.go' | wc -l) files"
	@echo "port:        $$(find pkg/application/port -name '*.go' | wc -l) files"
	@echo "usecase:     $$(find pkg/application/usecase -name '*.go' | wc -l) files"
	@echo "infra:       $$(find pkg/infra -name '*.go' | wc -l) files"
	@echo "handlers:    $$(find pkg/presentation/http/handlers -name '*.go' | wc -l) files"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
