package db

// migrationIDEngineWireRename rewrites every existing users.engine row from
// the old wire values (`wa_noise`/`wa_headless`) to the ones the repository
// rename of 2026-08-29 (HOUSEKEEP F385) cut the API contract over to
// (`noise`/`headless`).
//
// # Why a migration and not just a code change
//
// domain.EngineNoise/EngineHeadless changed VALUE (not just the Go symbol
// name — that was Fase 2, F383, already done). A clean cutover — the same
// pattern the F269 route rename already established for this project —
// means the OLD values are simply invalid input from this point forward,
// with no alias or transition window. That is only true if every row
// written under the old contract is rewritten too; otherwise a session
// created before this migration would read back an engine value the API
// itself now rejects.
const migrationIDEngineWireRename = 22

const migrationNameEngineWireRename = "rename_users_engine_wire_values"

// engineWireRenameSQL runs unchanged on both dialects: a plain UPDATE on a
// literal string column, the same reasoning migrations 17/19/21 already
// documented for their ALTERs.
//
// The old values are written as SQL literals, not as domain.Engine
// constants, on purpose: domain.EngineNoise/EngineHeadless now hold the NEW
// values, and this migration exists specifically to find rows still holding
// the value those constants used to have before this rename.
const engineWireRenameSQL = `
UPDATE users SET engine = 'noise' WHERE engine = 'wa_noise';
UPDATE users SET engine = 'headless' WHERE engine = 'wa_headless';
`

const engineWireRenameDownSQL = `
UPDATE users SET engine = 'wa_noise' WHERE engine = 'noise';
UPDATE users SET engine = 'wa_headless' WHERE engine = 'headless';
`
