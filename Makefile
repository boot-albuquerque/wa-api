.PHONY: build test lint lint-strict vet clean coverage coverage-gate coverage-report log-coverage-gate docker check tidy fmt stats help waclient-facade waclient-filesize waclient-test

# Default Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt
GOMOD := $(GOCMD) mod
BINARY := wa-api

# internal/wa-noise/ é o módulo de protocolo do projeto. Historicamente
# ficou fora dos gates que medem o que escrevemos (cobertura, lint, vet,
# test) por ter nascido como cópia; hoje é código mantido aqui e a inclusão
# progressiva nos gates está registrada como F17 em HOUSEKEEP.md.
COVER_PKGS := $(shell $(GOCMD) list ./... | grep -v '^wa-api/internal/wa-noise')
# vet e lint, ao contrario da cobertura, JA' incluem internal/wa-noise/ (F17).
#
# A F17 supunha que incluir o modulo quebraria o gate de lint, porque o gocyclo
# maximo dele estaria "muito acima do baseline do repo". Medido em 2026-08-07:
# a maior funcao de internal/wa-noise/ tem complexidade 46, e o baseline e' 56 —
# o gate aguenta sem afrouxar nada. E `go vet ./internal/wa-noise/...` ja' saia
# limpo (exit 0).
#
# A CONTAGEM de issues sobe (83 -> ~284), mas ela e' informativa: o que trava e'
# a complexidade maxima. Cobertura continua de fora — incluir o modulo mudaria o
# denominador e obrigaria a BAIXAR min_coverage, que e' afrouxar a catraca em
# troca de um numero maior de pacotes medidos.
ALL_PKGS := $(shell $(GOCMD) list ./...)
# pkg/infra/wa-noise/ ficava de fora de TEST_PKGS por uma data race real em
# safe_go_test.go (commit b426885). O teste foi corrigido junto da quebra do
# pacote em subpacotes: `go test -race` passa em toda a árvore, e a exclusão
# saiu — manter uma trava que não trava é pior que não ter trava.
TEST_PKGS := $(COVER_PKGS)

# Os pacotes que LANCAM BROWSER correm SERIALIZADOS (decisao 79, F103).
#
# O `go test` paraleliza PACOTES ate' ao numero de CPUs, e quatro pacotes desta
# arvore sobem Chrome. Medido durante uma execucao do gate: 24 processos de
# browser vivos ao mesmo tempo, 4 binarios de teste, numa maquina de 10 CPUs,
# com load average em 59,58. Sob essa saturacao um arranque de SPA estoura o
# prazo de 2m30 — o MESMO teste que, isolado, passa em 2,4s.
#
# E o estouro nao fica por ali: o CleanStop sinaliza, o browser recusa fechar
# (DIRTY_signal_close_refused) e o processo SOBREVIVE a execucao. Foram
# encontrados 28 orfaos, dois com mais de um dia. Cada um soma-se a carga da
# proxima corrida, entao o gate fica progressivamente mais fragil sem que nada
# no repositorio mude.
#
# `-p 1` basta, e isso foi MEDIDO em vez de suposto: os quatro pacotes tem ZERO
# chamadas a t.Parallel(), logo os testes DENTRO de cada um ja sao sequenciais e
# toda a concorrencia era entre pacotes. Uma trava de arquivo — que o
# paralelismo entre processos do `go test` exigiria — seria complexidade sem
# problema a resolver.
BROWSER_PKGS := \
	wa-api/internal/wa-headless \
	wa-api/internal/wa-headless/core \
	wa-api/internal/wa-headless/engine \
	wa-api/internal/wa-headless/runtime
SERIAL_TEST_PKGS := $(filter $(BROWSER_PKGS),$(TEST_PKGS))
PARALLEL_TEST_PKGS := $(filter-out $(BROWSER_PKGS),$(TEST_PKGS))
VET_TARGETS := $(ALL_PKGS)

# Lint
# golangci-lint espera padroes relativos ao filesystem (./pkg/x), nao paths
# de import Go (wa-api/pkg/x) como go vet/go test aceitam — por isso
# LINT_TARGETS deriva de COVER_PKGS trocando o prefixo do modulo por "./".
LINT          := golangci-lint
LINT_TARGETS  := $(shell $(GOCMD) list ./... | sed 's|^wa-api/|./|')
BASELINE_FILE := .golangci-baseline

# O linter e' COMPILADO a partir da versao fixada, com o Go DESTE repositorio,
# em vez de baixado como binario pronto.
#
# O motivo e' medido, nao preferencia. O golangci-lint carrega um go/types
# proprio, o da versao de Go com que o BINARIO foi compilado, e ele recusa
# qualquer pacote que declare um Go mais novo:
#
#   Error: can't load config: the Go language version (go1.25) used to build
#   golangci-lint is lower than the targeted Go version (1.26)
#
# Nenhuma release resolve isso: ate a v2.12.2 (a mais nova em 2026-05) declara
# `go 1.25.0` no proprio go.mod, e os binarios publicados sao compilados com
# 1.25.x. Como este modulo esta em Go 1.26 (exigencia do chromedp v0.16.0, o
# motor do internal/wa-headless/), o binario pronto nao consegue nem carregar
# o pacote — o type checker panica dentro da dependencia.
#
# GOTOOLCHAIN e' obrigatorio aqui: o go.mod do golangci-lint traz um
# `toolchain go1.25.12`, e sem sobrepo-lo o `go install` baixa 1.25.12 e
# reproduz exatamente a incompatibilidade. `go env GOVERSION` devolve o Go que
# ESTE repositorio ja' selecionou a partir do seu proprio go.mod, entao local e
# CI compilam o mesmo linter com o mesmo Go, sem repetir o numero da versao.
#
# A VERSAO do linter continua fixada: .golangci-baseline e' uma contagem
# absoluta de issues, atada a esta release. Subir GOLANGCI_VERSION e atualizar
# o baseline sao a mesma mudanca, no mesmo PR.
GOLANGCI_VERSION := v2.12.2
GOLANGCI_PKG     := github.com/golangci/golangci-lint/v2/cmd/golangci-lint

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

# test-split-check falha se a divisao perder ou soltar um pacote.
#
# $(filter …) descarta em SILENCIO o que nao casa: um typo em BROWSER_PKGS faria
# o pacote voltar ao grupo paralelo e a protecao sumiria sem nenhum aviso. Uma
# trava que pode desaparecer por engano de digitacao nao e' trava.
test-split-check:
	@serial=$$(echo $(SERIAL_TEST_PKGS) | wc -w); 	declared=$$(echo $(BROWSER_PKGS) | wc -w); 	if [ "$$serial" -ne "$$declared" ]; then 		echo "test-split: $$serial de $$declared pacotes de browser casaram com TEST_PKGS;" >&2; 		echo "  um nome em BROWSER_PKGS nao existe, e esse pacote correria em PARALELO." >&2; 		exit 1; 	fi; 	total=$$(echo $(TEST_PKGS) | wc -w); 	soma=$$(( $$(echo $(PARALLEL_TEST_PKGS) | wc -w) + serial )); 	if [ "$$total" -ne "$$soma" ]; then 		echo "test-split: $$soma pacotes na divisao contra $$total em TEST_PKGS;" >&2; 		echo "  a divisao perdeu ou duplicou pacote, e um pacote perdido nao e' testado." >&2; 		exit 1; 	fi

test: test-split-check ## Run unit tests with race detection
	$(GOTEST) -race -count=1 -timeout=20m $(PARALLEL_TEST_PKGS)
	$(GOTEST) -race -count=1 -timeout=20m -p 1 $(SERIAL_TEST_PKGS)

test-verbose: ## Run unit tests with verbose output
	$(GOTEST) -race -count=1 -v -timeout=20m $(TEST_PKGS)

coverage: ## Run tests and generate coverage report
	$(GOTEST) -race -count=1 -coverprofile=$(COVERAGE_OUT) ./...
	$(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -5

coverage-html: coverage ## Generate HTML coverage report
	$(GOCMD) tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

coverage-report: ## Cobertura por pacote com DEDUP DE BLOCOS + total que bate com go tool cover
	@$(GOTEST) -count=1 $(COVER_PKGS) -coverpkg=$(shell echo $(COVER_PKGS) | tr ' ' ',') -coverprofile=$(COVERAGE_OUT) > /dev/null
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
# O stdout vai para um ARQUIVO, nao para /dev/null, e so' e' impresso quando o
# passo FALHA (F99).
#
# O silencio existia para nao poluir o caminho feliz — e apagava exatamente a
# evidencia necessaria no caminho de falha. Custou duas investigacoes cegas em
# 2026-08-20: o alvo reprovava e o log mostrava apenas
# "make: *** [coverage-gate] Error 1", sem nome de pacote, sem linha de teste,
# sem nada. Numa delas isso me levou a escrever num commit que o gate estava
# verde tendo lido so' a AUSENCIA de linhas FAIL — ausencia que este redirect
# garantia mesmo havendo falha.
	@$(GOTEST) -count=1 $(COVER_PKGS) -coverpkg=$(shell echo $(COVER_PKGS) | tr ' ' ',') -coverprofile=$(COVERAGE_OUT) > $(COVERAGE_OUT).log 2>&1 \
	  || { echo "FALHA no passo de cobertura; saida do go test:"; cat $(COVERAGE_OUT).log; exit 1; }
	@pct=$$($(GOCMD) tool cover -func=$(COVERAGE_OUT) | tail -1 | grep -oE '[0-9]+(\.[0-9]+)?%' | tr -d '%'); \
	 if [ -z "$$pct" ]; then \
	   echo "FALHA: nao consegui extrair a cobertura total de $(COVERAGE_OUT)."; \
	   echo "       O formato de 'go tool cover -func' mudou, ou o run abortou."; \
	   echo "       Este gate FALHA FECHADO de proposito: cobertura ausente nao e' cobertura ok."; \
	   exit 1; \
	 fi; \
	 cur=$$(echo "$$pct" | LC_NUMERIC=C LC_ALL=C awk '{printf "%d", $$1*10 + 0.5}'); \
	 if [ -z "$$cur" ] || [ "$$cur" -eq 0 ]; then \
	   echo "FALHA: a conversao de '$$pct' para decimos falhou (resultado '$$cur')."; \
	   echo "       Gate FALHA FECHADO: numero ausente nao e' cobertura ok."; \
	   exit 1; \
	 fi; \
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

lint-tool: ## Compila o golangci-lint fixado com o Go deste repositorio
	GOTOOLCHAIN=$(shell $(GOCMD) env GOVERSION) $(GOCMD) install $(GOLANGCI_PKG)@$(GOLANGCI_VERSION)
	@$$($(GOCMD) env GOPATH)/bin/golangci-lint --version

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
	$(GOVET) $(VET_TARGETS)

fmt: ## Format code
	$(GOFMT) ./...

tidy: ## Tidy module dependencies
	$(GOMOD) tidy

log-coverage-gate: ## Cobertura de log (METRIC.md): advisory imprime; ratchet/floor falham em regressao (ADR-008)
	@stage=$$(grep -oE '^stage=[a-z]+' $(LOGCOV_BASELINE_FILE) | cut -d= -f2); \
	 case "$$stage" in \
	   advisory|ratchet|floor) ;; \
	   *) \
	     echo "FALHA: $(LOGCOV_BASELINE_FILE) nao declara stage=<advisory|ratchet|floor>."; \
	     echo "       Uma linha stage= ausente ou invalida NAO reverte para advisory:"; \
	     echo "       isso desarmaria as quatro travas em silencio. Gate FALHA FECHADO."; \
	     exit 1; \
	   ;; \
	 esac; \
	 echo "log-coverage: estagio do gate = $$stage (ADR-008)"; \
	 json=$$($(GOCMD) run ./cmd/logcov -json ./pkg) || { \
	   echo "FALHA: logcov nao conseguiu medir. A metrica esta' cega, o que nao e'"; \
	   echo "       o mesmo que 100%: ausencia de medicao nao vira aprovacao."; \
	   exit 1; \
	 }; \
	 func_cov=$$(echo "$$json" | grep -oE '"func_coverage_tenths": *[0-9]+' | grep -oE '[0-9]+'); \
	 errpath_cov=$$(echo "$$json" | grep -oE '"errpath_coverage_tenths": *[0-9]+' | grep -oE '[0-9]+'); \
	 eligible=$$(echo "$$json" | grep -oE '"eligible": *[0-9]+' | head -1 | grep -oE '[0-9]+'); \
	 exempt=$$(echo "$$json" | grep -oE '"exempt_annotations": *[0-9]+' | grep -oE '[0-9]+'); \
	 min_func=$$(grep -oE '^min_func_coverage=[0-9]+' $(LOGCOV_BASELINE_FILE) | grep -oE '[0-9]+'); \
	 min_errpath=$$(grep -oE '^min_errpath_coverage=[0-9]+' $(LOGCOV_BASELINE_FILE) | grep -oE '[0-9]+'); \
	 min_eligible=$$(grep -oE '^min_eligible=[0-9]+' $(LOGCOV_BASELINE_FILE) | grep -oE '[0-9]+'); \
	 max_exempt=$$(grep -oE '^max_exempt_annotations=[0-9]+' $(LOGCOV_BASELINE_FILE) | grep -oE '[0-9]+'); \
	 echo ""; \
	 echo "func_coverage    = $$func_cov decimos de % (piso $$min_func)"; \
	 echo "errpath_coverage = $$errpath_cov decimos de % (piso $$min_errpath)"; \
	 echo "eligible         = $$eligible (piso exato $$min_eligible)"; \
	 echo "exempt           = $$exempt (teto $$max_exempt)"; \
	 fail=0; \
	 if [ "$$stage" != "advisory" ]; then \
	   if [ -z "$$func_cov" ] || [ -z "$$errpath_cov" ] || [ -z "$$eligible" ] || [ -z "$$exempt" ] || \
	      [ -z "$$min_func" ] || [ -z "$$min_errpath" ] || [ -z "$$min_eligible" ] || [ -z "$$max_exempt" ]; then \
	     echo "FALHA: valor ausente no JSON ou em $(LOGCOV_BASELINE_FILE). Gate FALHA FECHADO."; \
	     fail=1; \
	   fi; \
	   if [ "$$fail" -eq 0 ] && [ "$$func_cov" -lt "$$min_func" ]; then \
	     echo "FALHA: func_coverage caiu ($$func_cov < $$min_func)."; fail=1; \
	   fi; \
	   if [ "$$fail" -eq 0 ] && [ "$$errpath_cov" -lt "$$min_errpath" ]; then \
	     echo "FALHA: errpath_coverage caiu ($$errpath_cov < $$min_errpath)."; fail=1; \
	   fi; \
	   if [ "$$fail" -eq 0 ] && [ "$$eligible" -lt "$$min_eligible" ]; then \
	     echo "FALHA: eligible caiu ($$eligible < $$min_eligible) — denominador encolheu."; fail=1; \
	   fi; \
	   if [ "$$fail" -eq 0 ] && [ "$$exempt" -gt "$$max_exempt" ]; then \
	     echo "FALHA: exempt_annotations subiu ($$exempt > $$max_exempt)."; fail=1; \
	   fi; \
	 fi; \
	 echo ""; \
	 echo "--- $(LOGCOV_BASELINE_FILE) (impresso em toda execucao, por design) ---"; \
	 grep -E '^(stage|min_|max_)' $(LOGCOV_BASELINE_FILE); \
	 echo ""; \
	 if [ "$$stage" = "advisory" ]; then \
	   echo "NOTA: estagio advisory — este alvo IMPRIME e nao falha."; \
	 elif [ "$$fail" -eq 1 ]; then \
	   echo "NOTA: estagio $$stage — regressao encontrada, gate FALHA FECHADO."; \
	   exit 1; \
	 else \
	   echo "NOTA: estagio $$stage — sem regressao."; \
	   if [ "$$func_cov" -gt "$$min_func" ]; then \
	     echo "ATENCAO: func_coverage subiu. Suba min_func_coverage para $$func_cov neste PR."; \
	   fi; \
	   if [ "$$errpath_cov" -gt "$$min_errpath" ]; then \
	     echo "ATENCAO: errpath_coverage subiu. Suba min_errpath_coverage para $$errpath_cov neste PR."; \
	   fi; \
	 fi

##@ Modulo de protocolo (internal/wa-noise/)

waclient-facade: ## Falha se algum .go fora de internal/wa-noise/ importar .../core direto em vez da fachada internal/wa-noise/main.go (Fase H etapa 6)
	@bash scripts/waclient-facade-check.sh

waclient-filesize: ## Falha se algum .go de producao de internal/wa-noise/ (exceto protocol/proto/ e binary/proto/, gerados) passar de 300 linhas (ADR-0004, Fases A/B/C)
	@bash scripts/waclient-filesize-check.sh

# internal/wa-noise/ esta fora de TEST_PKGS (ver comentario no topo e o achado
# F17 em HOUSEKEEP.md), entao um _test.go escrito la' nunca rodaria por `make
# check` — seria uma trava que nao trava. WACLIENT_TEST_PKGS lista, um a um, os
# subpacotes do fork que ja' tem teste real nosso; a lista cresce conforme as
# fases do ADR-0004 forem cobrindo o resto.
WACLIENT_TEST_PKGS := ./internal/wa-noise/core/ \
	./internal/wa-noise/protocol/msgpad/ ./internal/wa-noise/security/paircrypto/ \
	./internal/wa-noise/protocol/msgattrs/ ./internal/wa-noise/capabilities/media/ \
	./internal/wa-noise/capabilities/newsletter/ ./internal/wa-noise/capabilities/appstatesync/ \
	./internal/wa-noise/capabilities/prekeys/ ./internal/wa-noise/capabilities/pairing/ ./internal/wa-noise/capabilities/tctoken/ \
	./internal/wa-noise/capabilities/notification/ ./internal/wa-noise/capabilities/retry/ \
	./internal/wa-noise/capabilities/group/ ./internal/wa-noise/capabilities/user/ \
	./internal/wa-noise/capabilities/send/ ./internal/wa-noise/capabilities/message/ \
	./internal/wa-noise/security/handshake/ ./internal/wa-noise/runtime/keepalive/ \
	./internal/wa-noise/runtime/proxy/ \
	./internal/wa-noise/protocol/socket/ ./internal/wa-noise/protocol/appstate/ \
	./internal/wa-noise/persistence/store/ ./internal/wa-noise/persistence/store/sqlstore/ \
	./internal/wa-noise/protocol/binary/ ./internal/wa-noise/protocol/proto/ ./internal/wa-noise/protocol/types/ ./internal/wa-noise/protocol/types/events/ \
	./internal/wa-noise/security/cbc/ ./internal/wa-noise/security/gcm/ \
	./internal/wa-noise/security/hkdf/ ./internal/wa-noise/security/keys/ ./internal/wa-noise/observability/log/

waclient-test: ## Roda os testes dos subpacotes de internal/wa-noise/ ja' cobertos (ADR-0004)
	$(GOTEST) -race -count=1 $(WACLIENT_TEST_PKGS)

check: build vet test lint coverage-gate log-coverage-gate waclient-facade waclient-filesize waclient-test ## build + vet + test + lint + cobertura + cobertura de log + fachada/tamanho/testes de internal/wa-noise/

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
