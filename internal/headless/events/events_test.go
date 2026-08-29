package events

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestUnsubscribeStopsDeliveryAndIsSafeTwice. The returned function is the ONLY
// way to detach, so it has to work and it has to tolerate being called again —
// teardown paths run more than once more often than anybody plans.
func TestUnsubscribeStopsDeliveryAndIsSafeTwice(t *testing.T) {
	h := NewHub()
	var got int64
	off := h.Subscribe(func(Event) { atomic.AddInt64(&got, 1) }, MessageAdded)

	h.deliver(Event{Type: MessageAdded, Seq: 1})
	if atomic.LoadInt64(&got) != 1 {
		t.Fatalf("the subscriber did not receive: %d", got)
	}
	off()
	h.deliver(Event{Type: MessageAdded, Seq: 2})
	if atomic.LoadInt64(&got) != 1 {
		t.Fatalf("delivery continued after unsubscribe: %d", got)
	}
	off() // must not panic and must not resurrect anything
	if s := h.Stats(); s.Subscribers != 0 {
		t.Fatalf("a second unsubscribe changed the count: %s", s)
	}
}

// TestASubscriberOnlyGetsItsTypes, and an empty type set means everything —
// which is what a diagnostic subscriber wants.
func TestASubscriberOnlyGetsItsTypes(t *testing.T) {
	h := NewHub()
	var added, all int64
	h.Subscribe(func(Event) { atomic.AddInt64(&added, 1) }, MessageAdded)
	h.Subscribe(func(Event) { atomic.AddInt64(&all, 1) })

	h.deliver(Event{Type: MessageAdded, Seq: 1})
	h.deliver(Event{Type: ChatChanged, Seq: 2})
	h.deliver(Event{Type: MessageAck, Seq: 3})

	if added != 1 {
		t.Fatalf("the typed subscriber got %d events, want 1", added)
	}
	if all != 3 {
		t.Fatalf("the untyped subscriber got %d events, want 3", all)
	}
}

// TestAGapInTheSequenceIsCountedAsADrop. The page counts what it threw away and
// Go counts what never arrived; a test comparing them is what makes either
// number trustworthy.
func TestAGapInTheSequenceIsCountedAsADrop(t *testing.T) {
	h := NewHub()
	h.Subscribe(func(Event) {})
	h.deliver(Event{Type: MessageAdded, Seq: 1})
	h.deliver(Event{Type: MessageAdded, Seq: 5}) // three missing
	if s := h.Stats(); s.Gaps != 3 {
		t.Fatalf("gaps = %d, want 3: %s", s.Gaps, s)
	}
	// Out-of-order or repeated sequence must not invent gaps.
	h.deliver(Event{Type: MessageAdded, Seq: 5})
	h.deliver(Event{Type: MessageAdded, Seq: 6})
	if s := h.Stats(); s.Gaps != 3 {
		t.Fatalf("gaps moved on an in-order delivery: %s", s)
	}
}

// TestCloseDropsEverySubscriptionAndIsIdempotent.
func TestCloseDropsEverySubscriptionAndIsIdempotent(t *testing.T) {
	h := NewHub()
	var got int64
	h.Subscribe(func(Event) { atomic.AddInt64(&got, 1) })
	h.Close()
	h.deliver(Event{Type: MessageAdded, Seq: 1})
	if got != 0 {
		t.Fatalf("delivery survived Close: %d", got)
	}
	if !h.Closed() {
		t.Fatal("Closed() disagrees with Close()")
	}
	h.Close() // must not panic
	// And subscribing after Close hands back a no-op rather than a live
	// subscription nobody will ever feed.
	off := h.Subscribe(func(Event) { atomic.AddInt64(&got, 1) })
	off()
	h.deliver(Event{Type: MessageAdded, Seq: 2})
	if got != 0 {
		t.Fatalf("a post-Close subscription received: %d", got)
	}
}

// TestAHandlerMayCallBackIntoTheHub. Delivery happens outside the lock because
// the first thing somebody writes is a handler that subscribes or reads Stats,
// and a deadlock there would be blamed on their code.
func TestAHandlerMayCallBackIntoTheHub(t *testing.T) {
	h := NewHub()
	done := make(chan struct{})
	h.Subscribe(func(Event) {
		_ = h.Stats()
		off := h.Subscribe(func(Event) {})
		off()
		close(done)
	}, MessageAdded)
	h.deliver(Event{Type: MessageAdded, Seq: 1})
	select {
	case <-done:
	default:
		t.Fatal("the handler did not complete; delivery is holding the lock")
	}
}

// TestConcurrentSubscribersAndDeliveries is the -race case: a bus whose
// subscriber set changes while events flow is the ordinary condition, not an
// edge one.
func TestConcurrentSubscribersAndDeliveries(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				off := h.Subscribe(func(Event) {}, MessageAdded)
				off()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 400; i++ {
			h.deliver(Event{Type: MessageAdded, Seq: int64(i)})
		}
	}()
	wg.Wait()
	if s := h.Stats(); s.Subscribers != 0 {
		t.Fatalf("subscribers leaked: %s", s)
	}
}

// TestANilHandlerIsRefusedWithoutPanicking.
func TestANilHandlerIsRefusedWithoutPanicking(t *testing.T) {
	h := NewHub()
	off := h.Subscribe(nil, MessageAdded)
	off()
	h.deliver(Event{Type: MessageAdded, Seq: 1})
	if s := h.Stats(); s.Subscribers != 0 {
		t.Fatalf("a nil handler was registered: %s", s)
	}
}

// TestTheEventCarriesNoContent. An event bus is where a no-content rule erodes
// first, because every capability looks at it.
func TestTheEventCarriesNoContent(t *testing.T) {
	e := Event{Type: MessageAdded, Seq: 7, ChatJID: "1@c.us",
		MessageID: "3EB0", Kind: "chat", BodyLen: 42}
	s := e.String()
	if !strings.Contains(s, "bodyLen=42") {
		t.Fatalf("the length is missing: %s", s)
	}
	if strings.Contains(s, "1@c.us") || strings.Contains(s, "3EB0") {
		t.Fatalf("the rendering carries identities: %s", s)
	}
}

// TestThePageAndGoAgreeOnTheEventNames is the failure nobody notices: a handler
// installed for a name nothing subscribes to, or the reverse.
func TestThePageAndGoAgreeOnTheEventNames(t *testing.T) {
	script := installScript()
	for _, ty := range PageTypes {
		if !strings.Contains(script, "'"+string(ty)+"'") {
			t.Errorf("the ingress installs no handler that emits %q", ty)
		}
	}
	// And nothing is emitted that Go would throw away.
	for _, emitted := range []string{"message.added", "message.ack", "chat.changed"} {
		found := false
		for _, ty := range PageTypes {
			if string(ty) == emitted {
				found = true
			}
		}
		if !found {
			t.Errorf("the ingress emits %q and no Type matches it", emitted)
		}
	}
}

// TestTheIngressIsIdempotentAndBounded, read from the script itself: both
// properties are what keep a reload from doubling the handlers and a slow
// consumer from growing somebody else's heap.
func TestTheIngressIsIdempotentAndBounded(t *testing.T) {
	script := installScript()
	if !strings.Contains(script, "if (st && st.installed) { return { installed: true, already: true }; }") {
		t.Fatal("the install is not idempotent; a reinstall would add a second set of handlers")
	}
	if !strings.Contains(script, "if (s.buf.length >= s.cap) { s.dropped++; return; }") {
		t.Fatal("the page buffer is unbounded, or drops without counting")
	}
	if !strings.Contains(script, "looksRevoked") {
		t.Fatal("the revoke handler emits on a field change without checking the message reads as revoked")
	}
	if !strings.Contains(script, "s.handlers = [[MC, 'add', onAdd]") {
		t.Fatal("the handlers are not recorded, so Uninstall cannot remove them")
	}
	if !strings.Contains(uninstallScript, ".off(h[1], h[2])") {
		t.Fatal("uninstall does not detach the handlers it installed")
	}
}

// TestTheClockIsThePagesOnlyForDiagnosis. Invariant 6: Date.now() may stamp an
// event, and nothing may DECIDE on it inside the page.
func TestTheClockIsThePagesOnlyForDiagnosis(t *testing.T) {
	script := installScript()
	if !strings.Contains(script, "row.at = Date.now();") {
		t.Fatal("events carry no page timestamp, so ordering cannot be diagnosed")
	}
	for _, banned := range []string{"setTimeout(", "setInterval("} {
		if strings.Contains(script, banned) || strings.Contains(drainScript, banned) {
			t.Fatalf("the page schedules its own work (%s)", banned)
		}
	}
}

// THE PAGE CLASS TRAVELS, and this is the whole content of the
// AUTHENTICATION_FAILURE row.
//
// A boot that dies at not_ready against a QR screen and one that dies at
// not_ready against an unresponsive page carry the SAME stage. The upstream
// emits AUTHENTICATION_FAILURE for the first and something else for the second,
// and until the class rode on the event a subscriber here could not tell them
// apart — the distinction lived in the error message, and error messages do not
// enter this bus (H88).
func TestABootFailureCarriesWhatThePageLookedLike(t *testing.T) {
	h := NewHub()
	var got []Event
	unsub := h.Subscribe(func(e Event) { got = append(got, e) })
	defer unsub()

	h.PublishSessionStateWithClass(SessionBootFailed, "boot_failed", "not_ready", "LOGIN_REQUIRED")
	h.PublishSessionStateWithClass(SessionBootFailed, "boot_failed", "not_ready", "UNRESPONSIVE")

	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	// A FIXTURE E' CONFERIDA ANTES DA ASSERCAO: se os dois estagios diferissem,
	// este teste passaria sem medir a distincao que afirma medir.
	if got[0].Reason != got[1].Reason {
		t.Fatal("the fixture is wrong: both failures must share a stage")
	}
	if got[0].PageClass == got[1].PageClass {
		t.Fatalf("two failures at the same stage came through indistinguishable "+
			"(%q both), which is the state this field exists to end", got[0].PageClass)
	}
	if got[0].PageClass != "LOGIN_REQUIRED" {
		t.Fatalf("the class did not survive: %q", got[0].PageClass)
	}
}

// THE OLD ENTRY POINT CARRIES NO CLASS. Every caller with nothing to say about
// the page keeps saying nothing, rather than defaulting to a value that would
// read as a measurement.
func TestPublishingWithoutAClassLeavesItEmpty(t *testing.T) {
	h := NewHub()
	var got Event
	unsub := h.Subscribe(func(e Event) { got = e })
	defer unsub()
	h.PublishSessionState(SessionStopped, "stopped", "browser.close")
	if got.PageClass != "" {
		t.Fatalf("a publish with no class invented one: %q", got.PageClass)
	}
}
