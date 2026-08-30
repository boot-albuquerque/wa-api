package headless

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/chats"
	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeChatRemovedEvent re-measures a BLOCKED row whose justification this
// week's work invalidated.
//
// CHAT_REMOVED is BLOCKED on "provar exigiria APAGAR uma conversa, destruindo a
// fixture de todos os outros testes (H66)". H166 deleted a conversation — in a
// throwaway group built for the purpose — and proved chats.Delete. The reason is
// gone; the verdict was never re-checked.
//
// The question here is narrower than H166's: not whether Delete works, but
// whether it produces anything a subscriber can see.
func TestProbeChatRemovedEvent(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHATREMOVED") == "" {
		t.Skip("set WA_PROBE_CHATREMOVED=1 (creates a throwaway group and deletes it)")
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
	eval := sess.Tab().Evaluate
	g, c := group.New(runner, eval), chats.New(runner, eval)

	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/chatremoved/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	subject := "headless chatremoved probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := g.Ensure(ctx, subject, []string{ident.JID}, "probe/chatremoved/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	gjid := created.JID
	defer func() {
		_ = g.Leave(context.Background(), gjid, "probe/chatremoved/cleanup-leave")
		_ = c.Delete(context.Background(), gjid, "probe/chatremoved/cleanup-delete")
	}()
	if !created.Created {
		t.Fatal("Ensure matched an existing group instead of creating one")
	}

	// O BARRAMENTO ENTRA DEPOIS DO PREPARO (a lição da H169).
	hub := events.NewHub()
	var mu sync.Mutex
	type row struct {
		Type events.Type
		Chat string
	}
	var seen []row
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, row{e.Type, e.ChatJID})
	})
	defer unsub()
	pump := events.NewPump(runner, eval, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(8 * time.Second)
	mu.Lock()
	base := len(seen)
	mu.Unlock()

	if err := g.Leave(ctx, gjid, "probe/chatremoved/leave"); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	time.Sleep(4 * time.Second)
	if err := c.Delete(ctx, gjid, "probe/chatremoved/delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	time.Sleep(15 * time.Second)

	mu.Lock()
	fresh := append([]row(nil), seen[base:]...)
	mu.Unlock()
	tally := map[string]int{}
	for _, r := range fresh {
		k := string(r.Type)
		if r.Chat == gjid {
			k += " [this group]"
		}
		tally[k]++
	}
	t.Logf("events after leave+delete: %d", len(fresh))
	for k, n := range tally {
		t.Logf("   %s x%d", k, n)
	}

	// A PERGUNTA E' SE HA' SINAL PROPRIO. O barramento escuta COLEÇÕES; uma
	// conversa que sai da ChatCollection so' produz evento se houver ouvinte de
	// 'remove' nela — e o ingresso instala 'remove' apenas em MsgCollection.
	var forThisGroup int
	for _, r := range fresh {
		if r.Chat == gjid {
			forThisGroup++
		}
	}
	t.Logf("events naming the deleted conversation: %d", forThisGroup)
}
