package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"

	appport "wa-api/pkg/application/contracts"
)

// FIX-42 — wiring lock for `.WithStartSession` (wiring_handlers.go:410).
//
// Without this test, removing `.WithStartSession(s.startSession)` from
// initConnectHandler compiles, passes every other test, and only shows up when
// a user tries to connect in production — GET /session/connect responds
// 200 {"status":"connecting"} but nothing actually connects.
//
// The defect is the SIBLING of the poll-options wiring defect (F129/CAP-14):
// same mechanism (optional decorator silently dropped), same consequence
// (the route lies about what it did), different handler.

// connectWiringUser is the txtID used by the wiring tests in this file.
const connectWiringUser = "FIX42"

// connectWiringInvocationTimeout is how long the test waits for the
// fire-and-forget goroutine to signal. Generous enough for CI under load,
// tight enough to not waste time on a real failure.
const connectWiringInvocationTimeout = 2 * time.Second

// newConnectWiringRouter builds the production router from the handlers that
// initCustomHandlers constructs, then replaces the StartSession function on
// the ConnectHandler with a stub that signals invocation via a channel.
//
// Returns the router and a channel that receives the userID passed to
// StartSession. The channel is buffered (cap 1) so the fire-and-forget
// goroutine never blocks even if the test hasn't selected on it yet.
//
// customHandlerSet is a package global that initCustomHandlers overwrites;
// the test restores it on Cleanup so subsequent tests see no leftover state.
func newConnectWiringRouter(t *testing.T) (*mux.Router, <-chan string) {
	t.Helper()

	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	// initCustomHandlers stores s.startSession as a method value — it never
	// CALLS it during init, so a nil SessionOrchestrator is safe here.
	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	// Assert the wiring is in place BEFORE replacing it. This is the primary
	// assertion: if `.WithStartSession(s.startSession)` is removed from
	// initConnectHandler (wiring_handlers.go:410), StartSession is nil and
	// this line kills the test with a message that names the missing call.
	if customHandlerSet.Session.Connect.StartSession == nil {
		t.Fatal("StartSession is nil after initCustomHandlers — " +
			"the wiring in initConnectHandler (wiring_handlers.go:410) no longer calls " +
			".WithStartSession(s.startSession). Without it, GET /session/connect " +
			"responds 200 {\"status\":\"connecting\"} but no WhatsApp session starts: " +
			"the response lies.")
	}

	// Replace StartSession with a stub that signals invocation. This proves
	// that the handler actually CALLS StartSession during a real HTTP request,
	// not just that the field was set.
	invoked := make(chan string, 1)
	customHandlerSet.Session.Connect.StartSession = func(userID, token string) {
		invoked <- userID
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(connectWiringUser, 0))))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)
	return router, invoked
}

// TestStartSessionIsWiredIntoConnectHandler is the wiring lock.
//
// What it prevents: `.WithStartSession(s.startSession)` disappearing from
// wiring_handlers.go:410. With the call in place, a GET /session/connect
// through the registered route fires the session launcher in a goroutine.
// Without it, the handler responds 200 "connecting" and nothing connects —
// the first step of every session (pairing, reconnect, QR) silently fails.
func TestStartSessionIsWiredIntoConnectHandler(t *testing.T) {
	router, invoked := newConnectWiringRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/connect", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// The handler fires StartSession with `go` (handler_session.go:96), so
	// we wait on the channel with a timeout instead of checking immediately.
	select {
	case uid := <-invoked:
		if uid != connectWiringUser {
			t.Fatalf("StartSession received userID %q, want %q", uid, connectWiringUser)
		}
	case <-time.After(connectWiringInvocationTimeout):
		t.Fatal("StartSession was not invoked within the timeout — " +
			"the handler at GET /session/connect responded 200 but never called " +
			"StartSession. If .WithStartSession(s.startSession) is still in " +
			"wiring_handlers.go:410, then the handler itself stopped calling it " +
			"(check handler_session.go:91-96).")
	}
}
