package domain

import "time"

// The channel (newsletter) types the application works with.
//
// These are NOT the wire format. Until the DTO migration they were: the routes
// served `types.NewsletterMetadata` from the wa-noise fork straight to the JSON
// encoder, which meant the SHAPE of `POST /newsletter/info` was decided by the
// vendor's struct tags — and, on a headless session, by an entirely different
// struct (`channel.DirectoryEntry`, whose fields carry no tags at all and so
// reached the wire as `JID`, `Subscribers`, `Membership`).
//
// The public shape now lives in pkg/presentation/http/dto/newsletter, and these
// types are what both engines normalize INTO. See docs/HTTP-DTO-CONVENTIONS.md.

// NewsletterText is a channel's name or its description, together with when it
// last changed.
//
// One type for both because the protocol carries them identically, and because
// the timestamp is meaningless apart from the text it belongs to.
type NewsletterText struct {
	Text string
	// ID identifies THIS VERSION of the text, not the channel.
	ID        string
	UpdatedAt time.Time
}

// NewsletterPicture describes a channel's profile image or its preview.
//
// It carries no bytes: the protocol answers with a locator, and downloading it
// is a different call. A zero URL and a zero DirectPath together mean the
// channel reported no image on this route — which is not the same as the
// channel having none (measured 2026-08-26: `POST /newsletter/info` fills what
// `GET /newsletter/list` leaves empty for the same channel).
type NewsletterPicture struct {
	URL string
	// ID changes between calls for the same channel, so it is not a cache key.
	ID         string
	Type       string
	DirectPath string
}

// NewsletterViewer is what a channel is FOR THE CALLER.
//
// A pointer field on NewsletterMetadata rather than two flat strings: the
// protocol answers `null` for a channel the caller has no relationship with,
// and flattening it would turn "no relationship" into "unknown role", which are
// different answers.
type NewsletterViewer struct {
	// MuteState is "on" or "off" in every value observed. It is NOT a bool:
	// the protocol writes words here, and a third word would be lost by a bool.
	MuteState string
	// Role is "owner", "admin", "subscriber" or "guest" in every value
	// observed. Carried verbatim — an unknown role is served as it arrived
	// rather than collapsed onto a known one.
	Role string
}

// NewsletterMetadata is a channel, as the application knows it.
//
// Every string field is carried VERBATIM from the transport when it names a
// protocol vocabulary (State, VerificationState, ReactionsMode, Viewer.Role).
// None of them is validated against an enumeration here, for the reason the
// OpenAPI document already records: they are response fields copied from
// WhatsApp, the observed sets are not closed, and converting an unknown value
// would be inventing one.
type NewsletterMetadata struct {
	JID JID
	// State is "active", "deleted", "non_existing" in the values measured.
	// Empty when the transport does not report one.
	State string

	CreatedAt time.Time
	// InviteCode is the tail of the public link, and is what
	// POST /newsletter/info-invite accepts.
	InviteCode string

	Name        NewsletterText
	Description NewsletterText

	SubscriberCount   int
	VerificationState string
	ReactionsMode     string

	Picture *NewsletterPicture
	Preview *NewsletterPicture

	Viewer *NewsletterViewer
}

// NewsletterMessage is one post in a channel.
//
// WHAT IT DELIBERATELY DOES NOT CARRY is the protocol message tree. Until this
// migration `POST /newsletter/messages` served `*waE2E.Message` verbatim, and
// that is a protobuf whose JSON names are camelCase (`extendedTextMessage`,
// `fileLength`, `directPath`) — every one of them a violation of the public
// naming rule, and none of them ours to rename. The readable content is
// extracted into Text at the adapter; the tree itself is gone from the wire.
type NewsletterMessage struct {
	// ServerID identifies the post WITHIN the channel. It is what
	// /newsletter/mark-viewed, /newsletter/react and the `before` cursor take.
	ServerID int
	// ID is the global message identifier.
	ID string
	// Type is the protocol's own word for the post ("media", "text").
	// Measured unreliable: text-only posts arrive marked "media".
	Type      string
	Timestamp time.Time
	ViewCount int
	// ReactionCounts is emoji -> count. The keys are NOT normalized: the same
	// emoji appears as separate entries depending on the Unicode variation
	// selector, and normalizing here would silently merge counts the protocol
	// keeps apart.
	ReactionCounts map[string]int
	// Text is the readable content, extracted from the protocol message.
	// Empty when the post carries none (or carries only media without caption).
	Text string
}

// NewsletterCollection is the channels a session follows.
type NewsletterCollection struct {
	Newsletters []NewsletterMetadata
}

// NewsletterAdminInvite is the server's confirmation of an admin-invite
// creation: the invite's own ID and until when it is valid. F261 — until
// 2026-08-28 this value was read from the protocol response and discarded,
// so POST /newsletter/admin-invite answered `data:null`, indistinguishable
// from "nothing happened".
type NewsletterAdminInvite struct {
	ID             string
	ExpirationTime time.Time
}
