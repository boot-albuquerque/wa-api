package handlers

import (
	"errors"
	"testing"

	"wa-api/pkg/domain/apperr"
)

// TestSentinelErrors_CarryCause: each sentinel must say what went wrong. A
// sentinel with empty Error() loses the cause in the exit-path log on the
// most frequent 401/400 branches of the package.
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
			t.Fatalf("%s and %s have the same cause (%q) — indistinguishable in the log", name, other, msg)
		}
		seen[msg] = name
	}
}

// TestSentinelErrors_AreDistinctIdentities: sentinels are compared by
// identity (errors.Is) in handlers and in envelope contract tests. Two
// distinct codes must never match.
func TestSentinelErrors_AreDistinctIdentities(t *testing.T) {
	if !errors.Is(errUnauthorized, errUnauthorized) {
		t.Fatal("errors.Is does not recognise the sentinel itself")
	}
	if errors.Is(errUnauthorized, errMissingSessionID) {
		t.Fatal("distinct sentinels match in errors.Is")
	}
	if errors.Is(errDecodePayload, errMissingID) {
		t.Fatal("distinct sentinels match in errors.Is")
	}
}

// TestSentinelErrors_AreAppErrors: after F236, every sentinel is an
// *apperr.AppError with a non-empty Code and a Category whose HTTPStatus
// matches the status the callers historically pass. This is the contract
// that lets 244 call sites keep working unchanged.
func TestSentinelErrors_AreAppErrors(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantCode   string
		wantStatus int
	}{
		{"errUnauthorized", errUnauthorized, CodeUnauthorized, 401},
		{"errMissingSessionID", errMissingSessionID, CodeMissingSessionID, 400},
		{"errMissingID", errMissingID, CodeMissingID, 400},
		{"errDecodePayload", errDecodePayload, CodeDecodePayload, 400},
		{"errMissingJID", errMissingJID, CodeMissingJID, 400},
	}
	for _, tc := range cases {
		var appErr *apperr.AppError
		if !errors.As(tc.err, &appErr) {
			t.Fatalf("%s is not an *apperr.AppError", tc.name)
		}
		if appErr.Code != tc.wantCode {
			t.Fatalf("%s.Code = %q, want %q", tc.name, appErr.Code, tc.wantCode)
		}
		if got := appErr.Category.HTTPStatus(); got != tc.wantStatus {
			t.Fatalf("%s.Category.HTTPStatus() = %d, want %d", tc.name, got, tc.wantStatus)
		}
	}
}

// stubUserInfo is the minimal userInfo implementation — proof that the
// package-local interface requires nothing from pkg/bootstrap.
type stubUserInfo map[string]string

func (s stubUserInfo) Get(key string) string { return s[key] }

// TestUserInfo_MinimalContract: the interface asks for exactly one method,
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
