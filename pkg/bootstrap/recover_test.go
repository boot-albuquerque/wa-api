package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
)

// TestRecoverIsOutermost proves recover covers a panic reachable INSIDE
// authAlice — middleware/auth.go's AuthAlice calls db.Query on the *sql.DB
// it's given, which panics on a nil receiver. If recover were appended
// after auth (as alice.Chain would put it, since authAlice is the first
// append), this panic would propagate and crash the process instead of
// producing a 500 response. Deps.DB here is a zero-value *sqlx.DB, whose
// embedded *sql.DB is nil — the same shape as an unconfigured Deps.DB in
// production.
func TestRecoverIsOutermost(t *testing.T) {
	d := minimalDeps()
	d.DB = &sqlx.DB{} // embedded *sql.DB is nil: AuthAlice.db.Query panics
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/session/status", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (panic inside authAlice should be recovered into a 500, not propagate)", rec.Code, http.StatusInternalServerError)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("recovered response has an empty body — recoverMiddleware must still write a JSON envelope")
	}
}

// TestRecoverIsOutermost_CoversAdminRoutes proves recover wraps the /admin
// subrouter too, not just the alice chain `c` — its middleware is
// registered via mux.Router.Use on a subrouter, a separate mechanism from
// the alice.Chain that /admin does NOT go through.
func TestRecoverIsOutermost_CoversAdminRoutes(t *testing.T) {
	d := minimalDeps()
	d.CustomHandlers = emptyCustomHandlers()
	// UserHandlers.ListUsers() dereferences its unexported use-case field
	// when invoked — a nil *handlers.UserHandlers only avoided the panic in
	// TestNewRouter because ListUsers() itself was never called there with
	// a real request; here we drive an actual request through /admin/users.
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "wrong-token") // wrong token: passes authAdmin's comparison, reaches the handler
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// With the wrong admin token, authAdmin itself returns 401 before the
	// handler runs — this proves recover does NOT interfere with the
	// non-panicking path (a broader recover misconfiguration could
	// swallow legitimate error responses too).
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (wrong admin token should be rejected by authAdmin, unaffected by recover)", rec.Code, http.StatusUnauthorized)
	}
}
