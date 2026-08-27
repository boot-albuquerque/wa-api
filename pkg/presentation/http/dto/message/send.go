// Package message holds the PUBLIC wire types for the message family of
// routes — every /chat/send/*, /chat/delete/*, /chat/react and the chat
// management surface — plus the hand-written presenters that build them from
// domain values.
//
// Nothing here may be reused by the domain or by a use case: these types exist
// to be serialized, and every field name in them is a promise to a client.
// See docs/HTTP-DTO-CONVENTIONS.md.
package message

// SendResponse is the body of the `data` key for every send capability.
//
// The shape {message_id, timestamp, status} is the one decision F131 fixed for
// the fifteen send routes, and send_wire_contract_test.go locks it. What
// changes here versus the domain type it replaces is the ABSENCE of
// `omitempty` on timestamp: a client has to be able to tell "the engine did not
// report an instant" from "this key was dropped by this version", and
// omitempty makes those identical on the wire.
//
// Timestamp is *int64 for that reason, and not int64: zero is not an unknown
// instant, it is 1970-01-01T00:00:00Z — a real date a client would parse.
// Unknown is null.
type SendResponse struct {
	MessageID string `json:"message_id"`
	Timestamp *int64 `json:"timestamp"`
	Status    string `json:"status"`
}

// SendAudioResponse is the body of the `data` key for POST /chat/send/audio.
//
// It carries two fields the other fourteen do not, and they are not
// decoration: the audio caption goes out as a SEPARATE message because the
// protocol has no caption field on AudioMessage (F116), so the audio can
// succeed while the caption fails. Without caption_status a client would see a
// 200 and have to guess.
//
// Both are pointers and both are ALWAYS emitted: null means "there was no
// caption to send", which is a different fact from "the caption failed".
type SendAudioResponse struct {
	MessageID string `json:"message_id"`
	Timestamp *int64 `json:"timestamp"`
	Status    string `json:"status"`

	CaptionMessageID *string `json:"caption_message_id"`
	CaptionStatus    *string `json:"caption_status"`
}
