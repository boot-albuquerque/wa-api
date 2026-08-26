package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
)

// F268 — a field with the wrong name was dropped in silence, and because the
// zero value of the dropped field means "forever", the typo became a choice:
//
//	POST /chat/mute {"jid":"…","mute":true,"duration":"8h"} -> 200 "Chat muted"
//
// muted the chat indefinitely. These tests exercise the REGISTERED ROUTE, not
// the handler in isolation, and they measure the body verbatim as it was
// measured in the field.

const (
	// unknownFieldRoute is the route F268 was measured on.
	unknownFieldRoute = "/chat/mute"

	// unknownFieldBody is the body from the F268 measurement, verbatim: the
	// field is `mute_duration`, and `duration` is the typo.
	unknownFieldBody = `{"jid":"5511999999999@s.whatsapp.net","mute":true,"duration":"8h"}`

	// unknownFieldCleanBody is the same request with no unknown key: absent
	// `mute_duration` is the documented way to say "forever".
	unknownFieldCleanBody = `{"jid":"5511999999999@s.whatsapp.net","mute":true}`

	// unknownFieldName is the key that must appear, by name, wherever the
	// drop is reported.
	unknownFieldName = "duration"
)

// muteChatRouter builds the route exactly as the router registers it. Testing
// through the pattern and not the bare handler is what makes this exercise the
// real path (ARMADILHAS: "teste rota pela ROTA REGISTRADA").
func muteChatRouter(muter *contractsfake.ChatMuter) http.Handler {
	h := NewMuteChatHandler(chat.NewMuteChatUseCase(muter, &contractsfake.JIDResolver{}, silentLogger{}))
	r := mux.NewRouter()
	r.Handle(unknownFieldRoute, h).Methods(http.MethodPost)
	return r
}

// unknownFieldRecord finds the warning that names the dropped fields. It looks
// for the structured array rather than for text, so the assertion does not
// depend on the wording of the message.
func unknownFieldRecord(recs []logLine) (logLine, bool) {
	for _, rec := range recs {
		if rec.has(logFieldUnknownFields) {
			return rec, true
		}
	}
	return logLine{}, false
}

// Requirement 1: the misspelled field produces a warning that NAMES it.
//
// Naming is the whole requirement. A record saying only "unknown field" would
// leave whoever reads it exactly where the silence left them: knowing that
// something was dropped and not which thing.
func TestUnknownField_LenientMode_WarnsNamingTheField(t *testing.T) {
	muter := &contractsfake.ChatMuter{}
	rec, recs := ipmServe(t, muteChatRouter(muter), http.MethodPost, unknownFieldRoute,
		unknownFieldBody, func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: the default policy must not change "+
			"the contract, only make the drop visible", rec.Code)
	}

	got, ok := unknownFieldRecord(recs)
	if !ok {
		t.Fatalf("no log record carries %q: the field was dropped in SILENCE, "+
			"which is the defect itself", logFieldUnknownFields)
	}
	if lvl := got.str("level"); lvl != "warn" {
		t.Fatalf("level = %q, want warn: a dropped field is not narration", lvl)
	}
	if !strings.Contains(got.Raw, unknownFieldName) {
		t.Fatalf("record does not name %q: %s", unknownFieldName, got.Raw)
	}
	if msg := got.str("message"); !strings.Contains(msg, `"`+unknownFieldName+`"`) {
		t.Fatalf("message = %q, want it to contain %q — the name has to be "+
			"readable in the message, not only in a structured field",
			msg, `"`+unknownFieldName+`"`)
	}
	logassert.NoSecrets(t, recs)
}

// Requirement 3: with strict mode OFF — the default — the request that works
// today still works. This is what proves the contract did not change, and it
// is the reason the warning above is a warning and not a refusal.
func TestUnknownField_LenientMode_StillAcceptsTheRequest(t *testing.T) {
	muter := &contractsfake.ChatMuter{}
	rec, _ := ipmServe(t, muteChatRouter(muter), http.MethodPost, unknownFieldRoute,
		unknownFieldBody, func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: refusing by default would break every "+
			"client that sends an extra key today", rec.Code)
	}
	if len(muter.MuteChatCalls) != 1 {
		t.Fatalf("MuteChatCalls = %d, want 1: the request has to reach the port, "+
			"or 200 would be a lie", len(muter.MuteChatCalls))
	}
	// And the behaviour it produces is still the one F268 measured: forever.
	// Unchanged on purpose — the fix here is visibility, not semantics.
	if got := muter.MuteChatCalls[0].MuteDuration; got != 0 {
		t.Fatalf("MuteDuration = %v, want 0 (forever): lenient mode must not "+
			"alter what the dropped field produced", got)
	}
}

// Requirement 2: with strict mode ON, the same body is refused with 400 and a
// code of its own, naming the field.
func TestUnknownField_StrictMode_Rejects400NamingTheField(t *testing.T) {
	t.Setenv(EnvStrictUnknownFields, "true")

	muter := &contractsfake.ChatMuter{}
	rec, recs := ipmServe(t, muteChatRouter(muter), http.MethodPost, unknownFieldRoute,
		unknownFieldBody, func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(muter.MuteChatCalls) != 0 {
		t.Fatalf("MuteChatCalls = %d, want 0: a refused request must not reach "+
			"the port — answering 400 after muting the chat would be worse "+
			"than the defect", len(muter.MuteChatCalls))
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body is not the standard envelope (%v): %s", err, rec.Body.String())
	}
	if envelope.Error.Code != CodeUnknownField {
		t.Fatalf("error.code = %q, want %q: could_not_decode_payload would say "+
			"the body was unreadable, and it was not — it was readable and "+
			"carried a name this route does not know",
			envelope.Error.Code, CodeUnknownField)
	}
	if !strings.Contains(envelope.Error.Message, `"`+unknownFieldName+`"`) {
		t.Fatalf("error.message = %q, want it to name %q",
			envelope.Error.Message, unknownFieldName)
	}

	logassert.OutcomeLogged(t, recs, unknownFieldName)
}

// Requirement 4: a body with no unknown field produces no warning at all.
//
// Without this the log becomes noise, and a warning nobody reads is the same
// as no warning — which is the state F268 was measured in.
func TestUnknownField_CleanBody_ProducesNoWarning(t *testing.T) {
	muter := &contractsfake.ChatMuter{}
	rec, recs := ipmServe(t, muteChatRouter(muter), http.MethodPost, unknownFieldRoute,
		unknownFieldCleanBody, func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if _, ok := unknownFieldRecord(recs); ok {
		t.Fatalf("a clean body produced an unknown-field record: %v", recs)
	}
	assertNoOutcomeLog(t, recs)
}

// Strict mode must not refuse a clean body: the guard has to bite on the typo
// and on nothing else. Without this the previous test would pass with a
// mechanism that rejects everything.
func TestUnknownField_StrictMode_AcceptsCleanBody(t *testing.T) {
	t.Setenv(EnvStrictUnknownFields, "true")

	muter := &contractsfake.ChatMuter{}
	rec, _ := ipmServe(t, muteChatRouter(muter), http.MethodPost, unknownFieldRoute,
		unknownFieldCleanBody, func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: strict mode refuses UNKNOWN fields, "+
			"not requests", rec.Code)
	}
	if len(muter.MuteChatCalls) != 1 {
		t.Fatalf("MuteChatCalls = %d, want 1", len(muter.MuteChatCalls))
	}
}

// The switch is opt-in, and only the values the rest of the project treats as
// true turn it on. A typo in the variable must leave the lenient default, not
// silently enable a contract change.
func TestStrictUnknownFields_OptIn(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"true", true},
		{"TRUE", true},
		{" 1 ", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"ture", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(EnvStrictUnknownFields, tc.value)
			if got := strictUnknownFields(); got != tc.want {
				t.Fatalf("strictUnknownFields() with %s=%q = %v, want %v",
					EnvStrictUnknownFields, tc.value, got, tc.want)
			}
		})
	}
}
