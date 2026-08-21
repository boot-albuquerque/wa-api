package presence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	ok         bool
	stage      string
	why        string
	subscribed bool
	online     bool
	chatstate  string

	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "announce"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","subscribed":%t,"online":%t,"chatstate":%q}`,
			p.subscribed, p.online, p.chatstate)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func announcer(p *pageDouble) *Announcer { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := opBudget, opTick
	sb, st := subscribeBudget, subscribeTick
	opBudget, opTick = 200*time.Millisecond, 10*time.Millisecond
	subscribeBudget, subscribeTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() {
		opBudget, opTick = ob, ot
		subscribeBudget, subscribeTick = sb, st
	})
}

const peer = "5541992421234@c.us"

// TestAnUnknownStateIsRefusedRatherThanDefaulted. "We sent nothing" and "we
// sent paused" look identical from the caller's side, so a state this package
// does not know must not quietly become one it does.
func TestAnUnknownStateIsRefusedRatherThanDefaulted(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	err := announcer(p).Set(context.Background(), peer, State("dancing"), "t/set")
	if !errors.Is(err, ErrUnknownState) {
		t.Fatalf("got %v, want ErrUnknownState", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to announce %d unknown state(s)", p.kicks)
	}
}

// TestEachStateCallsItsOwnPageFunction. The mapping is explicit precisely so a
// state cannot be turned into a function name by string surgery — composing and
// recording are different announcements and must not collapse.
func TestEachStateCallsItsOwnPageFunction(t *testing.T) {
	compressClock(t)
	for state, want := range map[State]string{
		StateComposing: "markComposing",
		StatePaused:    "markPaused",
		StateRecording: "markRecording",
	} {
		p := &pageDouble{ok: true}
		if err := announcer(p).Set(context.Background(), peer, state, "t/set"); err != nil {
			t.Fatalf("Set(%s): %v", state, err)
		}
		if !strings.Contains(p.lastScript, "A."+want+"(") {
			t.Fatalf("state %q did not call %s", state, want)
		}
	}
}

// TestAnnouncingNeverCreatesAChat. Telling somebody you are typing is not the
// same as opening a conversation with them, and a capability that did both
// would create chats as a side effect of a status update.
func TestAnnouncingNeverCreatesAChat(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if err := announcer(p).Set(context.Background(), peer, StateComposing, "t/set"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if strings.Contains(p.lastScript, "findOrCreateLatestChat") {
		t.Fatal("the script can CREATE a chat; announcing presence must only look one up")
	}
	if !strings.Contains(p.lastScript, "Chats.get(") {
		t.Fatal("the script does not look the chat up at all")
	}
	// And it must RESOLVE the identity before looking up: this build files
	// chats under the identity the server assigns, so a lookup by the caller's
	// number answers NO_CHAT for two accounts that talk daily (H34, H39).
	if !strings.Contains(p.lastScript, "queryWidExists") {
		t.Fatal("the lookup does not resolve the identity first")
	}
}

// TestObservingWithoutASubscriptionIsAnError. Measured: zero of 384 presence
// models were subscribed. Returning "offline, not typing" for an unsubscribed
// presence would be a fact-shaped guess, and the caller could not tell it from
// somebody genuinely idle.
func TestObservingWithoutASubscriptionIsAnError(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, subscribed: false}
	got, err := announcer(p).Observe(context.Background(), peer, "t/observe")
	if !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("got %v, want ErrNotSubscribed", err)
	}
	if got.Subscribed {
		t.Fatal("the snapshot claims a subscription it does not have")
	}
}

func TestObservingReportsTheChatstateVerbatim(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, subscribed: true, online: true, chatstate: "composing"}
	got, err := announcer(p).Observe(context.Background(), peer, "t/observe")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !got.Typing() {
		t.Fatalf("Typing()=false for chatstate %q", got.ChatState)
	}
	if !got.Online {
		t.Fatal("Online was dropped")
	}

	// A value this package has not seen must survive unchanged rather than
	// being mapped onto one it has.
	p2 := &pageDouble{ok: true, subscribed: true, chatstate: "something_new"}
	got2, err := announcer(p2).Observe(context.Background(), peer, "t/observe")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if got2.ChatState != "something_new" {
		t.Fatalf("an unknown chatstate was rewritten to %q", got2.ChatState)
	}
	if got2.Typing() {
		t.Fatal("an unknown chatstate was read as typing")
	}
}

// TestObserveSubscribesBeforeReading is the ORDER: reading first would report
// the silence that exists precisely because nobody subscribed.
func TestObserveSubscribesBeforeReading(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, subscribed: true}
	if _, err := announcer(p).Observe(context.Background(), peer, "t/observe"); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	sub := strings.Index(p.lastScript, "subscribeUserPresence")
	read := strings.Index(p.lastScript, "PresenceCollection")
	if sub < 0 {
		t.Fatal("Observe never subscribes; measured, zero of 384 presences were " +
			"subscribed, so it would report everyone as permanently silent")
	}
	if read < 0 || sub > read {
		t.Fatal("the collection is read BEFORE subscribing")
	}
}

// TestTheRawBridgeIsNotUsed. WAWebChatStateBridge takes a wid and sends the
// protocol directly, skipping the app's own guards on newsletters, bots and
// broadcasts. Using it would mean deciding we know better than the app about
// where a typing indicator belongs.
func TestTheRawBridgeIsNotUsed(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if err := announcer(p).Set(context.Background(), peer, StateComposing, "t/set"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if strings.Contains(p.lastScript, "WAWebChatStateBridge") {
		t.Fatal("the script uses the raw bridge, bypassing the app's guards")
	}
	if !strings.Contains(p.lastScript, "WAWebPresenceChatAction") {
		t.Fatal("the script does not use the chat action")
	}
}

func TestOnlineAndOfflineAreDifferentCalls(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	if err := announcer(p).SetOnline(context.Background(), true, "t/on"); err != nil {
		t.Fatalf("SetOnline(true): %v", err)
	}
	if !strings.Contains(p.lastScript, "sendPresenceAvailable") {
		t.Fatal("going online did not call sendPresenceAvailable")
	}
	if err := announcer(p).SetOnline(context.Background(), false, "t/off"); err != nil {
		t.Fatalf("SetOnline(false): %v", err)
	}
	if !strings.Contains(p.lastScript, "sendPresenceUnavailable") {
		t.Fatal("going offline did not call sendPresenceUnavailable")
	}
}

func TestAMissingChatIsErrNoChat(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "resolve", why: "NO_CHAT"}
	if err := announcer(p).Set(context.Background(), peer, StateComposing, "t/set"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
}

func TestCancelledContextAnnouncesNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := announcer(p).Set(ctx, peer, StateComposing, "t/set"); err == nil {
		t.Fatal("a cancelled context announced presence")
	}
	if p.kicks != 0 {
		t.Fatalf("announced %d time(s) for a caller that had given up", p.kicks)
	}
}

// TestObserveWaitsForTheSubscriptionToLand. subscribeUserPresence returns
// before isSubscribed flips, so reading in the same breath is a race — the
// first version of Observe did exactly that, passed once, and failed the next
// run. A double that flips only after a few polls is what makes the retry
// load-bearing.
func TestObserveWaitsForTheSubscriptionToLand(t *testing.T) {
	compressClock(t)
	p := &lateSubscribeDouble{flipAfter: 3, chatstate: "composing"}
	got, err := New(engine.NewRunner(), p.eval).Observe(context.Background(), peer, "t/observe")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !got.Subscribed || !got.Typing() {
		t.Fatalf("the late subscription was not waited for: %s", got)
	}
	if p.reads < 3 {
		t.Fatalf("read %d time(s); the retry is not doing anything", p.reads)
	}
}

// lateSubscribeDouble reports "not subscribed" for the first few reads, the way
// the page does while the subscription is in flight.
type lateSubscribeDouble struct {
	flipAfter int
	chatstate string
	reads     int
}

func (p *lateSubscribeDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		p.reads++
		if p.reads < p.flipAfter {
			*out = `{"stage":"done","ok":true,"why":"","subscribed":false}`
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","subscribed":true,"online":true,"chatstate":%q}`,
			p.chatstate)
		return nil
	}
	*out = `{"started":true}`
	return nil
}

// THE TYPING STATE COMES FROM THE LISTS THAT MOVE, not from a field that does
// not exist on this build.
//
// The reader used to take p.chatstate.type. Measured on the real page: that is
// undefined, the chatstates collection is empty, and the model instead carries
// typingUserIds and recordingUserIds. A reader of an undefined field does not
// even throw — it just answers "" forever, which is indistinguishable from a
// capability that was never wired (H94).
func TestTheTypingStateIsReadFromTheListsThatMove(t *testing.T) {
	script := observeScript("5541999999999@c.us")
	for _, want := range []string{"p.typingUserIds", "p.recordingUserIds"} {
		if !strings.Contains(script, want) {
			t.Errorf("the observe script does not read %s", want)
		}
	}
	// COMMENTS STRIPPED. The script explains, at length, why it no longer reads
	// p.chatstate.type — and a plain Contains finds the explanation. This
	// repository has now written a guard that matched its own prose EIGHT times;
	// the helper below is the same one capabilities/call needed, duplicated
	// because a test helper cannot cross packages without becoming production
	// code that nothing calls.
	if strings.Contains(withoutComments(script), "p.chatstate.type") {
		t.Error("the observe script reads p.chatstate.type, which is undefined on this build")
	}
	// COUNTS, NEVER THE IDS: those lists hold identities, and this is the one
	// place they could leak into a Snapshot that gets logged.
	if strings.Contains(script, "typingUserIds[0]") || strings.Contains(script, "typingUserIds.join") {
		t.Error("the observe script reads an identity out of the typing list")
	}
}

// withoutComments removes // line comments, so an assertion about what a page
// script DOES cannot be satisfied — or defeated — by what it SAYS.
//
// Deliberately naive: it does not understand "//" inside a string literal. No
// script in this package contains one, and a helper pretending to be a
// JavaScript parser would be worse to trust than a stated limit.
func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
