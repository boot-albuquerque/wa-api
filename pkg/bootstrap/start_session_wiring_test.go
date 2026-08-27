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
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/presentation/http/handlers"
)

// FIX-42 (+ F281) — wiring lock for the session launcher on /session/connect.
//
// Without this test, dropping the launcher from the production wiring compiles,
// passes every other test, and only shows up when a user tries to connect in
// production — GET /session/connect responds 200 {"status":"connecting"} but
// nothing actually connects.
//
// The SEAM moved with the F281: it used to be `.WithStartSession(...)` on the
// ConnectHandler, and is now appport.SessionStarter, resolved per engine by
// pkg/pairing (see pkg/bootstrap/pairing_providers.go). The protection is
// unchanged — the route must CALL the launcher, not merely have one — and the
// substitution below happens at the provider instead of at the handler field.
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

	// The "is the wiring in place" half now lives in
	// TestConnectStarterIsWiredForWaNoise (connect_ownership_wiring_test.go),
	// which asserts the PRODUCTION registry resolves a *waNoiseSessionStarter.
	// What this file measures is the other half, and the one a nil-check can
	// never give: that a real HTTP request through the registered route
	// actually CALLS the launcher.
	invoked := make(chan string, 1)
	customHandlerSet.Session.Connect = handlers.NewConnectHandler(
		session.NewConnectUseCase(&contractsfake.Logger{}),
		starterRegistry(&contractsfake.SessionStarter{
			StartSessionFunc: func(_ context.Context, txtID, _ string) { invoked <- txtID },
		}))

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
	// `engine` e' obrigatorio desde a F281: sem ele a rota responde 400
	// invalid_engine antes de tocar em provider nenhum.
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/connect?engine=wa_noise", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// The wa-noise provider fires StartSession with `go`
	// (pkg/bootstrap/pairing_providers.go), so the wait stays even though this
	// double is synchronous: the production path is the one being described.
	select {
	case uid := <-invoked:
		if uid != connectWiringUser {
			t.Fatalf("StartSession received userID %q, want %q", uid, connectWiringUser)
		}
	case <-time.After(connectWiringInvocationTimeout):
		t.Fatal("StartSession was not invoked within the timeout — " +
			"the handler at GET /session/connect responded 200 but never called " +
			"the launcher resolved from the pairing registry (check " +
			"handler_session.go ConnectHandler.ServeHTTP and pkg/pairing).")
	}
}
