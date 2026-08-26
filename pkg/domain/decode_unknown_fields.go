package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
)

// Unknown-field detection (F268).
//
// Go's encoding/json drops a key no field accepts, in silence. That is not a
// cosmetic gap: a misspelled name does not become an error, it becomes the
// ZERO VALUE — and where the zero value carries a meaning of its own, the typo
// turns into a silent CHOICE. Measured on /chat/mute: `duration` instead of
// `mute_duration` answered 200 "Chat muted" and muted the chat FOREVER,
// because nil collapses into 0 and 0 means "forever".
//
// What this file adds is the ability to SEE it. Detection only — the decoded
// result is byte-for-byte what DecodeRequest already produced, and the policy
// (warn or reject) lives at the HTTP boundary, not here.

// unknownFieldErrorFormat is what encoding/json writes when
// DisallowUnknownFields rejects a key. The NAME is compared, not just the
// prefix, and that distinction is load-bearing:
//
// DisallowUnknownFields applies at EVERY level of the document, so probing
// `{"ReplyTo":{"StanzaId":"A","typo":1}}` reports `json: unknown field "typo"`
// — an error about a NESTED key, raised while probing a top-level key that is
// perfectly valid. Measured, before this was an exact match: a body with a
// stray key inside ReplyTo reported `ReplyTo` itself as unknown, which in
// strict mode would refuse a valid request while naming the wrong field.
//
// Comparing the name keeps the answer about the key actually being probed.
// The consequence is the documented limitation of this file: a nested stray
// key is not reported at all, rather than reported wrongly.
//
// Source: encoding/json (decode.go, object): with DisallowUnknownFields set,
// the decoder saves `json: unknown field %q` at field LOOKUP time and then
// skips the value — so an unknown key can never produce a type error first.
const unknownFieldErrorFormat = "json: unknown field %q"

// DecodeRequestWithUnknownFields decodes exactly as DecodeRequest does and
// additionally reports the names of the top-level JSON keys that dst accepts
// no field for. The returned names are sorted, so a caller that logs them
// gets a stable line (map iteration order in Go is random by design).
//
// The decoded value and the returned error are the same ones DecodeRequest
// would produce: this never turns an accepted request into a rejected one.
// When the body cannot be decoded at all, no field names are reported —
// naming fields of a body that was refused would be noise.
//
// LIMITATION, stated so nobody assumes otherwise: only TOP-LEVEL keys are
// inspected. A stray key nested inside an object-valued field is not reported.
// Every case measured so far (F268, F270, contract audit section 5) is
// top-level, and the enumeration technique below cannot address a nested key
// by name.
func DecodeRequestWithUnknownFields(r io.Reader, dst interface{}) ([]string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := DecodeRequest(bytes.NewReader(body), dst); err != nil {
		return nil, err
	}
	return unknownTopLevelFields(body, dst), nil
}

// unknownTopLevelFields names the keys of body that dst has no field for.
//
// It asks encoding/json itself, one key at a time, instead of walking the
// struct with reflection and matching names by hand. That is deliberate: Go
// matches JSON keys to fields case-INSENSITIVELY, honours `json:"-"`, and
// descends into embedded structs (ChatTarget is embedded in most request
// types). A hand-written matcher would be a double DIVERGENT from production —
// it would answer "unknown" for keys the real decoder accepts, which is the
// worst possible direction for a check that can reject requests.
func unknownTopLevelFields(body []byte, dst interface{}) []string {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		// Not a JSON object: an array, a scalar, or a body with trailing
		// content that Decode tolerated. There is no top-level key to name,
		// and the decode above already settled the outcome.
		return nil
	}

	target := reflect.TypeOf(dst)
	if target == nil || target.Kind() != reflect.Pointer {
		return nil
	}
	element := target.Elem()

	var unknown []string
	for name, value := range keys {
		if isUnknownField(name, value, element) {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// isUnknownField reports whether a struct of type element has no field that
// accepts this key, by decoding a one-key object into a FRESH zero value with
// DisallowUnknownFields. The fresh value is what keeps this pass free of side
// effects on the caller's dst.
//
// The original value is carried along instead of a placeholder because a type
// mismatch must not be mistaken for an unknown key — see
// unknownFieldErrorFormat for why it cannot be, and for why the error is
// matched against this key's NAME rather than against a prefix.
func isUnknownField(name string, value json.RawMessage, element reflect.Type) bool {
	object, err := json.Marshal(map[string]json.RawMessage{name: value})
	if err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(object))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(reflect.New(element).Interface())
	return err != nil && err.Error() == fmt.Sprintf(unknownFieldErrorFormat, name)
}
