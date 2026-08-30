package headless

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/addressbook"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// labContactName is what the proof writes, and it is written to be recognised
// by a human who finds it on the account: this is a test artifact, and it is
// deleted at the end of the run that made it.
const labContactName = "headless-lab"

// TestAddressbookSaveReal proves the address-book family AND the one event that
// had a listener and no proof.
//
// events.ContactChanged was installed by the bus and never fired in a test,
// because nothing in this module could make a contact record move: the only
// paths ran through another account editing its own profile, which this side
// cannot cause. A save moves the record HERE, which is the trigger the bus's
// own rule demands before a type counts as delivered.
//
// syncToPhone is FALSE. True would write the contact into the address book of
// the physical phone conta-A is paired with — a change outside this process
// that no test can undo.
func TestAddressbookSaveReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_ADDRBOOK") == "" {
		t.Skip("set WA_REAL_ADDRBOOK=1; this writes and then deletes a contact on conta-A")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	ab := addressbook.New(runner, sess.Tab().Evaluate)

	// THE CONTACT IS DELETED WHETHER OR NOT THE TEST PASSES, and by a defer
	// rather than t.Cleanup: cleanups run after every defer, so a cleanup here
	// would fire with the session already torn down. That exact mistake left
	// the lab group armed in H89.
	defer func() {
		if err := ab.Delete(context.Background(), peer, "ab/cleanup"); err != nil {
			t.Errorf("deleting the test contact: %v", err)
		}
	}()

	hub := events.NewHub()
	defer hub.Close()
	var mu sync.Mutex
	seen := map[events.Type]int{}
	defer hub.Subscribe(func(e events.Event) {
		if e.Replay {
			return
		}
		mu.Lock()
		seen[e.Type]++
		mu.Unlock()
	})()
	pumpCtx, stopPump := context.WithCancel(ctx)
	defer stopPump()
	go func() { _ = events.NewPump(runner, sess.Tab().Evaluate, hub).Run(pumpCtx) }()
	// Let the pump install and burn its replay window before the write.
	time.Sleep(3 * time.Second)

	countOf := func(ty events.Type) int {
		mu.Lock()
		defer mu.Unlock()
		return seen[ty]
	}

	// THE WRITE HAS TO BE A REAL CHANGE, AND THAT IS THE WHOLE LESSON OF H85.
	//
	// The first run of this test found the contact already named — a leftover
	// from a run whose delete had failed — so saveContactAction wrote the value
	// the record already held. A no-op moves nothing, no event fired, and the
	// test accused the bus of not delivering an event that had never been
	// caused. That is exactly the shape of the control that could not fail,
	// which this repository has now met from both sides.
	//
	// So the name is removed first, and the save's own HadName is asserted
	// false: if the record still carries a name at that point, nothing after
	// this line proves anything.
	if err := ab.Delete(ctx, peer, "ab/prepare"); err != nil {
		t.Fatalf("clearing the contact before the proof: %v", err)
	}
	time.Sleep(5 * time.Second)
	before := countOf(events.ContactChanged)

	saved, err := ab.Save(ctx, peer, labContactName, "", false, "ab/save")
	if err != nil {
		t.Fatalf("Save: %v (%s)", err, saved)
	}
	t.Logf("saved: %s", saved)
	if saved.HadName {
		t.Fatal("the record already carried a name; this save was a no-op and " +
			"nothing it does or does not fire means anything (H85)")
	}
	if !saved.HasName {
		t.Fatal("the save reported no name on the record")
	}
	if saved.SyncedToPhone {
		t.Fatal("the save synced to the phone; nothing here asked for that")
	}

	// THE EVENT. Give the pump several cycles past the write.
	deadline := time.Now().Add(20 * time.Second)
	for countOf(events.ContactChanged) == before && time.Now().Before(deadline) {
		time.Sleep(time.Second)
	}
	got := countOf(events.ContactChanged)
	mu.Lock()
	all := map[events.Type]int{}
	for k, v := range seen {
		all[k] = v
	}
	mu.Unlock()
	t.Logf("bus across the save: %v", all)
	if got == before {
		t.Fatalf("%s never fired; the type still has a listener and no proof", events.ContactChanged)
	}

	// A device read on a user this account really does exchange messages with.
	if n, err := ab.DeviceCount(ctx, peer, "ab/devices"); err != nil {
		t.Logf("device count unavailable: %v", err)
	} else {
		t.Logf("the peer has %d device(s)", n)
	}
}

// TestAddressbookDeleteIsIdempotentReal: deleting a number this account does not
// have saved must not be an error worth failing a cleanup over.
//
// It is checked against the REAL page rather than reasoned about, because the
// deferred cleanup in the test above runs on every path — including the ones
// where nothing was ever saved.
func TestAddressbookDeleteIsIdempotentReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_ADDRBOOK") == "" {
		t.Skip("set WA_REAL_ADDRBOOK=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	ab := addressbook.New(runner, sess.Tab().Evaluate)
	first := ab.Delete(ctx, peer, "ab/del-1")
	second := ab.Delete(ctx, peer, "ab/del-2")
	t.Logf("first delete: %v; second delete: %v", first, second)
	if second != nil && !errors.Is(second, addressbook.ErrDelete) {
		t.Fatalf("the second delete failed with something unexpected: %v", second)
	}
}

// TestLabMutualAddressbookSave puts each lab account in the other's contact
// list, and exists to reopen a finding that was closed as human-only.
//
// H50 could not prove presence OBSERVATION. Three hypotheses were knocked down
// by measurement; the fourth was privacy — WhatsApp can restrict "last seen and
// online" to contacts, and the two lab accounts do not have each other saved.
// The reopening condition recorded there was "an action on the physical
// handset", because nothing in this module could save a contact.
//
// addressbook.Save can. Whether it is ENOUGH is the open question: the
// last-seen filter is evaluated on the server, and a contact that exists only
// in this session's contact store may not count. That is precisely why this is
// worth one cheap run — syncToAddressbook stays FALSE, so nothing reaches
// anybody's phone, and the experiment is reversible by TestLabForgetEachOther.
func TestLabMutualAddressbookSave(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_LAB_MUTUAL") == "" {
		t.Skip("set WA_LAB_MUTUAL=save|forget")
	}
	mode := os.Getenv("WA_LAB_MUTUAL")
	if mode != "save" && mode != "forget" {
		t.Fatal("WA_LAB_MUTUAL must be save or forget")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	selfA := os.Getenv("WA_SELF_JID")
	if fromProfile == "" || toProfile == "" || peer == "" || selfA == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE, WA_SEND_TO_JID and WA_SELF_JID are required")
	}

	leg := func(what, profile, number, name string) {
		t.Helper()
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		defer h.Stop(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sess, err := h.Session(ctx)
		if err != nil {
			t.Fatalf("%s: boot: %v", what, err)
		}
		ab := addressbook.New(runner, sess.Tab().Evaluate)
		if mode == "forget" {
			if err := ab.Delete(ctx, number, what+"/forget"); err != nil {
				t.Errorf("%s: forget: %v", what, err)
				return
			}
			t.Logf("%s: forgotten", what)
			return
		}
		// SYNC IS OPT-IN PER RUN, and true reaches a physical handset.
		//
		// The first pass ran with false, on purpose, because it was cheap and
		// reversible. It was not enough: the contact came back
		// isAddressBookContact=1 with isContactSyncCompleted=0, and the
		// server-side "my contacts" filter only knows what was SYNCED (H91).
		// True is the next measurement and it needs a human's word, which is
		// why it is a flag and not a default.
		sync := os.Getenv("WA_LAB_MUTUAL_SYNC") == "1"
		saved, err := ab.Save(ctx, number, name, "", sync, what+"/save")
		if err != nil {
			t.Fatalf("%s: save: %v", what, err)
		}
		t.Logf("%s: %s", what, saved)
	}

	leg("A saves B", fromProfile, peer, labContactName+" B")
	leg("B saves A", toProfile, selfA, labContactName+" A")
}
