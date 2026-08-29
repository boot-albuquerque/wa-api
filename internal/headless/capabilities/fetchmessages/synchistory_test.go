package fetchmessages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

type syncDouble struct {
	answer     string
	kicks      int
	lastScript string
}

func (d *syncDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+syncStateKey) {
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func syncer(d *syncDouble) *Fetcher { return New(engine.NewRunner(), d.eval) }

func compressSyncClock(t *testing.T) {
	t.Helper()
	ob, ot := syncBudget, syncTick
	syncBudget, syncTick = 200*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { syncBudget, syncTick = ob, ot })
}

// withoutComments strips // comments before a script is asserted against.
//
// The eleven previous times a guard in this repository matched its own comment
// are reason enough to bring it along rather than discover it here.
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

// A CHAT WITH NOTHING LEFT IS NOT AN ERROR. endOfHistoryTransferType != 0 means
// the phone has already sent everything, and reporting that as a failure would
// make "there is nothing to fetch" indistinguishable from a broken request.
func TestAChatWithNothingLeftIsNotAnError(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: `{"ok":true,"sent":false,"hasType":true,"type":1}`}
	got, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t")
	if err != nil {
		t.Fatalf("a chat with no history left must not be an error: %v", err)
	}
	if got.Requested {
		t.Fatal("the request was reported as sent for a chat past its transfer")
	}
	if got.TransferType != 1 {
		t.Fatalf("the transfer type was not carried: %#v", got)
	}
}

// ABSENT IS NOT ZERO. Six of 389 chats carry no endOfHistoryTransferType, and
// zero is precisely the value that means "ask". Merging them would report a
// request as possible for a conversation the page never described.
func TestAnAbsentTransferTypeIsNotZero(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: `{"ok":true,"sent":false,"hasType":false,"type":0}`}
	got, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t")
	if err != nil {
		t.Fatalf("SyncHistory: %v", err)
	}
	if got.HasTransferType {
		t.Fatal("an absent transfer type came back as present")
	}
	if got.Requested {
		t.Fatal("a request was made for a chat with no transfer type at all")
	}
	if !strings.Contains(got.String(), "hasType=false") {
		t.Fatalf("the rendering hides the distinction: %s", got)
	}
}

// THE GUARD RUNS BEFORE THE REQUEST. This is an ORDER rule, which passes every
// other test when inverted, and it is asserted on the script because the double
// supplies the outcome either way.
func TestTheGuardRunsBeforeTheRequest(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: `{"ok":true,"sent":true,"hasType":true,"type":0}`}
	if _, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t"); err != nil {
		t.Fatalf("SyncHistory: %v", err)
	}
	code := withoutComments(d.lastScript)
	iGuard := strings.Index(code, "t !== 0")
	iSend := strings.Index(code, "sendPeerDataOperationRequest")
	if iGuard < 0 || iSend < 0 {
		t.Fatalf("script shape changed (guard=%d send=%d)", iGuard, iSend)
	}
	if iGuard > iSend {
		t.Fatal("the request is issued before the transfer-type guard, so a chat " +
			"that has nothing left would still be asked")
	}
}

// IT ASKS THE PHONE, NOT THE LOCAL STORE. Reading the collection is what Fetch
// does, and a SyncHistory that did that would answer instantly and change
// nothing — the exact confusion that kept this row open.
func TestSyncHistoryAsksTheReferencesModule(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: `{"ok":true,"sent":true,"hasType":true,"type":0}`}
	if _, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t"); err != nil {
		t.Fatalf("SyncHistory: %v", err)
	}
	if !strings.Contains(d.lastScript, "WAWebSendNonMessageDataRequest") {
		t.Fatal("the script does not call the module that asks the phone for history")
	}
}

func TestAnUnknownChatCannotBeSynced(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: `{"ok":true,"notFound":true}`}
	if _, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("want ErrNoChat, got %v", err)
	}
}

func TestAnEmptyJIDNeverReachesThePageForASync(t *testing.T) {
	d := &syncDouble{answer: `{"ok":true}`}
	if _, err := syncer(d).SyncHistory(context.Background(), " ", "t"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("want ErrNoChat, got %v", err)
	}
	if d.kicks != 0 {
		t.Fatalf("an empty jid reached the page %d times", d.kicks)
	}
}

func TestARefusedSyncIsAnError(t *testing.T) {
	compressSyncClock(t)
	d := &syncDouble{answer: fmt.Sprintf(`{"ok":false,"why":%q}`, "BOOM")}
	if _, err := syncer(d).SyncHistory(context.Background(), "1@lid", "t"); !errors.Is(err, ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
}
