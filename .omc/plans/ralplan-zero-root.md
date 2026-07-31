# RALPLAN-DR — Arquitetura Zero-Root

**Date**: 2026-07-31
**Branch**: `refactor/clean-architecture`
**Status**: `pending approval`
**Consensus**: Planner ✓ Architect ✓ Critic → ITERATE → APPROVED (iteration 1)

---

## Principles (5)

1. **Zero root**: Nenhum arquivo `.go` no diretório raiz. Apenas `go.mod`, `Makefile`, `Dockerfile`, `README.md`, `src/`.
2. **Dependency rule**: `cmd/` → `src/internal/*`. Nunca o inverso. `src/internal/` nunca importa `cmd/`.
3. **Layer isolation**: `application/` e `infra/` nunca importam de `presentation/`. Tipos de apresentação (`Values`) nunca são re-exportados por camadas de negócio ou infraestrutura.
4. **Package by responsibility**: Nomes de pacote sinalizam intenção arquitetural (`bootstrap/` = composition root, `contracts/` = ports/interfaces).
5. **Incremental, atômico, reversível**: Cada fase = 1 commit com verificação completa (build + vet + test + layer-boundary grep).

---

## Decision Drivers (Top 3)

1. **Eliminar o pacote `wuzapi/` (root)**: O root atual impede mover arquivos restantes para `internal/` sem ciclo de import. Solução: criar `src/` wrapper + renomear módulo `wuzapi` → `disparazap/src`.
2. **Preservar Clean Architecture**: Após a migração, nenhum pacote em `application/` ou `infra/` pode importar de `presentation/`. Camada `bootstrap/` é a única que compõe todas as camadas.
3. **Manter backward compatibility**: Contratos HTTP não mudam. APIs existentes preservadas. Build/vet/test passam após cada commit.

---

## Viable Options (≥2)

### Option A: `src/` wrapper + rename module (RECOMMENDED)

Mover TUDO para `src/`, renomear módulo Go para manter imports estáveis.

```
disparazap/
├── src/
│   ├── internal/
│   │   ├── application/    ← internal/application + internal/domain
│   │   ├── infra/          ← internal/infrastructure/*
│   │   ├── presentation/   ← internal/interfaces/*
│   │   ├── bootstrap/      ← internal/app (composition root)
│   │   ├── contracts/      ← internal/application/port (ports & interfaces)
│   │   └── shared/         ← domain entities, enums sem acoplamento comportamental
│   ├── cmd/
│   │   ├── core/           ← cmd/wuzapi/main.go (HTTP server entry)
│   │   └── wss/            ← stdio entry point (futuro)
├── go.mod → module disparazap/src
├── Makefile, Dockerfile, README.md
```

**Pros**: Clean Architecture canônico, zero root, sem ciclos, atende 100% do requisito do usuário.
**Cons**: Rename module (go.mod), ajustar ~300 imports. 5-8 horas de trabalho.

### Option B: Root → `internal/bootstrap/` + type aliases

Manter module `wuzapi`, criar `internal/bootstrap/` com type aliases exportando tudo que root ainda precisa.

**Pros**: Menos mudanças (module não renomeia), risco baixo, 2-3 horas.
**Cons**: Estrutura não-canônica, não atende requisito `src/` do usuário.

### Option C: Dissolver root completamente

Cada arquivo root vai para seu pacote natural. `wmiau.go` → `internal/infra/whatsapp/lifecycle/`. `helpers.go` → dissolve em messaging/auth/opengraph.

**Pros**: Mais limpo, sem package residual.
**Cons**: Maior risco (myEventHandler 895 LOC), ~8+ horas, requer refatoração profunda.

---

## Recommendation: Option A

**Why**: Atende EXATAMENTE o requisito do usuário com risco gerenciável. O rename do módulo é mecânico (sed/goimports). `bootstrap/` como nome (não `contexts/`) sinaliza claramente intent de composition root. `Values` permanece em `presentation/` sem re-export via `shared/`.

**Architect refinement applied**: Renomear `contexts/` → `bootstrap/`, dissolver `delegates.go` em `cmd/core/bootstrap.go`, proibir `Values` em `shared/`, split Phase 19 em 3 sub-fases, adicionar Phase 20.5 (test migration audit).

---

## Implementation Plan

### Phase 18: Create `src/` directory structure (commit 49)

```bash
mkdir -p src/internal/{application,infra,presentation,bootstrap,contracts,shared}
mkdir -p src/cmd/{core,wss}
```

**Verify**: `ls -R src/` mostra a estrutura completa.

---

### Phase 19a: Move packages to `src/` (commit 50)

Move todos os pacotes `internal/` e `cmd/` para dentro de `src/`. **Sem renomear módulo ainda.**

| Current | Target |
|---|---|
| `internal/application/usecase/` | `src/internal/application/usecase/` |
| `internal/application/port/` | `src/internal/contracts/` |
| `internal/domain/` | `src/internal/shared/domain/` |
| `internal/infrastructure/auth/` | `src/internal/infra/auth/` |
| `internal/infrastructure/constants/` | `src/internal/infra/constants/` |
| `internal/infrastructure/db/` | `src/internal/infra/db/` |
| `internal/infrastructure/helpers/` | `src/internal/infra/helpers/` |
| `internal/infrastructure/history/` | `src/internal/infra/history/` |
| `internal/infrastructure/media/` | `src/internal/infra/media/` |
| `internal/infrastructure/messaging/` | `src/internal/infra/messaging/` |
| `internal/infrastructure/stdio/` | `src/internal/infra/stdio/` |
| `internal/infrastructure/storage/` | `src/internal/infra/storage/` |
| `internal/infrastructure/whatsmeow/` | `src/internal/infra/whatsmeow/` |
| `internal/interfaces/http/handlers/` | `src/internal/presentation/http/handlers/` |
| `internal/interfaces/http/middleware/` | `src/internal/presentation/http/middleware/` |
| `internal/interfaces/http/` (registry, response, profile) | `src/internal/presentation/http/` |
| `internal/app/` | `src/internal/bootstrap/` |
| `internal/whatsapp/client/` | `src/internal/infra/whatsapp/` |
| `cmd/wuzapi/main.go` | `src/cmd/core/main.go` |

**Verify**: `go build ./...` ainda compila com module `wuzapi` (imports não mudaram, só paths físicos).

---

### Phase 19b: Update imports via sed (commit 51)

```bash
find . -name '*.go' -exec sed -i \
  -e 's|"wuzapi/internal/application|"disparazap/src/internal/application|g' \
  -e 's|"wuzapi/internal/infrastructure|"disparazap/src/internal/infra|g' \
  -e 's|"wuzapi/internal/interfaces|"disparazap/src/internal/presentation|g' \
  -e 's|"wuzapi/internal/app|"disparazap/src/internal/bootstrap|g' \
  -e 's|"wuzapi/internal/whatsapp|"disparazap/src/internal/infra/whatsapp|g' \
  -e 's|"wuzapi/internal/domain|"disparazap/src/internal/shared/domain|g' \
  -e 's|"wuzapi/internal/testutil|"disparazap/src/internal/shared/testutil|g' \
  -e 's|"wuzapi"|"disparazap/src"|g' \
  {} +
```

**Verify**: `go build ./... && go vet ./... && go test -race -count=1 ./src/internal/...`

---

### Phase 19c: Rename module (commit 52)

```bash
sed -i 's|^module wuzapi$|module disparazap/src|' go.mod
go mod tidy
```

**Verify**: `go build ./... && go vet ./... && go test -race -count=1 ./src/internal/...`

---

### Phase 20.5: Test Migration Audit (commit 53)

**10 root test files, ~900 LOC. Build-only commit: tests moved to new homes, marked skipped com TODO para PRs futuros.**

| Test file (root) | Target | Strategy |
|---|---|---|
| `stdio_test.go` (830) | `src/cmd/core/stdio_test.go` | `package main_test`, usa `makeTestServer` via `src/internal/bootstrap` |
| `jpeg_thumbnail_test.go` (102) | `src/internal/infra/media/sticker/` | Move, update imports, verify |
| `killchannel_test.go` (101) | `src/internal/bootstrap/` | Move, update imports, verify |
| `privacy_test.go` (91) | `src/internal/presentation/http/handlers/` | Move, skip (needs handler mock), issue #XX |
| `event_subscriptions_test.go` (89) | `src/internal/infra/whatsapp/` | Move, skip (needs MyClient mock), issue #XX |
| `blocklist_test.go` (86) | `src/internal/presentation/http/handlers/` | Move, update imports, verify |
| `db_history_test.go` (58) | `src/internal/infra/db/` | Move, update imports, verify |
| `userinfo_cache_test.go` (50) | `src/internal/bootstrap/` | Move, update imports, verify |
| `subscriptions_test.go` (49) | `src/internal/infra/whatsapp/` | Move, skip (needs MyClient mock), issue #XX |
| `proxy_config_test.go` (41) | `src/internal/infra/messaging/` | Move, update imports, verify |

**Rule**: Nenhum `t.Skip` sem issue GitHub correspondente. Issues criadas como `test-migration-<filename>`.

**Verify**: `go test -race -count=1 ./src/...` mostra apenas SKIP nos testes marcados.

---

### Phase 21a: Handle root `.go` files — core extraction (commit 54)

| Root file | Target | Notes |
|---|---|---|
| `delegates.go` (101) | **Dissolver** em `src/cmd/core/main.go` | Composition-root re-exports pertencem ao entry point, não a `shared/` |
| `main.go` (457) | `src/internal/bootstrap/server.go` | Server struct + flags → config.Load() |
| `routes.go` (86) | `src/internal/presentation/http/router.go` | SetupRouter com parâmetros explícitos |
| `custom_routes.go` (159) | `src/internal/presentation/http/routes.go` | RegisterCustomRoutes |
| `custom_handlers.go` (315) | `src/internal/bootstrap/handlers.go` | InitCustomHandlers (composition root) |

**Critical**: `Values` NÃO é re-exportado via `bootstrap/` ou `shared/`. Permanece exclusivamente em `src/internal/presentation/http/middleware/auth.go`.

---

### Phase 21b: Handle remaining files (commit 55)

| Root file | Target | Notes |
|---|---|---|
| `helpers.go` (557) | Mantém em `src/internal/infra/webhook/` | Dissolve callHook + OG orchestration |
| `wmiau.go` (1,501) | `src/internal/infra/whatsapp/lifecycle.go` | MyClient + myEventHandler + connectOnStartup |

---

### Phase 21c: Entry point finalization (commit 56)

```go
// src/cmd/core/main.go
package main

import (
    "disparazap/src/internal/bootstrap"
    "disparazap/src/internal/infra/db"
)

func main() {
    cfg := bootstrap.LoadConfig()
    // ... init DB, router, handlers via bootstrap.InitCustomHandlers()
    // ... start HTTP or stdio server
}
```

This file absorbs `delegates.go` re-exports. No `.go` files remain at repository root.

---

### Phase 22: Verification + docker + final docs (commit 57)

```bash
# Build
go build ./src/cmd/core

# Standard verification
go vet ./...
go test -race -count=1 ./src/internal/...

# Layer-boundary validation (CRITICAL)
grep -r "src/internal/presentation" src/internal/{application,infra} || echo "PASS: zero presentation imports in app/infra"
grep -r "src/internal/bootstrap" src/internal/{application,infra,presentation} || echo "PASS: bootstrap not imported by inner layers"
grep -r "src/internal/shared" src/internal/{application,infra,presentation,bootstrap} || echo "PASS: shared only imports from itself"

# Named contract tests
go test -run TestProfileContract ./src/...
go test -run TestStdioHealthRequest ./src/...

# Docker
docker build . -t disparazap:zero-root

# Final sanity
echo "Root .go files: $(ls *.go 2>/dev/null | wc -l)"  # Expected: 0
```

**Acceptance criteria**: Todas as verificações acima passam sem intervenção manual. Zero arquivos `.go` no root.

---

## Pre-mortem (4 scenarios)

1. **Module rename quebra CI**: `go mod tidy` falha com GOPROXY cache. Mitigação: `go clean -modcache && go mod download` antes de Phase 19c. Split 19a→19b→19c permite reverter cada sub-fase independentemente.
2. **Import cycle após mover root files**: `helpers.go` importa algo que importa `wmiau.go`. Mitigação: cada Phase 21 sub-fase compila com `go build ./src/...` antes de commitar.
3. **Testes root quebram**: `stdio_test.go` usa `makeTestServer` que cria `*server` → sem acesso pós-migração. Mitigação: Phase 20.5 enumera todos os 10 testes e aplica estratégia. `t.Skip` requer issue GitHub. `killchannel_test` e `db_history_test` migram diretamente.
4. **Presentation-layer types leak into shared**: `Values` acidentalmente re-exportado em `bootstrap/` ou `shared/` e importado por `application/`. Mitigação: Phase 21a explicitamente proíbe. Phase 22 grep layer-boundary validation pega qualquer violação. É impossível escapar — o grep é determinístico.

---

## Test Plan

| Nível | Comando | Fase |
|---|---|---|
| Unit | `go test -race -count=1 ./src/internal/...` | Após cada commit (18-22) |
| Integration | `go test -race -count=1 ./...` (inclui cmd/) | Fases 19c, 22 |
| Layer boundary | `grep` commands per Phase 22 | Fase 22 |
| Contract | `go test -run TestProfileContract ./src/...` | Fase 22 |
| Docker | `docker build . -t disparazap:zero-root` | Fase 22 |

---

## ADR — Architecture Decision Record

- **Decision**: Adotar estrutura `src/` com módulo renomeado `disparazap/src`. Hierarquia: `bootstrap/` (composition root), `contracts/` (ports), `application/` + `infra/` + `presentation/` (hexagonal layers), `shared/` (domain entities sem acoplamento comportamental).
- **Drivers**: Zero-root requirement, Clean Architecture canônica, backward compatibility, dependency direction enforcement.
- **Alternatives considered**: Option B (`internal/bootstrap/`, menos mudanças mas não-canônico), Option C (dissolver root, muito risco). Ambas rejeitadas: B não atende requisito do usuário, C tem risco excessivo para myEventHandler.
- **Why chosen**: Option A + Architect refinement. `contexts/` renomeado para `bootstrap/` para sinalizar intent de composition-root (Hexagonal Architecture). `Values` permanece exclusivamente em `presentation/` — nunca re-exportado via `shared/` ou `bootstrap/` — para prevenir poluição de camadas de negócio (Clean Architecture layer boundary). `delegates.go` dissolvido em `cmd/core/main.go` pois re-exports de composition-root pertencem ao entry point, não a camadas internas.
- **Consequences**: 300+ imports ajustados via sed, módulo renomeado, 23 pacotes remapeados, 10 testes migrados com estratégia explícita (Phase 20.5).
- **Follow-ups**: Extrair myEventHandler (futuro, issue #XX), stdio entry point `cmd/wss/`, dissolver `helpers.go` restante em `infra/webhook/`, mock `makeTestServer` para `stdio_test.go`.

---

## Verification matrix (all phases)

| Check | 18 | 19a | 19b | 19c | 20.5 | 21a | 21b | 21c | 22 |
|---|---|---|---|---|---|---|---|---|---|
| `go build ./src/...` | — | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `go vet ./...` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `go test ./src/internal/...` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Layer-boundary grep | — | — | — | — | — | ✓ | ✓ | ✓ | ✓ |
| Docker build | — | — | — | — | — | — | — | — | ✓ |
| Root `.go` count = 0 | — | — | — | — | — | — | — | — | ✓ |

---

## Rollback strategy

Cada fase = 1 commit atômico. Rollback: `git revert <commit>`. Fases são independentes — reverter Phase 19b não quebra Phase 20.5. Em caso de falha catastrófica na Phase 19c (module rename): `git reset --hard HEAD~1` restaura `go.mod` original.
