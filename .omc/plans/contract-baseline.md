# Contract Baseline — disparazaap-wuzapi

Captured: 2026-07-30 | Branch: refactor/clean-architecture

## Existing contracts

| Route | Method | Test | Schema fields |
|---|---|---|---|
| /session/profile | GET | TestProfileContract | pushname, avatar_url, avatar_id, jid, full_name, business_name |
| /session/profile | GET | TestProfileContract_EmptyFields | Handles empty/incomplete payloads |
| /session/profile | GET | TestProfileContract_GroupJID | Validates group JID format with `@g.us` suffix |

## Internal handler coverage (custom_routes.go)

- **71 routes** use `customHandlerSet.*` (internal handlers)
- **26 routes** use `s.*()` legacy server methods
- **6 admin routes** in routes.go (subrouter on `/admin` prefix)

All contract tests pass: `go test -v -run TestProfileContract ./internal/interfaces/http/...`

## Next

- Add contract tests for send-message, get-status, and group-list as Phase 4 sub-commits complete
- Re-run before Phase 6 (root file deletion)
