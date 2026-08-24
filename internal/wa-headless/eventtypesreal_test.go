package waheadless

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chatstate"
	"wa-api/internal/wa-headless/capabilities/edit"
	"wa-api/internal/wa-headless/capabilities/react"
	"wa-api/internal/wa-headless/capabilities/revoke"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAEachEventTypeCanBeMadeToHappen is the rule this bus is built on:
// a type exists once something can MAKE it fire on demand.
//
// The upstream has 31 events, and it would be easy to declare 31 names, install
// 31 listeners, and ship a bus whose quiet halves nobody notices for months. A
// name that has never been seen firing is a promise, not a capability.
//
// So each type here is triggered by a capability this module already delivers,
// in one session, with the bus watching.
func TestRealSPAEachEventTypeCanBeMadeToHappen(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_EVENTS_TEST") == "" {
		t.Skip("set WA_HEADLESS_EVENTS_TEST=1; this sends, edits and deletes messages to the peer lab account")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	oldPoll := events.PollInterval
	events.PollInterval = 200 * time.Millisecond
	defer func() { events.PollInterval = oldPoll }()

	hub := events.NewHub()
	pump := events.NewPump(runner, eval, hub)
	pctx, stop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = pump.Run(pctx) }()
	defer func() { stop(); wg.Wait(); _ = pump.Uninstall(context.Background()) }()

	var mu sync.Mutex
	live := map[events.Type]int{}
	hub.Subscribe(func(e events.Event) {
		if e.Replay {
			return // history reloading is not a thing happening now
		}
		mu.Lock()
		defer mu.Unlock()
		live[e.Type]++
	})
	time.Sleep(3 * time.Second)
	mu.Lock()
	live = map[events.Type]int{}
	mu.Unlock()

	fired := func(ty events.Type, budget time.Duration) bool {
		t.Helper()
		deadline := time.Now().Add(budget)
		for time.Now().Before(deadline) {
			mu.Lock()
			n := live[ty]
			mu.Unlock()
			if n > 0 {
				return true
			}
			time.Sleep(200 * time.Millisecond)
		}
		return false
	}

	// message.added and message.ack — sending one message produces both.
	sent, err := send.Text(ctx, runner, eval, peer,
		fmt.Sprintf("wa-headless types probe %d", time.Now().UnixNano()), "types/send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !fired(events.MessageAdded, 20*time.Second) {
		t.Errorf("%s never fired after a send", events.MessageAdded)
	}
	if !fired(events.MessageAck, 30*time.Second) {
		t.Errorf("%s never fired after a send", events.MessageAck)
	}

	// message.edited
	if _, err := edit.New(runner, eval).Text(ctx, sent.ID.ID,
		fmt.Sprintf("wa-headless types probe %d (edited)", time.Now().UnixNano()), "types/edit"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !fired(events.MessageEdited, 20*time.Second) {
		t.Errorf("%s never fired after an edit", events.MessageEdited)
	}

	// chat.changed — archiving and unarchiving the lab chat moves a chat field.
	chatJID := findLabChatJID(ctx, t, runner, eval, peer)
	if chatJID != "" {
		cs := chatstate.New(runner, eval)
		if _, err := cs.SetArchived(ctx, chatJID, true, "types/archive"); err != nil {
			t.Errorf("archive: %v", err)
		} else {
			if !fired(events.ChatChanged, 20*time.Second) {
				t.Errorf("%s never fired after archiving", events.ChatChanged)
			}
			if _, err := cs.SetArchived(context.Background(), chatJID, false, "types/unarchive"); err != nil {
				t.Errorf("RESTORE FAILED — the lab chat is left archived: %v", err)
			}
		}
	}

	// message.reaction — reacting to the probe message. Adding is the half H53
	// proved; the reaction is taken back right after.
	r := react.New(runner, eval)
	if _, err := r.Add(ctx, sent.ID.ID, "\U0001F44D", "types/react"); err != nil {
		t.Errorf("react: %v", err)
	} else {
		if !fired(events.MessageReaction, 20*time.Second) {
			t.Errorf("%s never fired after a reaction", events.MessageReaction)
		}
		if _, err := r.Remove(context.Background(), sent.ID.ID, "types/unreact"); err != nil {
			t.Errorf("RESTORE FAILED — a reaction is left on the probe message: %v", err)
		}
	}

	// message.revoked — deleting the probe message for everyone also cleans up
	// after this test, which is the reason it goes last.
	if _, err := revoke.New(runner, eval).ForEveryone(ctx, sent.ID.ID, false, "types/revoke"); err != nil {
		t.Errorf("revoke: %v", err)
	} else if !fired(events.MessageRevoked, 20*time.Second) {
		t.Errorf("%s never fired after a revoke", events.MessageRevoked)
	}

	mu.Lock()
	t.Logf("MEASURED: live events by type %v", live)
	mu.Unlock()

	// contact.changed is NOT triggered here, and saying so is the point: this
	// module cannot make somebody else change their name or picture. It is
	// installed because contacts.onContact already proved that collection fires
	// (H43), and it is reported as unproven ON THIS BUS rather than assumed.
	mu.Lock()
	n := live[events.ContactChanged]
	mu.Unlock()
	if n == 0 {
		t.Logf("NOT PROVEN by this run: %s — nothing here can make another account "+
			"change its profile, and idle traffic did not produce one", events.ContactChanged)
	}
}
