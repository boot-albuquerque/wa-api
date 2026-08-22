package events

import (
	"strings"
	"sync"
	"testing"
)

func collect(h *Hub, types ...Type) (*[]Event, *sync.Mutex, func()) {
	var mu sync.Mutex
	got := []Event{}
	stop := h.Subscribe(func(e Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	}, types...)
	return &got, &mu, stop
}

// A locally published fact reaches subscribers with its origin set, and with a
// ZERO sequence. The zero is not an omission: lifecycle facts have no place in
// the page's sequence, and giving them a number would put them into an order
// nobody measured.
func TestPublishSessionState_DeliversWithALocalOrigin(t *testing.T) {
	h := NewHub()
	got, mu, stop := collect(h, SessionReady)
	defer stop()

	h.PublishSessionState(SessionReady, "ready", "fresh")

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 {
		t.Fatalf("delivered %d events, want 1", len(*got))
	}
	e := (*got)[0]
	if e.Origin != SourceLocal {
		t.Errorf("origin = %q, want %q", e.Origin, SourceLocal)
	}
	if e.Seq != 0 {
		t.Errorf("seq = %d, want 0: a lifecycle fact has no page sequence", e.Seq)
	}
	if e.State != "ready" || e.Reason != "fresh" {
		t.Errorf("state/reason = %q/%q, want ready/fresh", e.State, e.Reason)
	}
	if e.ChatJID != "" || e.MessageID != "" {
		t.Error("a lifecycle fact carried an identity")
	}
}

// THE ZERO SEQUENCE MUST NOT BE READ AS A GAP. The Hub counts a jump in the
// page's sequence as a dropped event, and a lifecycle fact arriving between two
// page events is exactly the shape that would trip it: seq 0 after seq 7 looks
// like a rewind, and seq 8 after it would look like a jump if the zero had been
// recorded as the new high-water mark.
func TestLifecycleFactsDoNotCorruptTheGapCount(t *testing.T) {
	h := NewHub()
	_, _, stop := collect(h)
	defer stop()

	h.deliver(Event{Type: MessageAdded, Origin: SourcePage, Seq: 7})
	h.PublishSessionState(SessionStateChanged, "ALIVE", "APP_READY")
	h.deliver(Event{Type: MessageAdded, Origin: SourcePage, Seq: 8})

	if g := h.Stats().Gaps; g != 0 {
		t.Fatalf("gaps = %d, want 0: a lifecycle fact was counted as a lost page event", g)
	}
}

// The THREE lists are disjoint and together are everything. A type in two of
// them would mean the page can forge a fact only this process can know, or that
// a reclassification competes with a real handler; a type in none is a name no
// producer will ever emit.
//
// GroupTypes joined this test rather than being exempted from it: they are
// produced in Go from a page message, which is a third origin and needs the same
// arithmetic, not a special case.
func TestTypeListsAreDisjointAndComplete(t *testing.T) {
	lists := map[string][]Type{
		"PageTypes": PageTypes, "LocalTypes": LocalTypes, "GroupTypes": GroupTypes,
	}
	seen := map[Type]string{}
	for name, l := range lists {
		for _, t2 := range l {
			if other, dup := seen[t2]; dup {
				t.Errorf("%q is in both %s and %s", t2, other, name)
			}
			seen[t2] = name
		}
	}
	if len(KnownTypes) != len(PageTypes)+len(LocalTypes)+len(GroupTypes) {
		t.Fatalf("KnownTypes has %d, want %d+%d+%d", len(KnownTypes),
			len(PageTypes), len(LocalTypes), len(GroupTypes))
	}
	known := map[Type]bool{}
	for _, t2 := range KnownTypes {
		known[t2] = true
	}
	for t2 := range seen {
		if !known[t2] {
			t.Errorf("%q is in a producer list and not in KnownTypes", t2)
		}
	}
}

// EVERY MAPPED SUBTYPE LANDS ON A DECLARED GROUP TYPE. A typo in the table would
// otherwise produce an event type nothing knows about, which the ingress would
// then emit and no subscriber could match.
func TestEveryMappedSubtypeIsADeclaredGroupType(t *testing.T) {
	declared := map[Type]bool{}
	for _, g := range GroupTypes {
		declared[g] = true
	}
	if len(groupSubtypes) == 0 {
		t.Fatal("the subtype table is empty")
	}
	for sub, typ := range groupSubtypes {
		if !declared[typ] {
			t.Errorf("subtype %q maps to %q, which is not in GroupTypes", sub, typ)
		}
	}
	// AND THE ONE ACTUALLY OBSERVED must be mapped. Measured 2026-08-22 on the
	// lab account: 1 gp2 message of 395, subtype "membership_approval_mode".
	if got, ok := GroupTypeFor("membership_approval_mode"); !ok || got != GroupUpdated {
		t.Fatalf("the only subtype ever observed here maps to %q/%t", got, ok)
	}
}

// AN UNKNOWN SUBTYPE IS NOT SWALLOWED. The reference ends its chain with an
// `else` that turns anything unrecognised into a group update, so a subtype Meta
// adds tomorrow would arrive mislabelled. Here it stays unmapped.
func TestAnUnknownSubtypeIsNotClassified(t *testing.T) {
	for _, sub := range []string{"", "something_meta_adds_in_2027", "gp2"} {
		if got, ok := GroupTypeFor(sub); ok {
			t.Errorf("subtype %q was classified as %q; unmapped subtypes must stay "+
				"unclassified so they remain visible as MessageAdded", sub, got)
		}
	}
}

// The install script must not claim a lifecycle name. The page cannot know that
// a boot verified an inventory or that Go decided to stop, and a page-side
// emitter for one of those names would be a forgery the drain filter is there
// to refuse — but a filter is a poor place to discover the mistake.
func TestThePageDoesNotClaimLifecycleNames(t *testing.T) {
	script := installScript()
	for _, ty := range LocalTypes {
		if strings.Contains(script, "'"+string(ty)+"'") {
			t.Errorf("the install script emits %q, which only this process can know", ty)
		}
	}
}

// A subscriber filtered to page events must not be woken by lifecycle facts,
// and the reverse. Type filtering is the whole reason Subscribe takes types,
// and a bus that leaks across the filter makes every consumer defensive.
func TestFilteringSeparatesTheTwoOrigins(t *testing.T) {
	h := NewHub()
	pageOnly, pmu, stop1 := collect(h, MessageAdded)
	lifeOnly, lmu, stop2 := collect(h, SessionStopped)
	defer stop1()
	defer stop2()

	h.deliver(Event{Type: MessageAdded, Origin: SourcePage, Seq: 1})
	h.PublishSessionState(SessionStopped, "stopped", "graceful")

	pmu.Lock()
	defer pmu.Unlock()
	lmu.Lock()
	defer lmu.Unlock()
	if len(*pageOnly) != 1 || (*pageOnly)[0].Type != MessageAdded {
		t.Errorf("the page subscriber saw %v", *pageOnly)
	}
	if len(*lifeOnly) != 1 || (*lifeOnly)[0].Type != SessionStopped {
		t.Errorf("the lifecycle subscriber saw %v", *lifeOnly)
	}
}

// A closed Hub accepts no lifecycle fact either. Teardown emits a stop, and a
// teardown that closed the Hub first must not resurrect delivery.
func TestPublishAfterCloseIsSilent(t *testing.T) {
	h := NewHub()
	got, mu, stop := collect(h)
	defer stop()
	h.Close()
	h.PublishSessionState(SessionStopped, "stopped", "graceful")
	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 0 {
		t.Fatalf("a closed Hub delivered %d events", len(*got))
	}
}
