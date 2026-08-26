package bootstrap

import (
	"context"
	"fmt"

	"wa-api/pkg/domain"
	dbmig "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// # The temporary duality of engine names
//
// This package has spoken "wanoise"/"headless" since decision 94 — those are
// the values WA_API_ENGINE and WA_API_ENGINE_HEADLESS_SESSIONS already carry in
// production environments. domain.Engine speaks "wa_noise"/"wa_headless",
// because the persisted, publicly-visible contract was specified in snake_case.
//
// Both vocabularies are correct for their side, and this file is the ONLY
// crossing between them. Unifying them means changing environments operators
// already have, and that belongs with the worktree that makes routing consume
// domain.Engine (engine_routing.go, pkg/infra/enginerouter). Registered in
// HOUSEKEEP.md.

// engineDomainFor translates an infrastructure configuration engine name into
// the domain value.
//
// It ERRORS on anything else instead of falling back. The configuration was
// already validated by engineValido at this point, so an unknown value here
// means the two vocabularies drifted — and drifting silently would persist a
// wrong engine on every row, which is the most expensive kind of wrong this
// column can be.
func engineDomainFor(infraEngine string) (domain.Engine, error) {
	switch infraEngine {
	case EngineWaNoise:
		return domain.EngineWaNoise, nil
	case EngineWaHeadless:
		return domain.EngineWaHeadless, nil
	default:
		log.Error().Str("infra_engine", infraEngine).
			Msg("no domain engine for configured infrastructure engine")
		return "", fmt.Errorf("no domain engine for infrastructure engine %q", infraEngine)
	}
}

// runEngineBackfill records users.engine for the rows that predate the column.
//
// It runs on EVERY startup and is idempotent by construction: only rows still
// at legacy_unknown are touched. See dbmig.BackfillUserEngines for why the rule
// cannot live in the migration's SQL.
//
// Failure is FATAL, for the same reason the engine selection itself is: a row
// with no recorded engine is a session nothing can route, and starting anyway
// would move the failure to the first request — where it looks like a routing
// bug instead of a migration that did not finish.
func runEngineBackfill(ctx context.Context, db *sqlx.DB, sel EngineSelection) {
	if db == nil {
		// Only reachable from tests that build a server without a database.
		log.Warn().Msg("engine backfill skipped: no database handle")
		return
	}

	// engineDomainFor ja' disse QUAL valor nao mapeou; este Fatal e' o que
	// termina o processo, e nao uma segunda explicacao do mesmo erro.
	defaultEngine, err := engineDomainFor(sel.Default())
	if err != nil {
		log.Fatal().Err(err).Msg("cannot map configured default engine to a domain engine")
		return
	}

	report, err := dbmig.BackfillUserEngines(ctx, db, sel.SessoesEmHeadless(), defaultEngine)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to backfill session engines")
		return
	}

	// Printed on every startup, not only when it changed something: "zero rows
	// migrated" and "the backfill did not run" look identical in a log that
	// stays silent, and telling them apart is the whole point of this line.
	log.Info().
		Int("total_sessions", report.TotalUsers).
		Int("pending_before", report.PendingBefore).
		Int("to_wa_noise", report.ToWaNoise).
		Int("to_wa_headless", report.ToWaHeadless).
		Strs("listed_but_absent", report.ListedButAbsent).
		Str("default_engine", defaultEngine.String()).
		Msg("session engine backfill")
}
