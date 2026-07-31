# RALPLAN-DR — Clean Architecture + DDD-Lite + Package-per-File

**Date**: 2026-07-31
**Status**: `pending approval`

---

## Principles

1. **Package-per-file**: Cada arquivo `.go` vive em seu próprio diretório junto com seu `_test.go`. Ex: `*/send_message/{send_message.go, send_message_test.go}`.
2. **Domain-first**: Entidades de domínio agrupadas por conceito DDD (aggregate root), não por tipo técnico.
3. **Use case isolation**: Cada caso de uso é um pacote independente com sua porta e implementação.
4. **Adapter isolation**: Cada adaptador de infra é um pacote independente — sem `adapter/` catch-all.
5. **Composition root delgado**: `cmd/core/main.go` + `internal/bootstrap/` — apenas wiring.

---

## Decision Drivers

1. **Organização por domínio, não por camada**: `application/usecase/send_message.go` → `application/message/send/send.go` — cada caso de uso visível pelo nome do diretório.
2. **Testes colocalizados**: `*_test.go` no mesmo pacote do arquivo, padrão Go idiomático.
3. **Reduzir arquivos planos**: Acabar com diretórios de 70+ arquivos (usecase/) e 17 arquivos soltos (bootstrap/).

---

## Viable Options

### Option A: Package-per-file completo (RECOMMENDED)

Cada arquivo `.go` vira `dir/file.go` com seu teste. ~195 diretórios.

```
├── domain/
│   ├── user/{user.go}
│   ├── session/{session.go}
│   ├── group/{group.go}
│   ├── message/{message.go}
│   ├── profile/{profile.go, profile_test.go}
│   ├── jid/{jid.go}
│   ├── webhook/{webhook.go}
│   └── ...
├── application/
│   ├── user/add/{add.go}
│   ├── user/edit/{edit.go}
│   ├── user/delete/{delete.go}
│   ├── user/list/{list.go}
│   ├── message/send_text/{send_text.go, send_text_test.go}
│   ├── message/send_image/{send_image.go}
│   ├── session/connect/{connect.go}
│   └── ...
├── port/
│   ├── client_provider/{client_provider.go}
│   ├── logger/{logger.go}
│   └── ...
├── infra/
│   ├── persistence/connection/{connection.go}
│   ├── persistence/message_history/{message_history.go}
│   ├── persistence/migrations/{migrations.go}
│   ├── whatsmeow/client_manager/{client_manager.go}
│   ├── whatsmeow/client_provider/{client_provider.go}
│   ├── whatsapp/client/{client.go, media.go, webhook.go}
│   ├── messaging/rabbitmq/{rabbitmq.go}
│   ├── messaging/webhook/{webhook_hooks.go, webhook_utils.go}
│   ├── storage/s3/{s3.go, s3_client_helper.go}
│   ├── media/opengraph/{fetch.go}
│   ├── media/sticker/{sticker.go, exif.go}
│   ├── stdio/{stdio.go}
│   ├── auth/hmac/{hmac.go}
│   ├── auth/admin/{admin.go}
│   ├── auth/authenticators/{authenticators.go}
│   └── helpers/{pure.go}
├── presentation/
│   ├── handler/send_message/{send_message.go}
│   ├── handler/send_image/{send_image.go}
│   ├── handler/send_document/{send_document.go}
│   ├── handler/session_connect/{connect.go}
│   ├── handler/group_create/{create.go}
│   ├── middleware/auth/{auth.go, middleware_test.go}
│   ├── middleware/hmac/{hmac.go}
│   ├── middleware/retry/{retry.go}
│   ├── middleware/idempotency/{idempotency.go}
│   ├── router/routes/{routes.go}
│   ├── router/registry/{registry.go}
│   └── profile/{profile_handler.go, profile_handler_test.go}
├── bootstrap/
│   ├── server/{server.go}         # main.go server startup
│   ├── config/{config.go}         # flags → config
│   ├── wiring/{wiring.go}         # initCustomHandlers + delegates
│   ├── lifecycle/{lifecycle.go}   # wmiau.go (MyClient, myEventHandler)
│   ├── webhook/{webhook.go}       # helpers.go callHook, etc.
│   └── media/{media.go}           # helpers.go OpenGraph, processMedia delegates
└── contracts/
    ├── wuzapi-v1/                  # contract definitions (existing)
    └── ...
```

**Pros**: Máximo isolamento, cada pacote tem responsabilidade única, testes colocalizados, navegação intuitiva por nome de domínio.
**Cons**: ~195 diretórios, 300+ imports ajustados, 5-8 horas de trabalho.

### Option B: Package-per-feature (moderado)

Agrupar por feature/domínio, mantendo múltiplos arquivos por diretório quando coesos.

```
├── domain/{user,session,group,message,...}/
├── application/user/{add,edit,delete,list,...}/
├── application/message/{send,download,...}/
├── infra/persistence/, infra/whatsmeow/, infra/whatsapp/...
├── presentation/handler/{session,message,group,...}/
└── bootstrap/{server,config,wiring,lifecycle,webhook}/
```

**Pros**: Menos diretórios (~60), mais coesão, redução de boilerplate de imports.
**Cons**: Arquivos ainda agrupados (não package-per-file), alguns diretórios com 5+ arquivos.

### Option C: Manter estrutura atual + organizar bootstrap/

Apenas dissolver `bootstrap/` (17 arquivos soltos) em sub-diretórios. Resto mantém como está.

**Pros**: 1-2 horas de trabalho, risco baixíssimo.
**Cons**: Não resolve os diretórios planos de 70+ arquivos em `usecase/` e `handlers/`.

---

## Recommendation: Option A

O usuário explicitamente pediu `*/name_file/{name_file.go,name_file_test.go}` — package-per-file. A estrutura resultante tem máxima navegabilidade e isolamento.

---

## Implementation Plan (5 fases, ~50 commits)

### Phase 22: Dissolve `internal/bootstrap/` → sub-dirs (commit 53)

Cada um dos 17 arquivos soltos em `bootstrap/` vira seu próprio diretório:

| Current | Target |
|---|---|
| `bootstrap/main.go` | `bootstrap/server/server.go` |
| `bootstrap/wmiau.go` | `bootstrap/lifecycle/lifecycle.go` |
| `bootstrap/helpers.go` | `bootstrap/webhook/webhook.go` |
| `bootstrap/delegates.go` | `bootstrap/wiring/wiring.go` |
| `bootstrap/custom_handlers.go` | `bootstrap/wiring/handlers.go` |
| `bootstrap/custom_routes.go` | `bootstrap/router/routes.go` |
| `bootstrap/routes.go` | `bootstrap/router/setup.go` |
| Tests → colocalizados com seus alvos |

### Phase 23: Reorganize `internal/domain/` → `internal/shared/domain/` (commit 54-60)

Cada entidade vira `domain/entity/entity.go`:

| Current | Target |
|---|---|
| `shared/domain/user.go` | `shared/domain/user/user.go` |
| `shared/domain/session.go` | `shared/domain/session/session.go` |
| `shared/domain/message.go` | `shared/domain/message/message.go` |
| ... (20 entities) → 20 dirs |

### Phase 24: Reorganize `internal/application/usecase/` → feature dirs (commit 61-80)

75 usecases viram `application/{domain}/{action}/{action}.go`:

| Current | Target |
|---|---|
| `usecase/send_message.go` | `application/message/send/send.go` |
| `usecase/send_image.go` | `application/message/send_image/send_image.go` |
| `usecase/connect.go` | `application/session/connect/connect.go` |
| `usecase/list_users.go` | `application/user/list/list.go` |
| ... (75 usecases) |

### Phase 25: Reorganize `internal/presentation/http/handlers/` (commit 81-95)

14 handlers → `presentation/handler/{name}/{name}.go`:

| Current | Target |
|---|---|
| `handlers/session_handler.go` | `handler/session/session.go` |
| `handlers/send_message_base.go` | `handler/send_message/send_message.go` |
| `handlers/user_handler.go` | `handler/user/user.go` |
| ... (14 handlers) |

### Phase 26: Reorganize `internal/infra/` + verify (commit 96-102)

- `infra/media/opengraph/fetch.go` → `infra/media/opengraph/fetch/fetch.go`
- `infra/media/sticker/{sticker,exif}.go` → cada um seu dir
- `infra/messaging/rabbitmq.go` → `infra/messaging/rabbitmq/rabbitmq.go`
- `infra/whatsapp/client/{client,media,webhook}.go` → cada um seu dir
- Verify: build + vet + test

---

## Migration strategy

Cada arquivo movido = 1 commit atômico:
1. `git mv old.go target_dir/target.go`
2. Update package declaration: `package usecase` → `package send_message`
3. Update ALL imports across codebase via `find -exec sed`
4. `go build ./cmd/core && go vet ./... && go test -count=1 ./internal/...`
5. Commit

**195 commits** se for um por um, ou **~50 commits** agrupando por feature.

---

## Pre-mortem

1. **Import hell**: Cada rename de package requer atualizar TODOS os call sites. Mitigação: script automatizado `find -exec sed` + `goimports -w`.
2. **Package naming collisions**: Pacotes com mesmo nome em diretórios diferentes (ex: `send/send.go` em `message/` e `session/`). Mitigação: alias de import sempre que necessário.
3. **Test breakage**: Testes que usam `package wuzapi` (internal bootstrap tests) precisam ser convertidos para o novo package name. Mitigação: Phase 22.5 — audit de todos os testes bootstrap.

---

## Test Plan

- **Unit**: `go test -race -count=1 ./internal/...` após cada commit
- **Contract**: `go test -run TestProfileContract ./...`
- **Layer boundary**: `grep -r "presentation/" internal/application/` deve retornar zero

---

## ADR

- **Decision**: Package-per-file com organização DDD-lite
- **Drivers**: Isolamento, navegabilidade, testes colocalizados, Clean Architecture
- **Alternatives**: Option B (~60 dirs, menos granular), Option C (só bootstrap/)
- **Why chosen**: Atende exatamente o requisito do usuário
- **Consequences**: ~195 diretórios, 300+ imports ajustados, 5-8 horas
- **Follow-ups**: Extrair myEventHandler, dissolver helpers.go
