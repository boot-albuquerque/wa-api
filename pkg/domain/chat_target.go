package domain

import (
	"encoding/json"
	"io"
)

// ChatTarget is embedded in request types to accept "chat" as a universal
// JSON alias for the destination field. After standard json.Unmarshal
// populates ChatAlias from the "chat" key, the owning type calls
// ResolveChat() to copy it into the legacy destination field when that
// field is empty.
//
// Precedence: the route-specific field (Phone, groupJID, jid, Group)
// wins when non-empty — it is the explicit name the caller chose. The
// alias wins only when the legacy field is absent. When both are present
// and different, the legacy field prevails (the caller was explicit).
type ChatTarget struct {
	ChatAlias string `json:"chat"`
}

// ChatResolver is implemented by request types that embed ChatTarget.
// DecodeRequest calls it automatically after JSON decoding.
type ChatResolver interface {
	ResolveChat()
}

// DecodeRequest reads JSON from r into dst and, when dst implements
// ChatResolver, resolves the "chat" alias into the legacy destination
// field. Every handler that decodes a request with a destination field
// should use this instead of json.NewDecoder(r).Decode(dst).
func DecodeRequest(r io.Reader, dst interface{}) error {
	if err := json.NewDecoder(r).Decode(dst); err != nil {
		return err
	}
	if cr, ok := dst.(ChatResolver); ok {
		cr.ResolveChat()
	}
	return nil
}

// ResolveChatField copies alias into *dst when *dst is empty.
// Shared by all ResolveChat implementations.
func ResolveChatField(dst *string, alias string) {
	if *dst == "" {
		*dst = alias
	}
}
