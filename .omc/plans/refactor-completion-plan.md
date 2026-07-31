# Refactor Completion Plan — disparazaap-wuzapi

**Branch**: `refactor/clean-architecture`
**Base commit**: `9406815` (refactor(s3): delete root s3manager.go)
**Date**: 2026-07-30
**Author**: ralplan (auto-generated)
**Goal**: Complete the Clean Architecture migration so root contains only `cmd/wuzapi/main.go`.

---

## Summary

Migration is 60% complete. Fases 1–9 delivered 22 atomic commits, cut root LOC from 15,671 → 8,092, moved all 89 routes to internal handlers, and eliminated legacy `s.*()` method dispatch. What remains is the structural work Go's `package main` restriction blocked earlier: converting root into a library package so `internal/` can import its remaining types (`MyClient`, `Values`, `server`, `clientManager`), then moving the last 6,934 LOC across `wmiau.go`, `helpers.go`, `stdio.go`, `rabbitmq.go`, `db.go`, `auth.go`, and 15 other root files into their respective internal packages.

The plan is 5 phases (10–14), each a single atomic commit gated by `make build && make vet && make test && make lint && docker build`. Every phase is independently revertible.

## Success criteria (end-state)

- [ ] Root directory contains **only** `cmd/wuzapi/main.go` (plus tests, docs, deploy, static, Makefile, go.mod, go.sum).
- [ ] `package main` appears **exactly once** — in `cmd/wuzapi`.
- [ ] Zero circular imports; `internal/**` never imports `cmd/**`.
- [ ] All existing tests pass unchanged (contract tests, unit tests, integration tests).
- [ ] Docker image builds and runs identically to `9406815`.
- [ ] Root LOC ≤ 200 (down from 8,092).

## Non-goals

- Rewriting `myEventHandler` logic — only relocate and inject dependencies.
- Adding features, changing HTTP contracts, or altering database schema.
- Refactoring test files (they stay next to their targets or migrate mechanically).
- Renaming the module (`wuzapi/`) — keeps import paths stable.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Circular imports between renamed root pkg and `internal/` | Introduce adapter interfaces in `internal/application/port/` before moving impl |
| Breaking `main.db` / `users.db` filesystem layout | Preserve `os.Executable()` path resolution in `cmd/wuzapi/main.go` |
| Stdio JSON-RPC regression | Run `stdio_test.go` before + after each phase; snapshot request/response |
| Test file location breaks (`_test.go` in wrong pkg) | Move tests alongside implementation in same commit |
| `alice.Chain` middleware wiring breaks after router extraction | Preserve exact middleware order captured in `custom_routes.go:24` |

## Rollback strategy

Each phase = 1 commit. Rollback: `git revert <commit>`. Because phases are strictly additive-then-subtractive (add new package, then delete root file), reverting a phase never breaks a later phase's state — the later phase can be re-applied on top of the revert once the intermediate issue is fixed.

---

## Phase 10 — Root → `package wuzapi` + `cmd/wuzapi/main.go`

**Why now**: Go forbids importing `package main`. This unblocks Phases 11–14. Without it, `internal/whatsapp/client/` cannot reference `MyClient` and every subsequent extraction requires adapter shims.

**Steps**
1. Create `cmd/wuzapi/main.go` — move `func main()`, `startHTTPMode`, `startStdioMode` here; import `wuzapi` as `pkg "wuzapi"`.
2. Rename all root `.go` files from `package main` → `package wuzapi`.
3. Export the currently-lowercase identifiers used across files (`server`→`Server`, `clientManager`→`ClientManager` var, `appCtx`→`AppCtx`, `container`→`Container`, `userinfocache`→`UserInfoCache`, `lastMessageCache`→`LastMessageCache`, `globalHTTPClient`→`GlobalHTTPClient`, `privateIPBlocks`→`PrivateIPBlocks`). Keep test-file identifiers.
4. Update `Makefile`: `BINARY := wuzapi` → build target `./cmd/wuzapi`.
5. Update `Dockerfile` build path: `RUN go build -o wuzapi ./cmd/wuzapi`.
6. Update `.omc/plans/root-loc-baseline.txt` to record new baseline.

**Files touched**: all 24 root `.go` files (mechanical rename), `Makefile`, `Dockerfile`, new `cmd/wuzapi/main.go`.

**Verify**:
```bash
go build ./cmd/wuzapi
go vet ./...
go test -race -count=1 ./internal/... ./...
docker build . -t wuzapi:phase10
```

**Rollback**: `git revert HEAD` restores `package main` root.

**Estimated blast radius**: 24 files renamed, ~120 identifier updates (grep-driven `sed`).

---

## Phase 11 — Extract `MyClient` + event handler → `internal/whatsapp/client/`

**Why**: `wmiau.go` is 1,531 LOC and holds the largest single-file complexity. `myEventHandler` (895 LOC) dispatches every WhatsApp event to webhooks, message history, and media processing. Extraction requires Phase 10.

**Target layout**
```
internal/whatsapp/client/
├── myclient.go             # MyClient struct + AddEventHandler wiring
├── eventhandler.go         # myEventHandler dispatch (was wmiau.go:636-1531)
├── startup.go              # connectOnStartup + startClient
├── subscriptions.go        # updateAndGetUserSubscriptions, checkIfSubscribedToEvent
└── event_router.go         # per-event-type routing (Message, Receipt, Presence, etc.)
```

**Steps**
1. Introduce `internal/application/port/ClientLifecycle` interface (Start, Connect, Disconnect, RegisterEventHandler).
2. Extract `MyClient` struct + methods to `internal/whatsapp/client/myclient.go`.
3. Move `myEventHandler` — inject `webhookDispatcher`, `messageHistoryStore`, `mediaProcessor` via constructor (these already exist in `internal/`).
4. Move `connectOnStartup` (was `wmiau.go:249`) and `startClient` (was `wmiau.go:339`) — server receiver removed, take DB + config as constructor deps.
5. Update `cmd/wuzapi/main.go` to instantiate `client.Manager` and call `.ConnectOnStartup()` instead of `s.connectOnStartup()`.
6. Delete `wmiau.go` from root (moved wmiau_test.go — none exists, so no test move).

**Files touched**: create 5 files under `internal/whatsapp/client/`, delete `wmiau.go`, edit `cmd/wuzapi/main.go`.

**Verify**:
```bash
go build ./cmd/wuzapi
go test -race ./internal/...
# Smoke: docker run + connect one user + send one message + verify webhook fires
```

**Rollback**: `git revert HEAD` restores root `wmiau.go`.

**Estimated blast radius**: 1,531 LOC moved, ~40 refs to `clientManager` become injected `cm ClientManager`.

---

## Phase 12 — Split `helpers.go` (1,084 LOC) by concern

**Why**: `helpers.go` mixes webhook dispatch, Open Graph fetching, sticker processing, and HMAC crypto. Each has a natural home in `internal/infrastructure/`.

**Target layout**
```
internal/infrastructure/messaging/
├── webhook_dispatch.go     # callHook, callHookWithHmac (helpers.go:291-444)
└── webhook_file.go         # callHookFile, callHookFileWithHmac (helpers.go:447-567)

internal/infrastructure/media/opengraph/
├── fetcher.go              # getOpenGraphData, fetchOpenGraphData (helpers.go:214-269, 689-740)
├── image.go                # fetchOpenGraphImage, encodeJPEGThumbnail (helpers.go:742-801)
└── semaphore.go            # UserSemaphoreManager (helpers.go:127-139)

internal/infrastructure/media/sticker/
├── converter.go            # convertVideoStickerToWebP, convertImageToWebP (helpers.go:838-867)
├── processor.go            # processStickerData, convertToWebPSticker (helpers.go:869-915)
├── exif.go                 # embedStickerEXIF, buildWhatsAppEXIF (helpers.go:917-976)
└── webp.go                 # injectWebPEXIF, parseWebPChunks, assembleWebP (helpers.go:978-1084)

internal/infrastructure/auth/hmac.go
                            # generateHmacSignature, encryptHMACKey, decryptHMACKey (helpers.go:609-679)

internal/infrastructure/media/outgoing.go
                            # ProcessOutgoingMedia (helpers.go:570-606)

internal/whatsapp/client/values.go  (or reuse existing)
                            # Values + updateUserInfo (auth.go:22-28, helpers.go:272-288)
                            # NOTE: Values already lives in root auth.go; migrate together in Phase 13.
```

**Steps**
1. Move each concern one sub-commit at a time within the phase (5 sub-commits total; final phase commit is a squash for rollback simplicity — but internal work-in-progress commits are OK on the branch before squash-merging).
2. Introduce `internal/application/port/WebhookDispatcher` interface; existing callers in `wmiau.go` now use the port.
3. Move constants (openGraphFetchTimeout, etc.) to their respective packages.
4. Delete `helpers.go` from root; delete `media.go` (already thin, `ProcessOutgoingMedia`'s twin — merge into `internal/infrastructure/media/outgoing.go`).
5. Delete `jpeg_thumbnail_test.go` from root → move to `internal/infrastructure/media/sticker/`.

**Files touched**: create ~9 files under `internal/infrastructure/`, delete `helpers.go`, `media.go`, `jpeg_thumbnail_test.go`.

**Verify**:
```bash
go build ./cmd/wuzapi
go test -race ./internal/...
go test -run TestJPEG ./internal/infrastructure/media/sticker/
```

**Rollback**: `git revert HEAD` restores root files.

**Estimated blast radius**: 1,191 LOC moved (helpers.go + media.go), ~15 imports rewired.

---

## Phase 13 — Migrate `stdio.go`, `rabbitmq.go`, `db.go`, `auth.go`, `history_sync.go`, `custom_handlers.go`, `clients.go`, `constants.go`

**Why**: These files each have a natural internal home but were blocked by `package main`. With Phase 10 done, they move mechanically.

**Target moves**
| Root file | Destination | Notes |
|---|---|---|
| `stdio.go` | `internal/infrastructure/stdio/server.go` | Consolidate with existing `internal/infrastructure/stdio/` |
| `stdio_test.go` | `internal/infrastructure/stdio/` | Move test alongside |
| `rabbitmq.go` | `internal/infrastructure/messaging/rabbitmq.go` | Merge with existing messaging pkg |
| `db.go` | `internal/infrastructure/db/connection.go` | Merge with existing `internal/infrastructure/db/` |
| `db_history_test.go` | `internal/infrastructure/db/` | Move test alongside |
| `auth.go` | `internal/interfaces/http/middleware/auth.go` | authAlice, authAdmin, Values, respondJSON |
| `history_sync.go` | `internal/infrastructure/whatsapp/history_sync.go` | Depends on Phase 11 (needs `client.MyClient`) |
| `clients.go` | `internal/whatsapp/client/manager.go` | ClientManager — depends on Phase 11 |
| `constants.go` | `internal/infrastructure/constants/events.go` | Merge `isValidEventType` with existing file |
| `custom_handlers.go` | `internal/interfaces/http/wiring/handlers.go` | initCustomHandlers wiring |
| `custom_routes.go` | `internal/interfaces/http/wiring/routes.go` | registerCustomRoutes |
| `routes.go` | `internal/interfaces/http/wiring/router.go` | server.routes() → package fn |
| `event_subscriptions_test.go`, `privacy_test.go`, `subscriptions_test.go`, `blocklist_test.go`, `killchannel_test.go`, `userinfo_cache_test.go`, `proxy_config_test.go` | Move alongside migrated impl | 7 test files |

**Steps**
1. Move each root file to its destination — one sub-move per file, batched into one atomic commit.
2. Update `cmd/wuzapi/main.go` imports.
3. Introduce `wiring.Setup(db, cfg) *http.ServeMux` façade for cmd/main.go to keep bootstrap thin.
4. Move constants (`supportedEventTypes`) that are still root-only.

**Files touched**: 24 root file moves, ~8 imports rewritten in cmd/main.go and wiring/.

**Verify**:
```bash
go build ./cmd/wuzapi
go test -race -count=1 ./...
docker build . -t wuzapi:phase13
```

**Rollback**: `git revert HEAD`.

**Estimated blast radius**: All remaining root .go files (except cmd/), ~2,900 LOC relocated.

---

## Phase 14 — Bootstrap minimization + docs finalization

**Why**: After phases 10–13, `cmd/wuzapi/main.go` becomes the only root code, but it may still exceed 100 LOC due to flag parsing + config loading. Extract config to `internal/app/config.go`.

**Steps**
1. Move flag definitions + env var overrides (`main.go:52-380`) to `internal/app/config.Load() *config.Config`.
2. Move CIDR parsing (`main.go:184-199`) to `internal/app/httpclient.NewSafeClient()` — already partially there.
3. Trim `cmd/wuzapi/main.go` to: parse config → wire deps → start server. Target: ≤ 80 LOC.
4. Update `README.md` architecture diagram to reflect final layout.
5. Refresh `.omc/plans/migration-summary.md` — mark phases 10–14 done, update final metrics.
6. Delete `.omc/plans/root-loc-baseline.txt` (superseded).
7. Run `make check` (build + vet + test + lint) as merge gate.

**Files touched**: `cmd/wuzapi/main.go`, `internal/app/config.go`, `README.md`, `.omc/plans/migration-summary.md`.

**Verify**:
```bash
make check
docker build . -t wuzapi:final
# Manual: docker run wuzapi:final --version
```

**Rollback**: `git revert HEAD`.

**Estimated blast radius**: ~500 LOC of config extraction; cmd/main.go from ~600 → 80 LOC.

---

## Verification matrix (per phase)

| Check | 10 | 11 | 12 | 13 | 14 |
|---|---|---|---|---|---|
| `go build ./cmd/wuzapi` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `go vet ./...` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `go test -race ./internal/...` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `make lint` (`golangci-lint run ./internal/...`) | ✓ | ✓ | ✓ | ✓ | ✓ |
| `docker build .` | ✓ | ✓ | ✓ | ✓ | ✓ |
| Contract tests (`TestProfileContract*`) | ✓ | ✓ | ✓ | ✓ | ✓ |
| Stdio JSON-RPC snapshot | — | — | — | ✓ | ✓ |
| Manual smoke (connect user, send message) | — | ✓ | — | ✓ | ✓ |

## Metrics targets

| Metric | Phase 9 (now) | Phase 14 (target) |
|---|---|---|
| Root .go LOC | 8,092 | ≤ 200 (cmd/wuzapi/main.go only) |
| Files at root | 24 | 1 |
| `package main` count | 1 (root) | 1 (cmd/wuzapi) |
| Internal .go files | 168 | ~190 |
| Circular import risk | Present (globals) | Zero |
| Config still global | Yes (`*flag.String`) | No (config.Config value) |

## Commit protocol

Each phase = one commit. Message template:

```
refactor(phase-N): <one-line summary>

<why this phase, structurally>

Files:
- <path>: <what happened>
...

Verified: build ✓ vet ✓ test ✓ lint ✓ docker ✓
```

## Execution mode

Recommended: `/ralph` on each phase individually, with a manual review pause between phases 11 and 12 (the extraction of `myEventHandler` deserves human eyes). Alternatively, run phases 10, 12, 13, 14 via `/ultrawork` (mechanical) and phase 11 solo.

**Do not** run all phases in one autopilot session — the intermediate states are valuable regression anchors.
