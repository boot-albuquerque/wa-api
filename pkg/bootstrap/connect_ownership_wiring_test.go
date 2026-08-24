package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
)

// F108 — wiring lock for `.WithCheckOwnership` on /session/connect.
//
// Without this test, removing `.WithCheckOwnership(...)` from
// initConnectHandler compiles, passes every other test, and only shows up
// when a user tries to connect in multi-pod mode — GET /session/connect
// responds 200 {"status":"connecting"} for a session owned by another
// replica, and nothing ever connects.

const ownershipWiringUser = "FIX108"

// TestConnectOwnershipCheckIsWired: the production router must call
// CheckOwnership on the ConnectHandler. Tested via the REGISTERED ROUTE,
// not the handler directly — a handler that works but is not mounted on the
// route exercises nothing (ARMADILHAS.md #2).
func TestConnectOwnershipCheckIsWired(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	if customHandlerSet.Session.Connect.CheckOwnership == nil {
		t.Fatal("CheckOwnership is nil after initCustomHandlers — " +
			"the wiring in initConnectHandler (wiring_handlers.go) no longer calls " +
			".WithCheckOwnership(...). Without it, GET /session/connect responds " +
			"200 {\"status\":\"connecting\"} when ownership is denied: the response lies (F108).")
	}

	// Replace with a stub that always denies.
	customHandlerSet.Session.Connect.CheckOwnership = func(string) error {
		return apperr.New(
			"session_owned_by_another_replica",
			apperr.CategoryConflict,
			"this session is owned by another replica; route the request to its owner",
			false,
			nil,
		)
	}
	customHandlerSet.Session.Connect.StartSession = func(string, string) {
		t.Fatal("StartSession must not be called when ownership is denied")
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(ownershipWiringUser, 0))))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/connect", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 — ownership denied must reach the HTTP "+
			"boundary through the registered route, not be swallowed behind a 200 "+
			"(body: %s)", rec.Code, rec.Body.String())
	}
}

// TestConnectOwnershipCheckGranted_200: when CheckOwnership passes through
// the registered route, the handler proceeds normally.
func TestConnectOwnershipCheckGranted_200(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	customHandlerSet.Session.Connect.CheckOwnership = func(string) error { return nil }
	customHandlerSet.Session.Connect.StartSession = func(string, string) {}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(ownershipWiringUser, 0))))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/connect", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}
