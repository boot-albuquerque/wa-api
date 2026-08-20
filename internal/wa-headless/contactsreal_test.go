package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAListsContactsWithoutDoubleCounting proves against the live roster
// what the unit suite proves against doubles: the collection describes some
// people twice, and the listing returns each of them once.
//
// It asserts COUNTS and RELATIONS, never a contact. The measurement that shaped
// the capability was 944 rows / 454 c.us / 489 lid / 390 phones reachable from
// lid rows and all 390 also present on their own — so the roster here must come
// out meaningfully smaller than the row count, with the difference accounted
// for by the merge counter rather than by silent dropping.
func TestRealSPAListsContactsWithoutDoubleCounting(t *testing.T) {
	requireRealSPA(t)
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Skip("WA_SEND_FROM_PROFILE is required for the live roster")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	got, err := contacts.New(runner, sess.Tab().Evaluate).List(ctx, "real/contacts")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("live roster: %s", got)

	if got.Rows == 0 {
		t.Fatal("the live roster reported zero rows; this account has contacts, so " +
			"an empty read is a broken read, not an empty address book")
	}
	if len(got.Contacts) == 0 {
		t.Fatal("rows were read but no contact survived the merge")
	}
	// NOTHING MAY VANISH. Every row is either its own person or merged into
	// one, so people + merged must account for the rows that are contacts.
	// A row that is neither is a group, and there was exactly one.
	accounted := len(got.Contacts) + got.Merged
	if accounted > got.Rows {
		t.Fatalf("people(%d) + merged(%d) = %d exceeds rows(%d): the merge invented "+
			"contacts", len(got.Contacts), got.Merged, accounted, got.Rows)
	}
	if dropped := got.Rows - accounted; dropped > 5 {
		t.Fatalf("%d rows are unaccounted for (rows=%d people=%d merged=%d). Only "+
			"non-user rows may be dropped, and the measurement found 1",
			dropped, got.Rows, len(got.Contacts), got.Merged)
	}
	if got.Merged == 0 {
		t.Fatal("no row was merged on a roster measured to hold ~390 duplicated " +
			"people; either the join stopped working or the page changed shape")
	}
	if len(got.Contacts) >= got.Rows {
		t.Fatalf("people(%d) is not smaller than rows(%d): deduplication did nothing",
			len(got.Contacts), got.Rows)
	}

	// Every returned contact must be addressable, and no identity may be blank.
	both, lidOnly, pnOnly, named := 0, 0, 0, 0
	for _, c := range got.Contacts {
		if c.Identity() == "" {
			t.Fatal("a contact came back with no identity at all")
		}
		switch {
		case c.PN != "" && c.LID != "":
			both++
		case c.LID != "":
			lidOnly++
		default:
			pnOnly++
		}
		if c.Pushname != "" {
			named++
		}
	}
	t.Logf("identities: both=%d lid-only=%d phone-only=%d | with pushname=%d",
		both, lidOnly, pnOnly, named)
	if both == 0 {
		t.Fatal("not one contact carries BOTH identities, which is what merging produces")
	}
}
