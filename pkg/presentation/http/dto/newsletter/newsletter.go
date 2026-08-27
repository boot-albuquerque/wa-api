// Package newsletter holds the PUBLIC wire types for the channel (newsletter)
// family of routes, plus the hand-written presenters that build them from
// domain values.
//
// Nothing here may be reused by the domain or by a use case: these types exist
// to be serialized, and every field name in them is a promise to a client.
// See docs/HTTP-DTO-CONVENTIONS.md.
package newsletter

// NewsletterTextResponse is a channel's name or description, with when it last
// changed.
//
// `updated_at` is a pointer because "never changed" and "changed at the epoch"
// are different facts, and the protocol reports the first as a zero it also
// uses for deleted channels. It was `update_time: "0"` before this migration —
// a string holding an integer, which a client parsing dates read as
// 1970-01-01.
type NewsletterTextResponse struct {
	Text string `json:"text"`
	// ID identifies THIS VERSION of the text, not the channel.
	ID        string  `json:"id"`
	UpdatedAt *string `json:"updated_at"`
}

// NewsletterPictureResponse locates a channel's image. It carries no bytes:
// downloading is a different call.
type NewsletterPictureResponse struct {
	URL        string `json:"url"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	DirectPath string `json:"direct_path"`
}

// NewsletterViewerResponse is what the channel is FOR THE CALLER. The whole
// object is null when the caller has no relationship with the channel — which
// is a different answer from "role unknown", and the reason it is not flattened
// into two sibling strings.
type NewsletterViewerResponse struct {
	MuteState string `json:"mute_state"`
	Role      string `json:"role"`
}

// NewsletterResponse is a channel, as served.
//
// No `omitempty` anywhere: a client has to be able to tell "this channel has no
// description" from "this key was dropped in this version".
//
// FOUR FIELDS CHANGED TYPE in the DTO cutover, and all four for the same
// reason — the protocol writes numbers as quoted strings, and serving them
// through meant every consumer parsed them again:
//
//	creation_time "1787746245"   -> created_at  "2026-08-…Z" or null
//	subscribers_count "0"        -> subscriber_count 0
//	name.update_time "0"         -> name.updated_at null
//	state {"type": "active"}     -> state "active"
type NewsletterResponse struct {
	JID string `json:"jid"`
	// State is the protocol's own word ("active", "deleted", "non_existing" in
	// the values measured), carried verbatim and NOT validated: an unknown
	// value is served as it arrived. A client switching on it needs a default
	// branch, and the safe filter for "my channels" is `== "active"`.
	State string `json:"state"`

	CreatedAt  *string `json:"created_at"`
	InviteCode string  `json:"invite_code"`

	Name        NewsletterTextResponse `json:"name"`
	Description NewsletterTextResponse `json:"description"`

	SubscriberCount   int    `json:"subscriber_count"`
	VerificationState string `json:"verification_state"`
	ReactionsMode     string `json:"reactions_mode"`

	Picture *NewsletterPictureResponse `json:"picture"`
	Preview *NewsletterPictureResponse `json:"preview"`

	Viewer *NewsletterViewerResponse `json:"viewer"`
}

// NewsletterMessageResponse is one post in a channel, as served.
//
// THE PROTOCOL MESSAGE TREE IS GONE from this shape, and its absence is the
// one deliberate loss of this migration. The route used to serve `Message`, the
// raw `waE2E.Message` protobuf, whose JSON names are the vendor's camelCase
// (`extendedTextMessage`, `fileLength`, `directPath`) — none of them able to
// satisfy the public naming rule and none of them ours to rename. What a client
// renders is `text`; what it could not have relied on was the tree, whose inner
// shape the 2026-08-26 measurement found varying with the publishing client.
type NewsletterMessageResponse struct {
	ServerID  int    `json:"server_id"`
	MessageID string `json:"message_id"`
	// Type is the protocol's word for the post. MEASURED UNRELIABLE: 14 of 60
	// text-only posts arrived marked "media". Read `text` to know whether there
	// is readable content.
	Type      string  `json:"type"`
	Timestamp *string `json:"timestamp"`
	ViewCount int     `json:"view_count"`
	// ReactionCounts is emoji -> count, never null and `{}` for a post nobody
	// reacted to. The keys are NOT Unicode-normalized: the protocol keeps "❤"
	// and "❤️" apart, and merging them here would invent a total it never sent.
	ReactionCounts map[string]int `json:"reaction_counts"`
	Text           string         `json:"text"`
}

// ListNewslettersResponse is the `data` of GET /newsletter/list.
//
// The key is `newsletters` and was `newsletter`: a list of channels named in
// the singular reads as one channel. Renamed rather than kept because this
// repository has no external consumers and the cutover is the moment such a
// name gets fixed or never does.
type ListNewslettersResponse struct {
	Newsletters []NewsletterResponse `json:"newsletters"`
}

// NewsletterInfoResponse is the `data` of the routes that answer with one
// channel: create, info and info-invite.
//
// It WRAPS instead of inlining the channel at the top of `data`, which is what
// the route did before. The wrapper is what makes those three routes shaped
// like every other one in this family — each answers an object with a named
// payload — and it is where a `meta` would go later without moving any existing
// field.
type NewsletterInfoResponse struct {
	Newsletter *NewsletterResponse `json:"newsletter"`
}

// NewsletterMessagesResponse is the `data` of the two routes that answer with
// posts: messages and updates.
type NewsletterMessagesResponse struct {
	Messages []NewsletterMessageResponse `json:"messages"`
}

// NewsletterSubscribeResponse is the `data` of POST /newsletter/subscribe.
//
// `duration_seconds` REACHED NO CLIENT before this migration: the use case
// computed it, and the handler served only `rsp.Data`, which subscribe leaves
// nil. The route answered `null` and the caller had no way to know when to
// renew — the one fact the operation exists to report.
type NewsletterSubscribeResponse struct {
	Status          string `json:"status"`
	DurationSeconds int64  `json:"duration_seconds"`
}

// NewsletterAckResponse is the `data` of every channel route that changes
// something and returns no payload: follow, unfollow, mute, mark-viewed, react,
// demote, change-owner, delete and the three admin-invite routes.
//
// Those routes answered `data: null`, for the same reason subscribe lost its
// duration: the status the use case sets never reached the handler's argument.
// `null` cannot be told apart from a route that failed to fill a payload, so a
// caller checking `data` for a sign of success found none.
type NewsletterAckResponse struct {
	Status string `json:"status"`
}
