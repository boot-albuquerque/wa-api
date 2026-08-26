package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"
)

// errors.go and user_context.go are the two glue files in this package:
// the boundary-error vocabulary that ~30 handlers respond with, and the
// minimal shape they require from the middleware's userinfo. Neither has
// an exit path of its own — they are tested directly.

// TestSentinelErrors_CarryCause: every sentinel must say what is missing.
// A sentinel whose Error() is empty loses the cause in the exit-path log
// on the most frequent 401/400 branches of the package.
func TestSentinelErrors_CarryCause(t *testing.T) {
	sentinels := map[string]error{
		"errUnauthorized":     errUnauthorized,
		"errMissingSessionID": errMissingSessionID,
		"errMissingID":        errMissingID,
		"errDecodePayload":    errDecodePayload,
		"errMissingJID":       errMissingJID,
	}

	seen := make(map[string]string, len(sentinels))
	for name, err := range sentinels {
		if err == nil {
			t.Fatalf("%s is nil", name)
		}
		msg := err.Error()
		if msg == "" {
			t.Fatalf("%s.Error() is empty", name)
		}
		if other, dup := seen[msg]; dup {
			t.Fatalf("%s and %s share the same cause (%q) — indistinguishable in logs", name, other, msg)
		}
		seen[msg] = name
	}
}

// TestSentinelErrors_AreDistinctIdentities: sentinels are compared by
// identity (errors.Is) in handlers and envelope contract tests. Two
// distinct sentinels must never match each other.
func TestSentinelErrors_AreDistinctIdentities(t *testing.T) {
	all := []error{
		errUnauthorized,
		errMissingSessionID,
		errMissingID,
		errDecodePayload,
		errMissingJID,
	}
	for i, a := range all {
		if !errors.Is(a, a) {
			t.Fatalf("errors.Is does not recognise sentinel %d", i)
		}
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Fatalf("sentinels %d and %d match in errors.Is", i, j)
			}
		}
	}
}

// TestSentinelErrors_AreAppError: after F236, every boundary sentinel is
// an *apperr.AppError, so RespondJSON produces the structured envelope.
// This test locks that property — reverting to simpleErr would silently
// reintroduce the two-format split.
func TestSentinelErrors_AreAppError(t *testing.T) {
	sentinels := map[string]error{
		"errUnauthorized":     errUnauthorized,
		"errMissingSessionID": errMissingSessionID,
		"errMissingID":        errMissingID,
		"errDecodePayload":    errDecodePayload,
		"errMissingJID":       errMissingJID,
	}
	for name, err := range sentinels {
		var ae *apperr.AppError
		if !errors.As(err, &ae) {
			t.Fatalf("%s is not *apperr.AppError", name)
		}
		if ae.Code == "" {
			t.Fatalf("%s has empty Code", name)
		}
		if ae.Message == "" {
			t.Fatalf("%s has empty Message", name)
		}
	}
}

// sentinelEnvelopeCase describes a sentinel, its expected HTTP status and
// the apperr code the structured envelope must carry.
type sentinelEnvelopeCase struct {
	name       string
	sentinel   error
	wantStatus int
	wantCode   string
	wantMsg    string
}

func sentinelEnvelopeCases() []sentinelEnvelopeCase {
	return []sentinelEnvelopeCase{
		{"unauthorized", errUnauthorized, http.StatusUnauthorized, "unauthorized", "unauthorized"},
		{"missing_session_id", errMissingSessionID, http.StatusBadRequest, "missing_session_id", "missing session id"},
		{"missing_id", errMissingID, http.StatusBadRequest, "missing_id", "missing ID"},
		{"decode_payload_failed", errDecodePayload, http.StatusBadRequest, "decode_payload_failed", "could not decode payload"},
		{"missing_jid", errMissingJID, http.StatusBadRequest, "missing_jid", "missing jid in path"},
	}
}

// TestSentinelErrors_StructuredEnvelope: each sentinel, when passed to
// RespondJSON, must produce the structured object envelope — never the
// generic string that the pre-F236 simpleErr produced. The HTTP status
// MUST match the pre-F236 value (401 for unauthorized, 400 for the rest).
func TestSentinelErrors_StructuredEnvelope(t *testing.T) {
	type errorObj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type envelope struct {
		Code    int             `json:"code"`
		Success bool            `json:"success"`
		Error   json.RawMessage `json:"error"`
	}

	for _, tc := range sentinelEnvelopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			customhttp.RespondJSON(rec, 999, nil, tc.sentinel)

			if rec.Code != tc.wantStatus {
				t.Fatalf("HTTP status: got %d, want %d", rec.Code, tc.wantStatus)
			}

			var env envelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if env.Code != tc.wantStatus {
				t.Fatalf("envelope.code: got %d, want %d", env.Code, tc.wantStatus)
			}
			if env.Success {
				t.Fatal("envelope.success must be false for errors")
			}

			// The error field MUST be a structured object, not a string.
			var errStr string
			if json.Unmarshal(env.Error, &errStr) == nil {
				t.Fatalf("envelope.error is a bare string %q — the two-format split from F236 is back", errStr)
			}

			var errObj errorObj
			if err := json.Unmarshal(env.Error, &errObj); err != nil {
				t.Fatalf("envelope.error is not a structured object: %s", env.Error)
			}
			if errObj.Code != tc.wantCode {
				t.Fatalf("error.code: got %q, want %q", errObj.Code, tc.wantCode)
			}
			if errObj.Message != tc.wantMsg {
				t.Fatalf("error.message: got %q, want %q", errObj.Message, tc.wantMsg)
			}
		})
	}
}

// TestSentinelErrors_CategoryMapsToCorrectStatus: the Category on each
// sentinel must map to the same HTTP status that the pre-F236 handlers
// hard-coded. This is the assertion that prevents the fix from
// reclassifying status codes by accident.
//
// Uses errors.As (not concrete *apperr.AppError) so that reverting a
// sentinel to a non-apperr type compiles and fails with a message
// instead of a build error (ARMADILHAS #3).
func TestSentinelErrors_CategoryMapsToCorrectStatus(t *testing.T) {
	cases := []struct {
		name       string
		sentinel   error
		wantStatus int
	}{
		{"unauthorized", errUnauthorized, http.StatusUnauthorized},
		{"missing_session_id", errMissingSessionID, http.StatusBadRequest},
		{"missing_id", errMissingID, http.StatusBadRequest},
		{"decode_payload_failed", errDecodePayload, http.StatusBadRequest},
		{"missing_jid", errMissingJID, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ae *apperr.AppError
			if !errors.As(tc.sentinel, &ae) {
				t.Fatalf("%s is not *apperr.AppError — reclassification impossible to verify", tc.name)
			}
			got := ae.Category.HTTPStatus()
			if got != tc.wantStatus {
				t.Fatalf("Category.HTTPStatus(): got %d, want %d — reclassification detected", got, tc.wantStatus)
			}
		})
	}
}

// TestExistingApperrErrors_StillStructured: a pre-F236 apperr (like the
// ones the F224 introduced) must still produce the same structured
// envelope. This confirms the fix did not regress the already-converted
// errors.
func TestExistingApperrErrors_StillStructured(t *testing.T) {
	existing := apperr.New("missing_chat", apperr.CategoryValidation, "missing chat in payload", false, nil)
	rec := httptest.NewRecorder()
	customhttp.RespondJSON(rec, 999, nil, existing)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("HTTP status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	type errorObj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type envelope struct {
		Error json.RawMessage `json:"error"`
	}
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	var errObj errorObj
	if err := json.Unmarshal(env.Error, &errObj); err != nil {
		t.Fatalf("envelope.error is not structured: %s", env.Error)
	}
	if errObj.Code != "missing_chat" {
		t.Fatalf("error.code: got %q, want %q", errObj.Code, "missing_chat")
	}
	if errObj.Message != "missing chat in payload" {
		t.Fatalf("error.message: got %q, want %q", errObj.Message, "missing chat in payload")
	}
}

// stubUserInfo is the minimal userInfo implementation — proof that the
// package's local interface requires nothing from pkg/bootstrap.
type stubUserInfo map[string]string

func (s stubUserInfo) Get(key string) string { return s[key] }

// TestUserInfo_MinimalContract: the interface requires exactly one method,
// and a missing key returns empty (which handlers treat as "no session"),
// not a panic.
func TestUserInfo_MinimalContract(t *testing.T) {
	var info userInfo = stubUserInfo{"Id": "42", "Token": "tok-42"}

	if got := info.Get("Id"); got != "42" {
		t.Fatalf("Get(\"Id\") = %q", got)
	}
	if got := info.Get("Token"); got != "tok-42" {
		t.Fatalf("Get(\"Token\") = %q", got)
	}
	if got := info.Get("Missing"); got != "" {
		t.Fatalf("missing key returned %q, expected empty", got)
	}
}
