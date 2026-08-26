package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// F268 — the defect measured in the field:
//
//	POST /chat/mute {"jid":"…","mute":true,"duration":"8h"} -> 200 "Chat muted"
//
// `duration` is not a field of MuteChatRequest (`mute_duration` is), so Go
// dropped it, MuteDuration stayed nil, nil became 0, and 0 means FOREVER.
// These tests lock the detection that makes the drop visible.
//
// The type under test is the REAL MuteChatRequest, not a stand-in: the rule
// being measured is how encoding/json matches keys to fields, and a simplified
// double would match differently — the embedded ChatTarget and the
// case-insensitive matching are exactly what a hand-written double gets wrong.

// muteBodyWithTypo is the body measured in F268, verbatim.
const muteBodyWithTypo = `{"jid":"5511999999999@s.whatsapp.net","mute":true,"duration":"8h"}`

// muteBodyClean is the same request written correctly.
const muteBodyClean = `{"jid":"5511999999999@s.whatsapp.net","mute":true,"mute_duration":28800000000000}`

func TestDecodeRequestWithUnknownFields_NamesTheMisspelledField(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(strings.NewReader(muteBodyWithTypo), &req)
	if err != nil {
		t.Fatalf("decode error = %v, want nil: the body is valid JSON and the "+
			"detection must not turn an accepted request into a rejected one", err)
	}
	if len(unknown) != 1 || unknown[0] != "duration" {
		t.Fatalf("unknown = %v, want [duration]: the field that was silently "+
			"dropped has to be NAMED, or the report is undiagnosable", unknown)
	}
	// The decoded value must be identical to what DecodeRequest produced
	// before: detection only.
	if req.Jid != "5511999999999@s.whatsapp.net" || !req.Mute || req.MuteDuration != nil {
		t.Fatalf("decoded = %+v, want jid/mute set and MuteDuration nil", req)
	}
}

func TestDecodeRequestWithUnknownFields_CleanBodyReportsNothing(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(strings.NewReader(muteBodyClean), &req)
	if err != nil {
		t.Fatalf("decode error = %v, want nil", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none: a body whose every key is a field "+
			"of the request must produce no report at all, or the report is "+
			"noise and stops being read", unknown)
	}
	if req.MuteDuration == nil || *req.MuteDuration != 8*time.Hour {
		t.Fatalf("MuteDuration = %v, want 8h", req.MuteDuration)
	}
}

// The "chat" alias lives in the EMBEDDED ChatTarget. A detector that walked
// only the outer struct's own fields would call it unknown — and in strict
// mode that would refuse a request the API documents as valid.
func TestDecodeRequestWithUnknownFields_EmbeddedFieldIsKnown(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(
		strings.NewReader(`{"chat":"5511999999999@s.whatsapp.net","mute":true}`), &req)
	if err != nil {
		t.Fatalf("decode error = %v, want nil", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none: `chat` is a field of the embedded "+
			"ChatTarget", unknown)
	}
	if req.Jid != "5511999999999@s.whatsapp.net" {
		t.Fatalf("Jid = %q, want the alias resolved: DecodeRequestWithUnknownFields "+
			"must run ResolveChat exactly as DecodeRequest does", req.Jid)
	}
}

// Go matches JSON keys to struct fields IGNORING case. A key the real decoder
// accepts must not be reported as unknown, or strict mode refuses working
// requests. This is the direction of error that would hurt most.
func TestDecodeRequestWithUnknownFields_CaseInsensitiveMatchIsKnown(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(
		strings.NewReader(`{"JID":"5511999999999@s.whatsapp.net","Mute":true,"MUTE_DURATION":28800000000000}`), &req)
	if err != nil {
		t.Fatalf("decode error = %v, want nil", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none: encoding/json matches keys to "+
			"fields case-insensitively, so these three ARE read", unknown)
	}
	if req.MuteDuration == nil || *req.MuteDuration != 8*time.Hour {
		t.Fatalf("MuteDuration = %v, want 8h — proof the keys were really read", req.MuteDuration)
	}
}

// A key that exists but carries the wrong type is a DECODE error, not an
// unknown field. The two come out of the same decoder and must not be
// confused: F268 measured `mute_duration:"8h"` answering 400
// could_not_decode_payload, and that answer has to stay.
func TestDecodeRequestWithUnknownFields_WrongTypeIsDecodeErrorNotUnknown(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(
		strings.NewReader(`{"jid":"x@s.whatsapp.net","mute":true,"mute_duration":"8h"}`), &req)
	if err == nil {
		t.Fatal("decode error = nil, want a type error: `mute_duration` is a " +
			"time.Duration and \"8h\" is a string")
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none: the field exists, its value does not fit", unknown)
	}
}

// Several unknown keys must all be named, in a stable order: map iteration in
// Go is random by design, and a report that reorders itself between two
// identical requests cannot be diffed.
func TestDecodeRequestWithUnknownFields_AllNamesSortedAndStable(t *testing.T) {
	const body = `{"jid":"x@s.whatsapp.net","zebra":1,"alpha":2,"mid":3}`
	for attempt := 0; attempt < 20; attempt++ {
		var req MuteChatRequest
		unknown, err := DecodeRequestWithUnknownFields(strings.NewReader(body), &req)
		if err != nil {
			t.Fatalf("decode error = %v, want nil", err)
		}
		want := []string{"alpha", "mid", "zebra"}
		if len(unknown) != len(want) {
			t.Fatalf("unknown = %v, want %v", unknown, want)
		}
		for i := range want {
			if unknown[i] != want[i] {
				t.Fatalf("attempt %d: unknown = %v, want %v (sorted)", attempt, unknown, want)
			}
		}
	}
}

// A body that is not a JSON object has no top-level key to name, and must not
// crash or invent one. The decode outcome is whatever it already was.
func TestDecodeRequestWithUnknownFields_NonObjectBody(t *testing.T) {
	var req MuteChatRequest
	unknown, err := DecodeRequestWithUnknownFields(strings.NewReader(`[1,2,3]`), &req)
	if err == nil {
		t.Fatal("decode error = nil, want the same error DecodeRequest gives for an array")
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none", unknown)
	}
}

// An empty body must keep answering exactly as before (io.EOF from Decode),
// because every handler turns that into 400 could_not_decode_payload.
func TestDecodeRequestWithUnknownFields_EmptyBodyKeepsDecodeError(t *testing.T) {
	var withDetection, plain MuteChatRequest
	_, gotErr := DecodeRequestWithUnknownFields(strings.NewReader(``), &withDetection)
	wantErr := DecodeRequest(strings.NewReader(``), &plain)
	if gotErr == nil || wantErr == nil {
		t.Fatalf("errors = (%v, %v), want both non-nil", gotErr, wantErr)
	}
	if !errors.Is(gotErr, wantErr) && gotErr.Error() != wantErr.Error() {
		t.Fatalf("error = %v, want the same DecodeRequest gives (%v)", gotErr, wantErr)
	}
}

// DisallowUnknownFields applies at EVERY level of the document, so probing a
// top-level key whose value is an object reports the error of a key NESTED
// inside it. Measured while writing the negative controls, with a
// prefix-matched error:
//
//	{"Phone":"…","Body":"oi","ReplyTo":{"StanzaId":"A","Participant":"p","typo":1}}
//	  -> unknown=[ReplyTo]
//
// `ReplyTo` is a field of SendMessageRequest. Reporting it is wrong twice
// over: it names a valid field, and in strict mode it would refuse a request
// while pointing the caller at the wrong key.
func TestDecodeRequestWithUnknownFields_NestedStrayKeyDoesNotBlameTheParent(t *testing.T) {
	var req SendMessageRequest
	unknown, err := DecodeRequestWithUnknownFields(strings.NewReader(
		`{"Phone":"5511999999999@s.whatsapp.net","Body":"oi","ReplyTo":{"StanzaId":"A","Participant":"p","typo":1}}`), &req)
	if err != nil {
		t.Fatalf("decode error = %v, want nil", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none: `ReplyTo` IS a field of "+
			"SendMessageRequest, and the stray key is one level below it — "+
			"which this pass does not inspect and must therefore not blame "+
			"the parent for", unknown)
	}
	if req.ReplyTo == nil || req.ReplyTo.StanzaID != "A" {
		t.Fatalf("ReplyTo = %+v, want it decoded: the nested object is read "+
			"normally, stray key and all", req.ReplyTo)
	}
}
