# RALPLAN-DR — Package-per-File com restrições Go

**Date**: 2026-07-31
**Status**: `pending approval`

---

## Principles

1. `*/name/{name.go, name_test.go}` — cada arquivo independente ganha seu próprio pacote.
2. **Respeitar Go**: arquivos que compartilham receiver methods (`func (c *MyClient)`) devem permanecer no mesmo pacote.
3. **Cada pacote = 1 responsabilidade**: onde Go permite, um arquivo = um pacote.
4. **Incremental, atômico**: cada domínio em 1 commit.

---

## Onde aplicar × onde NÃO aplicar

| Pacote | Arquivos | Aplica? | Razão |
|---|---|---|---|
| `application/usecase/` | 75 | ✅ SIM | Cada usecase é independente (1 constructor por arquivo) |
| `application/contracts/` | 9 | ✅ SIM | Cada contrato define 1 interface |
| `presentation/handlers/` | 14 | ✅ SIM | Cada handler é independente |
| `presentation/http/` | 7 | ✅ SIM | Profile, registry, response — cada um independente |
| `presentation/middleware/` | 6 | ⚠️ Parcial | auth.go + middleware_test.go ficam juntos |
| `domain/` | 22 | ❌ NÃO | Entidades compartilham tipos entre si, testados juntos |
| `bootstrap/` | 20 | ❌ NÃO | 5 receivers (`*MyClient`, `*server`, `*KillChannel`, `*ClientManagerAdapterImpl`, `*testRequest`), globais compartilhados |
| `infra/whatsapp/client/` | 3 | ❌ NÃO | `*MyClient` receiver em 3 arquivos |
| `infra/whatsmeow/` | 11 | ⚠️ Parcial | `*ClientManager`, `*ClientProviderAdapter`, `*ZerologAdapter`, `*ProfileDataAccess`, `*HealthClientProviderAdapter` — 5 receivers em arquivos diferentes |
| `infra/auth/` | 5 | ✅ SIM | Cada arquivo independente |
| `infra/media/sticker/` | 2 | ✅ SIM | sticker.go + exif.go independentes |
| `infra/messaging/` | 4 | ✅ SIM | rabbitmq, webhook_hooks, webhook_utils, hmac_utils |
| `infra/db/` | 3 | ⚠️ Parcial | connection.go (standalone), message_history.go + migrations.go (compartilham db) |
| `infra/storage/` | 2 | ✅ SIM | s3.go + s3_client_helper.go |

---

## Plano (4 fases, ~90 commits)

### Phase 1: `application/usecase/` → per-file (75 dirs)

Cada arquivo vira `application/usecase/name/name.go`:

```
application/usecase/
├── add_user/{add_user.go}
├── archive_chat/{archive_chat.go}
├── block_user/{block_user.go, block_user_test.go}
├── ...
└── validation_test/{validation_test.go}
```

Package name = directory name. Cada package exporta 1 constructor `NewXxx`.

**Desafio**: handlers importam `usecase.XxxUseCase`. Com per-file, viram `add_user.AddUserUseCase`, etc. 100+ imports ajustados.

### Phase 2: `application/contracts/` → per-file (9 dirs)

```
application/contracts/
├── client_provider/{client_provider.go}
├── logger/{logger.go}
└── ...
```

### Phase 3: `presentation/` → per-file (27 dirs)

Handlers + middleware + http root files.

### Phase 4: `infra/` packages seguros (15 dirs)

auth/, messaging/, storage/, sticker/, db/connection.go, helpers/, history/, opengraph/.

---

## ADR

- **Decision**: Package-per-file onde Go permite (arquivos sem receiver coupling). ~110 diretórios criados.
- **Drivers**: Requisito explícito do usuário, navegabilidade, isolamento de responsabilidade.
- **Why not 100%**: Go proíbe methods on types from other packages. `bootstrap/`, `whatsapp/client/`, `whatsmeow/`, `domain/` permanecem multi-file.
- **Consequences**: ~110 dirs, 500+ imports ajustados, 4-6 horas de trabalho.
