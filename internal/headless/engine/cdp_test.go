package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeBrowser is a CDP endpoint that imitates the REAL rule, not a friendlier
// one. The rules it enforces come from the protocol as Chromium implements it:
//
//   - a command is a JSON object carrying a non-zero "id" and a "method";
//   - the reply MUST echo that same id, because replies and events share the
//     socket and correlation is the only thing that tells them apart.
//
// A double that answers any frame with any id would let a client that ignores
// correlation pass — and ignoring correlation is precisely how the study once
// read a stale reply as confirmation.
type fakeBrowser struct {
	srv      *httptest.Server
	received chan cdpMessage
}

// action is what the fake does with a command it receives. Three outcomes,
// because the real browser has three: it answers, it stays silent, or — for
// Browser.close specifically — it answers and then drops the socket, and a
// dropped socket with no reply is the case a naive client hangs on forever.
type action int

const (
	actionReply action = iota
	actionSilent
	actionDrop
)

// reply describes what the fake does with a command it receives.
type replyFunc func(cmd cdpMessage) (cdpMessage, action)

func startFakeBrowser(t *testing.T, reply replyFunc) *fakeBrowser {
	t.Helper()
	f := &fakeBrowser{received: make(chan cdpMessage, 8)}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		for {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var m cdpMessage
			if err := json.Unmarshal(data, &m); err != nil {
				return
			}
			if m.ID == 0 || m.Method == "" {
				return // Chromium would reject; so does the double
			}
			f.received <- m
			res, act := reply(m)
			switch act {
			case actionSilent:
				continue
			case actionDrop:
				_ = c.CloseNow()
				return
			}
			b, _ := json.Marshal(res)
			if err := c.Write(r.Context(), websocket.MessageText, b); err != nil {
				return
			}
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBrowser) wsURL() string {
	return "ws" + strings.TrimPrefix(f.srv.URL, "http")
}

// echoOK is the well-behaved browser: it acknowledges the command it was sent.
func echoOK(cmd cdpMessage) (cdpMessage, action) {
	return cdpMessage{ID: cmd.ID, Result: json.RawMessage(`{}`)}, actionReply
}

func TestCloseBrowserSendsBrowserClose(t *testing.T) {
	f := startFakeBrowser(t, echoOK)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := CloseBrowserViaCDP(ctx, f.wsURL()); err != nil {
		t.Fatalf("CloseBrowserViaCDP: %v", err)
	}

	select {
	case got := <-f.received:
		if got.Method != methodBrowserClose {
			t.Fatalf("sent %q, want %q", got.Method, methodBrowserClose)
		}
		if got.ID == 0 {
			t.Fatal("command sent with id 0; Chromium cannot correlate a reply to it")
		}
	default:
		t.Fatal("nothing reached the browser")
	}
}

// The correlation must be real. A client that returns on the first frame it
// sees would pass every other test here and would, against a live browser,
// read an unrelated reply as confirmation of the shutdown.
func TestCallIgnoresRepliesWithAnotherID(t *testing.T) {
	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		return cdpMessage{ID: cmd.ID + 1000, Result: json.RawMessage(`{}`)}, actionReply
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := CloseBrowserViaCDP(ctx, f.wsURL())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the call to keep waiting: a reply with another id "+
			"is not an answer to this command", err)
	}
}

func TestCallSurfacesACDPRejection(t *testing.T) {
	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		return cdpMessage{ID: cmd.ID, Error: &cdpError{Code: -32000, Message: "not allowed"}}, actionReply
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := CloseBrowserViaCDP(ctx, f.wsURL())
	if err == nil {
		t.Fatal("a rejected command was reported as success")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error lost the browser's reason: %v", err)
	}
}

// The connection has no clock of its own: the caller's budget is the only
// budget. A fixed internal timeout would be a deadline the call site cannot
// see, which is the shape of defect the DeadlinePolicy exists to remove.
func TestCallIsBoundedByTheCallersContext(t *testing.T) {
	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		return cdpMessage{}, actionSilent
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := CloseBrowserViaCDP(ctx, f.wsURL())
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("returned after %v; the caller's budget was 200ms", elapsed)
	}
}

// A browser that dies mid-command must not leave the caller waiting for a reply
// that will never come. This is the ordinary case for Browser.close, and it is
// why the caller decides the outcome by watching the process, not this error.
func TestCallFailsWhenTheConnectionDropsBeforeTheReply(t *testing.T) {
	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		return cdpMessage{}, actionDrop // the browser is going down; no reply comes
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- CloseBrowserViaCDP(ctx, f.wsURL()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a lost connection was reported as a completed shutdown")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call never returned after the connection dropped")
	}
}

func TestDialFailureIsReported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 0 is never listening.
	if err := CloseBrowserViaCDP(ctx, "ws://127.0.0.1:0/devtools/browser/x"); err == nil {
		t.Fatal("dialling a dead endpoint was reported as success")
	}
}
