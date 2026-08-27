package bootstrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The 404 and the 405 are the only two responses on this API that no handler
// produces: mux answers them from NotFoundHandler / MethodNotAllowedHandler.
// That is exactly why they were the last two escapes from the envelope — they
// used to come out of http.NotFoundHandler and http.Error as `text/plain`
// with a bare sentence, so a client that mistyped a path got a JSON decode
// failure where every other rejection gives it `error.code`.
//
// These tests go through NewRouter, not through a hand-built mux: the wiring
// of the two handlers IS the thing under test, and a mux assembled in the test
// would prove nothing about the one the process serves.

func TestRouterUnmatchedRoute_AnswersCanonicalErrorEnvelope(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodGet, "/no/such/route", nil)
	req.Header.Set("token", boundaryTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertCanonicalErrorEnvelope(t, rec, http.StatusNotFound, "not_found")
}

func TestRouterWrongMethod_AnswersCanonicalErrorEnvelope(t *testing.T) {
	var buf bytes.Buffer
	d := boundaryDeps(t, &buf)
	seedToken(d)
	router := NewRouter(d)

	// /session/status exists and is registered GET-only, so a POST is a
	// matched path with a mismatched method.
	req := httptest.NewRequest(http.MethodPost, "/session/status", nil)
	req.Header.Set("token", boundaryTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertCanonicalErrorEnvelope(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
}

// assertCanonicalErrorEnvelope is the full envelope contract, asserted here
// and not delegated: `error` an OBJECT (never the plain sentence these two
// routes used to write), a code that matches, a non-empty message, success
// false, and the envelope's own `code` equal to the status actually written.
func assertCanonicalErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, wantStatus, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json — this is the regression that made "+
			"the two router-level rejections unparseable", ct)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body is not JSON (%v): %s", err, rec.Body.String())
	}
	assertEnvelopeShell(t, envelope, wantStatus, rec.Body.String())
	assertErrorObject(t, envelope["error"], wantCode)
}

// assertEnvelopeShell checks everything OUTSIDE the error object: success is a
// boolean and false, code is a number equal to the status actually written,
// and no data rode along on a rejection.
func assertEnvelopeShell(t *testing.T, envelope map[string]json.RawMessage, wantStatus int, raw string) {
	t.Helper()

	var success bool
	if err := json.Unmarshal(envelope["success"], &success); err != nil {
		t.Fatalf("success is not a boolean: %s", envelope["success"])
	}
	if success {
		t.Errorf("success = true on a %d", wantStatus)
	}

	var code int
	if err := json.Unmarshal(envelope["code"], &code); err != nil {
		t.Fatalf("code is not a number: %s", envelope["code"])
	}
	if code != wantStatus {
		t.Errorf("envelope.code = %d, want %d (the status actually written)", code, wantStatus)
	}

	if _, has := envelope["data"]; has {
		t.Errorf("an error response carried data: %s", raw)
	}
}

// assertErrorObject checks the `error` key itself: an OBJECT and never the
// plain sentence these two routes used to write, with the expected code and a
// non-empty message.
func assertErrorObject(t *testing.T, raw json.RawMessage, wantCode string) {
	t.Helper()

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		t.Fatalf("error came back as the STRING %q: the contract requires an object for every status", asString)
	}
	var errObj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &errObj); err != nil {
		t.Fatalf("error is not an object {code,message} (%v): %s", err, raw)
	}
	if errObj.Code != wantCode {
		t.Errorf("error.code = %q, want %q", errObj.Code, wantCode)
	}
	if errObj.Message == "" {
		t.Error("error.message is empty")
	}
}
