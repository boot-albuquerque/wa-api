package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/session"
	infrawa "wa-api/pkg/infra/whatsmeow"
	"wa-api/pkg/presentation/http/handlers"
	"wa-api/pkg/presentation/http/middleware"
)

// boundaryLogMsg is the literal the AccessHandler callback in router.go
// writes. Filtering on it is what separates the one boundary record per
// request from every other line the same buffer collects.
const boundaryLogMsg = "Got API Request"

const boundaryTestToken = "boundary-test-token"

// noSessionGuard satisfies appport.SessionGuard and always reports "no
// session". It exists so a real use case (session.GetStatusUseCase) runs and
// logs through the real ZerologAdapter during a router-driven request —
// without needing a live whatsmeow connection. Que ele seja trivial de
// escrever é o ponto da ADR-001: com a porta antiga, a mesma fake tinha que
// produzir um *whatsmeow.Client.
type noSessionGuard struct{}

func (noSessionGuard) EnsureSession(context.Context, string) error {
	return infrawa.ErrNoSession("boundary-test-user", nil)
}

// panicSessionGuard panics instead of returning, so a REAL panic originates
// inside a REAL handler running behind the full middleware stack — the only
// way to observe what the AccessHandler callback sees when a request dies.
type panicSessionGuard struct{}

const boundaryTestPanicMsg = "boundary-test-induced panic"

func (panicSessionGuard) EnsureSession(context.Context, string) error {
	panic(boundaryTestPanicMsg)
}

// boundaryDeps builds Deps whose Log writes JSON lines into buf, backed by
// the same real in-memory sqlite goldenTestDeps uses (a zero-value *sqlx.DB
// makes AuthAlice panic on db.Query instead of producing a clean 401).
//
// Session.GetStatus is wired with a REAL use case and a REAL ZerologAdapter
// over the same buf, so a use-case-level record and the boundary record for
// the same request land in one buffer and can be correlated by req_id.
func boundaryDeps(t *testing.T, buf *bytes.Buffer) Deps {
	t.Helper()

	d := goldenTestDeps(t)
	d.Log = zerolog.New(buf).With().Timestamp().Logger()

	ch := emptyCustomHandlers()
	ch.Session.GetStatus = handlers.NewGetStatusHandler(
		session.NewGetStatusUseCase(
			noSessionGuard{},
			infrawa.NewZerologAdapter(zerolog.New(buf).With().Timestamp().Logger()),
		),
	)
	d.CustomHandlers = ch
	return d
}

// boundaryPanicDeps is boundaryDeps with the GetStatus use case rigged to
// panic, so GET /session/status is a route that reliably dies mid-handler.
func boundaryPanicDeps(t *testing.T, buf *bytes.Buffer) Deps {
	t.Helper()

	d := boundaryDeps(t, buf)
	d.CustomHandlers.Session.GetStatus = handlers.NewGetStatusHandler(
		session.NewGetStatusUseCase(
			panicSessionGuard{},
			infrawa.NewZerologAdapter(zerolog.New(buf).With().Timestamp().Logger()),
		),
	)
	return d
}

// seedToken registers a valid token in the user cache, which is the branch
// AuthAlice takes before it ever queries the DB.
func seedToken(d Deps) {
	d.UserCache.Set(boundaryTestToken, middleware.NewValues(map[string]string{
		"Id":   "boundary-test-user",
		"Name": "boundary",
	}), cache.NoExpiration)
}

type logRecord map[string]any

func (r logRecord) str(key string) string {
	if v, ok := r[key].(string); ok {
		return v
	}
	return ""
}

// decodeRecords parses the buffer as newline-delimited JSON.
func decodeRecords(t *testing.T, buf *bytes.Buffer) []logRecord {
	t.Helper()
	var out []logRecord
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec logRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON (%v): %s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

func boundaryRecords(t *testing.T, buf *bytes.Buffer) []logRecord {
	t.Helper()
	var out []logRecord
	for _, rec := range decodeRecords(t, buf) {
		if rec.str("message") == boundaryLogMsg {
			out = append(out, rec)
		}
	}
	return out
}

// TestBoundaryLog pins the ambiguity decisions this phase had to make, in the
// subtest names themselves, as the plan requires:
//
//   - a 401 IS a request and MUST produce exactly one boundary record — it
//     is the primary signal for diagnosing auth problems, and before this
//     phase it produced nothing at all, because authAlice was the outermost
//     link in the chain and returned before hlog ever ran;
//   - an unmatched route (404) and a wrong-method request (405) ARE requests:
//     a scanner sweep across unknown paths is exactly the traffic an operator
//     needs to see, and logging a 401 on a known route while dropping a 404 on
//     an unknown one would be incoherent;
//   - an /admin/* request IS a request, and the most important one to log:
//     these are the highest-privilege operations in the system;
//   - a panicking request IS a server_error, not a "success" — it must carry
//     status 500 and correlate with the panic record by req_id;
//   - /livez is NOT a request for boundary-logging purposes.
//
// Each subtest is its own function so this one stays trivially simple.
func TestBoundaryLog(t *testing.T) {
	t.Run("401_counts_as_a_request", boundaryLog401CountsAsARequest)
	t.Run("authenticated_request_emits_exactly_one_record", boundaryLogAuthenticatedEmitsOneRecord)
	t.Run("req_id_correlates_boundary_and_usecase_log", boundaryLogReqIDCorrelates)
	t.Run("404_and_validation_do_not_emit_error_level", boundaryLog404AndValidationNotError)
	t.Run("unmatched_route_404_counts_as_a_request", boundaryLog404CountsAsARequest)
	t.Run("wrong_method_405_counts_as_a_request", boundaryLog405CountsAsARequest)
	t.Run("admin_route_counts_as_a_request", boundaryLogAdminCountsAsARequest)
	t.Run("panic_produces_server_error_outcome_with_req_id", boundaryLogPanicIsServerError)
	t.Run("livez_probe_is_excluded_from_boundary_logging", boundaryLogLivezExcluded)
}

func boundaryLog401CountsAsARequest(t *testing.T) {
	var buf bytes.Buffer
	router := NewRouter(boundaryDeps(t, &buf))

	req := httptest.NewRequest(http.MethodGet, "/session/status", nil)
	req.Header.Set("token", "definitely-not-a-valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	got := boundaryRecords(t, &buf)
	if len(got) != 1 {
		t.Fatalf("boundary records = %d, want exactly 1\nbuffer:\n%s", len(got), buf.String())
	}
	if status, _ := got[0]["status"].(float64); int(status) != http.StatusUnauthorized {
		t.Errorf("boundary record status = %v, want 401", got[0]["status"])
	}
	if got[0].str("outcome") != "client_error" {
		t.Errorf("outcome = %q, want %q", got[0].str("outcome"), "client_error")
	}
	if got[0].str("route") != "/session/status" {
		t.Errorf("route = %q, want %q", got[0].str("route"), "/session/status")
	}
}

func boundaryLogAuthenticatedEmitsOneRecord(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/session/status", nil)
	req.Header.Set("token", boundaryTestToken)
	router.ServeHTTP(httptest.NewRecorder(), req)

	got := boundaryRecords(t, &buf)
	if len(got) != 1 {
		t.Fatalf("boundary records = %d, want exactly 1\nbuffer:\n%s", len(got), buf.String())
	}
	if got[0].str("userid") != "boundary-test-user" {
		t.Errorf("userid = %q, want %q", got[0].str("userid"), "boundary-test-user")
	}
	if _, ok := got[0]["duration_ms"]; !ok {
		t.Error("boundary record is missing duration_ms")
	}
}

func boundaryLogReqIDCorrelates(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/session/status", nil)
	req.Header.Set("token", boundaryTestToken)
	router.ServeHTTP(httptest.NewRecorder(), req)

	var boundaryID, usecaseID string
	for _, rec := range decodeRecords(t, &buf) {
		switch rec.str("message") {
		case boundaryLogMsg:
			boundaryID = rec.str("req_id")
		case "no whatsmeow session": // emitted by session.GetStatusUseCase
			usecaseID = rec.str("req_id")
		}
	}
	if boundaryID == "" {
		t.Fatalf("boundary record has no req_id\nbuffer:\n%s", buf.String())
	}
	if usecaseID == "" {
		t.Fatalf("use case record has no req_id\nbuffer:\n%s", buf.String())
	}
	if boundaryID != usecaseID {
		t.Fatalf("req_id mismatch: boundary=%q usecase=%q", boundaryID, usecaseID)
	}
}

func boundaryLog404AndValidationNotError(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	// 404: no route matches at all.
	notFound := httptest.NewRequest(http.MethodGet, "/no/such/route", nil)
	notFound.Header.Set("token", boundaryTestToken)
	nfRec := httptest.NewRecorder()
	router.ServeHTTP(nfRec, notFound)
	if nfRec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want 404", nfRec.Code)
	}

	// 400: authenticated, matched route, malformed payload — rejected by the
	// handler's own validation before any use case runs.
	bad := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader("{not json"))
	bad.Header.Set("token", boundaryTestToken)
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("malformed payload status = %d, want 400", badRec.Code)
	}

	for _, rec := range decodeRecords(t, &buf) {
		if rec.str("level") == "error" {
			t.Errorf("a 404/validation request emitted an error-level record: %v", map[string]any(rec))
		}
	}
}

// boundaryLog404CountsAsARequest pins the DECISION (see router.go's
// NotFoundHandler wiring): an unmatched path emits exactly one boundary
// record, bucketed as client_error. The pre-existing
// 404_and_validation_do_not_emit_error_level subtest only asserted the
// ABSENCE of error records, so it passed whether 404 logging emitted one
// record, zero, or ten — this one pins the count in both directions.
func boundaryLog404CountsAsARequest(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/no/such/route", nil)
	req.Header.Set("token", boundaryTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertSingleClientErrorRecord(t, &buf, http.StatusNotFound)
}

// boundaryLog405CountsAsARequest is the wrong-method half of the same
// decision: /session/status exists but is registered GET-only, so a POST is a
// matched path with a mismatched method — mux routes it to
// MethodNotAllowedHandler, which router.go wraps in the same logging stack.
func boundaryLog405CountsAsARequest(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodPost, "/session/status", nil)
	req.Header.Set("token", boundaryTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	assertSingleClientErrorRecord(t, &buf, http.StatusMethodNotAllowed)
}

// assertSingleClientErrorRecord is the shared shape of both 404/405 checks:
// exactly one boundary record, the expected status, outcome client_error, and
// no error-level noise (these are client-side mistakes, not our incidents).
func assertSingleClientErrorRecord(t *testing.T, buf *bytes.Buffer, wantStatus int) {
	t.Helper()

	got := boundaryRecords(t, buf)
	if len(got) != 1 {
		t.Fatalf("boundary records = %d, want exactly 1\nbuffer:\n%s", len(got), buf.String())
	}
	if status, _ := got[0]["status"].(float64); int(status) != wantStatus {
		t.Errorf("boundary record status = %v, want %d", got[0]["status"], wantStatus)
	}
	if got[0].str("outcome") != "client_error" {
		t.Errorf("outcome = %q, want %q", got[0].str("outcome"), "client_error")
	}
	for _, rec := range decodeRecords(t, buf) {
		if rec.str("level") == "error" {
			t.Errorf("client-side request emitted an error-level record: %v", map[string]any(rec))
		}
	}
}

// boundaryLogAdminCountsAsARequest pins the DECISION (see router.go's
// adminRoutes wiring): /admin/* goes through the boundary-logging stack too,
// and the stack is registered BEFORE authAdmin so a rejected admin token still
// leaves a record — the admin equivalent of 401_counts_as_a_request.
func boundaryLogAdminCountsAsARequest(t *testing.T) {
	var buf bytes.Buffer
	router := NewRouter(boundaryDeps(t, &buf))

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "wrong-admin-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	got := boundaryRecords(t, &buf)
	if len(got) != 1 {
		t.Fatalf("boundary records for /admin/users = %d, want exactly 1\nbuffer:\n%s", len(got), buf.String())
	}
	if got[0].str("route") != "/admin/users" {
		t.Errorf("route = %q, want %q", got[0].str("route"), "/admin/users")
	}
	if got[0].str("outcome") != "client_error" {
		t.Errorf("outcome = %q, want %q", got[0].str("outcome"), "client_error")
	}
}

// boundaryLogPanicIsServerError is the regression test for the two defects
// innerRecoverMiddleware exists to fix: without an inner recover, the
// AccessHandler callback observed status 0 (so outcomeFor logged "success" for
// a request that returned 500), and the panic record carried no req_id (so it
// could not be tied back to the boundary record).
func boundaryLogPanicIsServerError(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryPanicDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/session/status", nil)
	req.Header.Set("token", boundaryTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500\nbuffer:\n%s", rec.Code, buf.String())
	}

	got := boundaryRecords(t, &buf)
	if len(got) != 1 {
		t.Fatalf("boundary records = %d, want exactly 1\nbuffer:\n%s", len(got), buf.String())
	}
	if status, _ := got[0]["status"].(float64); int(status) != http.StatusInternalServerError {
		t.Errorf("boundary record status = %v, want 500", got[0]["status"])
	}
	if got[0].str("outcome") != "server_error" {
		t.Errorf("outcome = %q, want %q (a panic must not read as a success)", got[0].str("outcome"), "server_error")
	}

	var panicID string
	for _, r := range decodeRecords(t, &buf) {
		if r.str("level") == "error" && strings.Contains(r.str("panic"), boundaryTestPanicMsg) {
			panicID = r.str("req_id")
		}
	}
	if panicID == "" {
		t.Fatalf("panic record has no req_id (it must log through hlog.FromRequest, not the global logger)\nbuffer:\n%s", buf.String())
	}
	if panicID != got[0].str("req_id") {
		t.Fatalf("req_id mismatch: panic=%q boundary=%q", panicID, got[0].str("req_id"))
	}
}

// boundaryLogLivezExcluded documents an accepted, explicit gap: /livez is a
// container liveness probe hit every few seconds by the orchestrator, so
// logging it would drown the boundary stream in noise without describing a
// single real API request — it is deliberately registered on a bare
// alice.New() outside the logging chain. Asserted here so it stays a
// decision rather than decaying into an unnoticed regression.
func boundaryLogLivezExcluded(t *testing.T) {
	var buf bytes.Buffer
	router := NewRouter(boundaryDeps(t, &buf))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/livez status = %d, want 200", rec.Code)
	}
	if got := boundaryRecords(t, &buf); len(got) != 0 {
		t.Fatalf("boundary records for /livez = %d, want 0\nbuffer:\n%s", len(got), buf.String())
	}
}

func TestLogLevel(t *testing.T) {
	t.Run("info_level_suppresses_debug_records", logLevelInfoSuppressesDebug)
}

func logLevelInfoSuppressesDebug(t *testing.T) {
	prev := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	var buf bytes.Buffer
	// Built exactly like the request-scoped logger the middleware chain
	// installs, so what is asserted is the real filtering path, not a
	// stand-in.
	base := zerolog.New(&buf).With().Timestamp().Logger()
	handler := hlog.NewHandler(base)(
		hlog.RequestIDHandler("req_id", "Request-Id")(
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				l := hlog.FromRequest(r)
				l.Debug().Msg("a debug record")
				l.Info().Msg("an info record")
			}),
		),
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/anything", nil))

	var debugs, infos int
	for _, rec := range decodeRecords(t, &buf) {
		switch rec.str("level") {
		case "debug":
			debugs++
		case "info":
			infos++
		}
	}
	if debugs != 0 {
		t.Errorf("debug records = %d, want 0 at info level\nbuffer:\n%s", debugs, buf.String())
	}
	if infos == 0 {
		t.Error("info records = 0 — the level filter swallowed everything, so the debug assertion proves nothing")
	}
}
