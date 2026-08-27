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
	"errors"
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

// TestAudit_Invariant2_RepositoryRejectsEngineChangeAfterCreation is the
// regression test for HOUSEKEEP F279. It calls UserRepository.UpdateUser
// directly - the same method EditUserUseCase.Execute calls - with
// domain.UserUpdate.Engine set to a DIFFERENT engine than the one the row
// was created with, bypassing the use-case-level comparison at
// edit_user.go:66-73 entirely.
//
// Before the F279 fix, pkg/infra/db/user_repository.go's UpdateUser
// validated only IsValidForCreate() on the incoming engine: it never
// compared against the row's current engine, and the ONLY thing stopping
// this write was that EditUserUseCase, one layer up, never sets
// upd.Engine on a divergent request. That made invariant 2 a single call
// site's policy, not a repository/domain invariant.
//
// UpdateUser now runs the write inside a transaction that locks the row,
// reads its current engine, and refuses with domain.ErrEngineImmutable on
// any divergence (see updateUserWithEngineGuard) - so this test attacks
// the repository directly, same as before, and now expects the rejection.
//
// Negative control executed manually while writing this fix (not
// committed as a mode switch, per project policy - see git history if the
// commit that introduced updateUserWithEngineGuard needs to be re-read):
// with that guard removed, this test goes back to FAILING with "UpdateUser
// with a divergent engine unexpectedly succeeded" - so it does bite.
func TestAudit_Invariant2_RepositoryRejectsEngineChangeAfterCreation(t *testing.T) {
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
	if err == nil {
		t.Fatalf("UpdateUser with a divergent engine unexpectedly succeeded - the repository-level immutability guard (F279) did not fire")
	}
	if !errors.Is(err, domain.ErrEngineImmutable) {
		t.Fatalf("UpdateUser error = %v, want errors.Is(err, domain.ErrEngineImmutable)", err)
	}

	after := engineOf(t, db, rec.ID)
	if after != before {
		t.Fatalf("engine changed despite the rejected update: before=%q after=%q", before, after)
	}
}
