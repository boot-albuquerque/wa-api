package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/pairing"
	"wa-api/pkg/presentation/http/handlers"
)

// F108 + F281 — wiring lock for the ownership pre-check on /session/connect.
//
// F108: without a SYNCHRONOUS ownership check, GET /session/connect answers
// 200 {"status":"connecting"} for a session owned by another replica, and
// nothing ever connects — the response lies.
//
// F281 moved that check from a closure injected into ConnectHandler
// (`.WithCheckOwnership(...)`) into appport.SessionStarter, resolved per engine
// by pkg/pairing. The protection is the same and the seam is different, so this
// file's two halves are:
//
//  1. the production wiring still reaches an ownership-checking starter for a
//     wa_noise session (TestConnectStarterIsWiredForWaNoise);
//  2. through the REGISTERED ROUTE, a denial becomes 409 and StartSession is
//     never called (TestConnectOwnershipCheckIsWired).
//
// Testing via the registered route rather than the handler directly is
// ARMADILHAS.md #2: a handler that works but is not mounted exercises nothing.

const ownershipWiringUser = "FIX108"

// TestConnectStarterIsWiredForWaNoise: the registry the production wiring
// builds must resolve a starter for a wa_noise session, and it must be the one
// that consults the lease manager.
//
// The type assertion is the point. `Starter != nil` would pass for any struct
// wired by mistake; the ownership check lives in *waNoiseSessionStarter
// specifically, and this is what stops a future refactor from wiring a
// pass-through that answers 200 without ever claiming a lease.
func TestConnectStarterIsWiredForWaNoise(t *testing.T) {
	s := &server{DB: newChatHistoryDB(t), ExPath: t.TempDir()}
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(_ context.Context, id string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: id, Engine: domain.EngineNoise}}, nil
		},
	}
	reg := buildPairingRegistry(s, users, nil, capabilityregistry.NewCapabilityRegistry())

	starter, err := reg.ResolveStarter(context.Background(), ownershipWiringUser, domain.EngineNoise)
	if err != nil {
		t.Fatalf("ResolveStarter for a wa_noise session: %v — production wiring no longer reaches a starter, "+
			"so GET /session/connect cannot connect anything (F273/F281)", err)
	}
	if _, ok := starter.(*waNoiseSessionStarter); !ok {
		t.Fatalf("starter is %T, want *waNoiseSessionStarter — the ownership pre-check lives there, and without "+
			"it GET /session/connect answers 200 {\"status\":\"connecting\"} when ownership is denied (F108)", starter)
	}
}

// TestConnectOwnershipCheckIsWired: a denial reaches the client as 409 through
// the registered route, and StartSession is never called.
func TestConnectOwnershipCheckIsWired(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	denying := &contractsfake.SessionStarter{
		CheckOwnershipFunc: func(context.Context, string) error {
			return apperr.New(codeSessionOwnedByAnotherReplica, apperr.CategoryConflict,
				msgSessionOwnedByAnotherReplica, false, nil)
		},
		StartSessionFunc: func(context.Context, string, string) {
			t.Fatal("StartSession must not be called when ownership is denied")
		},
	}
	customHandlerSet.Session.Connect = handlers.NewConnectHandler(
		session.NewConnectUseCase(&contractsfake.Logger{}), starterRegistry(denying))

	rec := serveConnect(t, http.MethodGet, "/session/connect?engine=noise")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 — ownership denied must reach the HTTP "+
			"boundary through the registered route, not be swallowed behind a 200 "+
			"(body: %s)", rec.Code, rec.Body.String())
	}
	if len(denying.StartSessionCalls) != 0 {
		t.Fatalf("StartSession called %d time(s) with ownership denied — the ORDER is the invariant (F108)",
			len(denying.StartSessionCalls))
	}
}

// TestConnectOwnershipCheckGranted_200: when the check passes through the
// registered route, the handler proceeds normally.
func TestConnectOwnershipCheckGranted_200(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	granting := &contractsfake.SessionStarter{}
	customHandlerSet.Session.Connect = handlers.NewConnectHandler(
		session.NewConnectUseCase(&contractsfake.Logger{}), starterRegistry(granting))

	rec := serveConnect(t, http.MethodGet, "/session/connect?engine=noise")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(granting.StartSessionCalls) != 1 {
		t.Fatalf("StartSession calls = %d, want 1", len(granting.StartSessionCalls))
	}
}

// TestConnectRouteRequiresEngine: the registered route refuses a request with
// no engine, before any starter is reached.
//
// This is the wiring half of the invalid_engine contract: the handler-level
// test lives in pkg/presentation/http/handlers, and this one proves the route
// as MOUNTED behaves the same — the two are different questions, and F81 is the
// entry in ARMADILHAS.md that says so.
func TestConnectRouteRequiresEngine(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	starter := &contractsfake.SessionStarter{}
	customHandlerSet.Session.Connect = handlers.NewConnectHandler(
		session.NewConnectUseCase(&contractsfake.Logger{}), starterRegistry(starter))

	rec := serveConnect(t, http.MethodGet, "/session/connect")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 invalid_engine (body: %s)", rec.Code, rec.Body.String())
	}
	if len(starter.CheckOwnershipCalls) != 0 || len(starter.StartSessionCalls) != 0 {
		t.Fatalf("a request with no engine reached the starter: %+v / %+v",
			starter.CheckOwnershipCalls, starter.StartSessionCalls)
	}
}

// starterRegistry builds a pairing registry whose wa_noise provider is the
// given starter, over a session recorded as wa_noise. The capability matrix is
// the PRODUCTION one — a permissive stand-in would bless paths that do not
// exist (ARMADILHAS.md #1).
func starterRegistry(starter appport.SessionStarter) *pairing.Registry {
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(_ context.Context, id string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: id, Engine: domain.EngineNoise}}, nil
		},
	}
	return pairing.NewRegistry(users, capabilityregistry.NewCapabilityRegistry(),
		&pairing.Provider{Engine: domain.EngineNoise, Starter: starter},
		&pairing.Provider{Engine: domain.EngineWaHeadless})
}

// serveConnect drives the request through the REGISTERED routes with the
// authenticated user injected the way the auth middleware does.
func serveConnect(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(ownershipWiringUser, 0))))
		})
	}
	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}
