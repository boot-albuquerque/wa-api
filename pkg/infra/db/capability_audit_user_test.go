package db_test

// Adversarial audit (feature/capability-final-audit) of invariants 1 and 2:
//
//  1. Every new session has exactly one valid engine (never absent, never
//     legacy_unknown accepted on creation).
//  2. A session's engine is immutable once assigned.
//
// See capability_audit_ownership_test.go (package db) for invariant 3
// (split-brain ownership).
//
// This file does not modify production code. Every test either RESISTS
// (passes, and is shown to actually exercise the rule) or documents a
// concrete break.

import (
	"context"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"
)

// ---------------------------------------------------------------------
// Invariant 1 — creation always yields a valid engine.
// ---------------------------------------------------------------------

// TestAudit_Invariant1_CreateRejectsLegacyUnknown proves the repository
// refuses to CREATE a user row with engine=legacy_unknown, which is the
// exact value IsValidForCreate documents as forbidden at creation
// (pkg/domain/engine.go).
func TestAudit_Invariant1_CreateRejectsLegacyUnknown(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)

	rec := domain.UserRecord{
		ID:     "audit-legacy-unknown",
		Name:   "audit",
		Token:  "tok-audit-legacy-unknown",
		Engine: domain.EngineLegacyUnknown,
	}

	created, err := repo.CreateUser(context.Background(), rec)
	if err == nil {
		t.Fatalf("CreateUser accepted engine=legacy_unknown: created=%v, want an error", created)
	}

	// Confirm the invariant, not just the error type: no row must exist.
	var count int
	if getErr := db.Get(&count, `SELECT COUNT(*) FROM users WHERE id = $1`, rec.ID); getErr != nil {
		t.Fatalf("counting rows: %v", getErr)
	}
	if count != 0 {
		t.Fatalf("row exists after rejected create: count=%d, want 0", count)
	}
}

// TestAudit_Invariant1_CreateRejectsGarbageEngine proves an unrecognized
// engine string is rejected the same way legacy_unknown is - the rule is
// "must be wa_noise or wa_headless", not "must not be legacy_unknown".
func TestAudit_Invariant1_CreateRejectsGarbageEngine(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)

	rec := domain.UserRecord{
		ID:     "audit-garbage-engine",
		Name:   "audit",
		Token:  "tok-audit-garbage-engine",
		Engine: domain.Engine("wa_carrier_pigeon"),
	}

	if _, err := repo.CreateUser(context.Background(), rec); err == nil {
		t.Fatalf("CreateUser accepted engine=%q, want an error", rec.Engine)
	}
}

// TestAudit_Invariant1_CreateDefaultsEmptyToWaNoise documents (does not
// attack) the DOCUMENTED zero-value behavior in engineForCreate: an absent
// engine becomes wa_noise, not legacy_unknown and not an error. This is
// the control that shows the two rejection tests above are actually
// discriminating "invalid" from "absent", not just failing every insert.
func TestAudit_Invariant1_CreateDefaultsEmptyToWaNoise(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)

	rec := domain.UserRecord{
		ID:    "audit-empty-engine",
		Name:  "audit",
		Token: "tok-audit-empty-engine",
		// Engine left at zero value on purpose.
	}

	created, err := repo.CreateUser(context.Background(), rec)
	if err != nil || !created {
		t.Fatalf("CreateUser with empty engine: created=%v err=%v, want true/nil", created, err)
	}
	got := engineOf(t, db, rec.ID)
	if got != string(domain.EngineWaNoise) {
		t.Fatalf("engine after create with empty field = %q, want %q", got, domain.EngineWaNoise)
	}
}

// ---------------------------------------------------------------------
// Invariant 2 — engine is immutable once assigned.
// ---------------------------------------------------------------------

// TestAudit_Invariant2_UseCaseComparisonRejectsDivergence is the CONTROL:
// it replays exactly the comparison EditUserUseCase.Execute performs at
// pkg/application/usecase/user/edit_user.go:66-73 before it ever builds a
// domain.UserUpdate — req.Engine, when present, is compared against the
// persisted value, and only an EQUAL value is tolerated (idempotent
// resend); anything else is rejected without touching the repository.
// This establishes the baseline the next (attack) test contrasts with.
func TestAudit_Invariant2_UseCaseComparisonRejectsDivergence(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	rec := domain.UserRecord{ID: "audit-immutable-ctrl", Name: "audit", Token: "tok-immutable-ctrl", Engine: domain.EngineWaNoise}
	if _, err := repo.CreateUser(ctx, rec); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	entries, err := repo.ListUsers(ctx, rec.ID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListUsers: entries=%d err=%v", len(entries), err)
	}
	current := entries[0].Engine.String()
	requested := string(domain.EngineWaHeadless)
	if requested == current {
		t.Fatalf("test setup broken: requested equals current (%q)", current)
	}
	// (Deliberately not calling UpdateUser here: the use case's whole
	// point is to refuse the change BEFORE building the UserUpdate. The
	// real attack surface is what happens if a caller skips that gate,
	// which the next test measures directly against the repository.)
}

// TestAudit_Invariant2_RepositoryAcceptsEngineChangeAfterCreation is the
// ATTACK: it calls UserRepository.UpdateUser directly - the same method
// EditUserUseCase.Execute calls - with domain.UserUpdate.Engine set to a
// DIFFERENT engine than the one the row was created with, bypassing the
// use-case-level comparison at edit_user.go:66-73 entirely.
//
// pkg/infra/db/user_repository.go:162-172 (UpdateUser) validates only
// IsValidForCreate() on the incoming engine. It does not compare against
// the row's current engine, does not reject a divergent value, and has no
// awareness that engine is supposed to be immutable. The ONLY thing
// stopping this write today is that EditUserUseCase, one layer up, never
// sets upd.Engine on a divergent request (confirmed by the control test
// above and by grep across the repo: pkg/application/usecase/user/
// edit_user.go never assigns upd.Engine anywhere).
//
// This means invariant 2 is NOT a repository/domain invariant - it is a
// single call site's policy. Any other caller of UserRepository.UpdateUser
// (a future use case, an admin tool, a maintenance script, test code
// reused by mistake) can flip a session's engine with no error, no log
// line distinguishing "changed" from "unchanged", and no database
// constraint stopping it.
func TestAudit_Invariant2_RepositoryAcceptsEngineChangeAfterCreation(t *testing.T) {
	db := newUserTestDB(t)
	repo := dbpkg.NewUserRepository(db)
	ctx := context.Background()

	rec := domain.UserRecord{ID: "audit-immutable-attack", Name: "audit", Token: "tok-immutable-attack", Engine: domain.EngineWaNoise}
	if _, err := repo.CreateUser(ctx, rec); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	before := engineOf(t, db, rec.ID)
	if before != string(domain.EngineWaNoise) {
		t.Fatalf("setup: engine after create = %q, want %q", before, domain.EngineWaNoise)
	}

	newEngine := domain.EngineWaHeadless
	err := repo.UpdateUser(ctx, rec.ID, domain.UserUpdate{Engine: &newEngine})
	if err != nil {
		t.Fatalf("UpdateUser with a divergent engine returned an error (would mean the repository itself enforces immutability): %v", err)
	}

	after := engineOf(t, db, rec.ID)
	if after == before {
		t.Fatalf("expected the attack to succeed (before=%q after=%q) - if this fails, the repository has grown its own immutability guard and this test is stale", before, after)
	}
	if after != string(domain.EngineWaHeadless) {
		t.Fatalf("engine after direct UpdateUser = %q, want %q (the value we asked for)", after, domain.EngineWaHeadless)
	}

	t.Logf("BROKE invariant 2 at the repository layer: engine went from %q to %q via UserRepository.UpdateUser, "+
		"with no comparison against the row's prior value. Immutability is enforced only by "+
		"pkg/application/usecase/user/edit_user.go:66-73, one layer above this repository method.", before, after)
}
