package newsletter

import (
	"time"

	"wa-api/pkg/domain/apperr"
)

// NewsletterRequest is the body of the seventeen channel routes.
//
// ONE REQUEST TYPE FOR SEVENTEEN ROUTES, and the reason is the one the handler
// already gives for having one handler type: the routes differ only in which
// operation they carry, and the use case's `newsletterRequirements` table is
// what decides which field each of them demands. Seventeen request types would
// be seventeen copies of the same decode, kept in step with one table by hand.
//
// WHAT CHANGED IN THE CUTOVER is five field names. They were `serverIDs`,
// `serverID`, `messageID`, `userJID` and `confirmJID` — camelCase tags written
// by hand on an anonymous body struct, and the only keys in this family that
// broke the public naming rule. They are `server_ids`, `server_id`,
// `message_id`, `user_jid` and `confirm_jid` now, with no alias for the old
// spelling: this repository has no external consumers, and a dual spelling
// would mean the wrong one never dies.
type NewsletterRequest struct {
	JID    string `json:"jid"`
	Invite string `json:"invite"`

	Name        string `json:"name"`
	Description string `json:"description"`
	// Picture is raw base64 — the field is []byte in Go, so the JSON carries
	// base64 with no `data:` prefix. This diverges from /chat/send/image, which
	// takes a data URI.
	Picture []byte `json:"picture"`

	Mute bool `json:"mute"`

	Count  int    `json:"count"`
	Before string `json:"before"`
	After  string `json:"after"`
	// Since is RFC 3339 text and not a time.Time, so that a malformed instant
	// is refused by Validate with a named error code instead of dying in the
	// decoder with `could_not_decode_payload`, which says nothing about WHICH
	// field was wrong.
	Since string `json:"since"`

	ServerIDs []int  `json:"server_ids"`
	ServerID  int    `json:"server_id"`
	Reaction  string `json:"reaction"`
	MessageID string `json:"message_id"`

	UserJID    string `json:"user_jid"`
	ConfirmJID string `json:"confirm_jid"`
}

// codeInvalidSince is the error code for an unparseable `since`. A constant
// because a client branches on it across the HTTP boundary (ADR-0004).
const codeInvalidSince = "invalid_since"

// Validate refuses a body that cannot produce a use case request.
//
// IT VALIDATES ONE THING, and the narrowness is deliberate: everything else
// this family requires depends on WHICH operation the route carries, and that
// table lives in the use case (`newsletterRequirements`). Duplicating any row
// of it here would create a second place for `missing_jid` to be decided, and
// the two would drift — which is the defect the single dispatch table was built
// to prevent. `since` is different because it is a FORMAT the client can get
// wrong, independent of the operation.
func (r NewsletterRequest) Validate() error {
	if r.Since == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, r.Since); err != nil {
		return apperr.New(codeInvalidSince, apperr.CategoryValidation,
			"since must be an RFC 3339 instant", false, err)
	}
	return nil
}

// ToDomain produces the use case input. Only call it after Validate: `since`
// is parsed again here and a malformed one is dropped, because a presenter that
// returned an error would make every caller handle a failure Validate already
// reported.
func (r NewsletterRequest) ToDomain() NewsletterInput {
	in := NewsletterInput{
		JID:         r.JID,
		Invite:      r.Invite,
		Name:        r.Name,
		Description: r.Description,
		Picture:     r.Picture,
		Mute:        r.Mute,
		Count:       r.Count,
		Before:      r.Before,
		After:       r.After,
		ServerIDs:   r.ServerIDs,
		ServerID:    r.ServerID,
		Reaction:    r.Reaction,
		MessageID:   r.MessageID,
		UserJID:     r.UserJID,
		ConfirmJID:  r.ConfirmJID,
	}
	if r.Since != "" {
		if since, err := time.Parse(time.RFC3339, r.Since); err == nil {
			in.Since = since
		}
	}
	return in
}

// NewsletterInput is the validated, normalized body — the shape the handler
// hands to the use case.
//
// It exists rather than having ToDomain build the use case request directly for
// the import rule of docs/HTTP-DTO-CONVENTIONS.md §3: this package may import
// pkg/domain and nothing above it. The handler is where this becomes a
// notification.NewsletterRequest, and that is one short, explicit assignment
// per field in the handler's own file.
type NewsletterInput struct {
	JID    string
	Invite string

	Name        string
	Description string
	Picture     []byte

	Mute bool

	Count  int
	Before string
	After  string
	Since  time.Time

	ServerIDs []int
	ServerID  int
	Reaction  string
	MessageID string

	UserJID    string
	ConfirmJID string
}
