package newsletter

import (
	"time"

	"wa-api/pkg/domain"
)

// Presenters are hand-written, one function per type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.NewsletterMetadata
// and this file stops building. A reflection-based mapper would keep building
// and would silently drop the key from every response.

// PresentNewsletter maps one channel. A nil input presents as nil, so the
// caller does not branch before calling.
func PresentNewsletter(m *domain.NewsletterMetadata) *NewsletterResponse {
	if m == nil {
		return nil
	}
	out := &NewsletterResponse{
		JID:   string(m.JID),
		State: m.State,

		CreatedAt:  presentTime(m.CreatedAt),
		InviteCode: m.InviteCode,

		Name:        PresentNewsletterText(m.Name),
		Description: PresentNewsletterText(m.Description),

		SubscriberCount:   m.SubscriberCount,
		VerificationState: m.VerificationState,
		ReactionsMode:     m.ReactionsMode,

		Picture: PresentNewsletterPicture(m.Picture),
		Preview: PresentNewsletterPicture(m.Preview),
	}
	if m.Viewer != nil {
		out.Viewer = &NewsletterViewerResponse{
			MuteState: m.Viewer.MuteState,
			Role:      m.Viewer.Role,
		}
	}
	return out
}

// PresentNewsletterText maps a channel's name or description.
func PresentNewsletterText(t domain.NewsletterText) NewsletterTextResponse {
	return NewsletterTextResponse{
		Text:      t.Text,
		ID:        t.ID,
		UpdatedAt: presentTime(t.UpdatedAt),
	}
}

// PresentNewsletterPicture maps an image locator.
func PresentNewsletterPicture(p *domain.NewsletterPicture) *NewsletterPictureResponse {
	if p == nil {
		return nil
	}
	return &NewsletterPictureResponse{
		URL:        p.URL,
		ID:         p.ID,
		Type:       p.Type,
		DirectPath: p.DirectPath,
	}
}

// PresentNewsletterMessage maps one post.
func PresentNewsletterMessage(m domain.NewsletterMessage) NewsletterMessageResponse {
	out := NewsletterMessageResponse{
		ServerID:  m.ServerID,
		MessageID: m.ID,
		Type:      m.Type,
		Timestamp: presentTime(m.Timestamp),
		ViewCount: m.ViewCount,
		Text:      m.Text,
	}
	// Allocated even when the post has no reactions: `{}` and `null` are
	// different values to every client, and only one of them can be indexed
	// without a check.
	out.ReactionCounts = make(map[string]int, len(m.ReactionCounts))
	for emoji, count := range m.ReactionCounts {
		out.ReactionCounts[emoji] = count
	}
	return out
}

// PresentListNewsletters maps the listing use case result onto the route's
// `data`.
func PresentListNewsletters(c *domain.NewsletterCollection) ListNewslettersResponse {
	out := ListNewslettersResponse{Newsletters: []NewsletterResponse{}}
	if c == nil {
		return out
	}
	// Allocated even for an empty listing: `[]` and `null` are different values
	// to every client, and only one of them can be ranged over without a check.
	out.Newsletters = make([]NewsletterResponse, 0, len(c.Newsletters))
	for i := range c.Newsletters {
		out.Newsletters = append(out.Newsletters, *PresentNewsletter(&c.Newsletters[i]))
	}
	return out
}

// PresentNewsletterInfo maps the one-channel operations (create, info,
// info-invite).
//
// It takes the domain value and not the use case result on purpose: a DTO
// package that imported the application layer would be one import away from a
// use case signature mentioning a DTO, which is the coupling this layer exists
// to break (docs/HTTP-DTO-CONVENTIONS.md §3).
func PresentNewsletterInfo(m *domain.NewsletterMetadata) NewsletterInfoResponse {
	return NewsletterInfoResponse{Newsletter: PresentNewsletter(m)}
}

// PresentNewsletterMessages maps the post-listing operations (messages,
// updates).
func PresentNewsletterMessages(msgs []domain.NewsletterMessage) NewsletterMessagesResponse {
	// Allocated even for an empty page: `[]` and `null` are different values to
	// every client, and only one of them can be ranged over without a check.
	out := NewsletterMessagesResponse{Messages: make([]NewsletterMessageResponse, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, PresentNewsletterMessage(m))
	}
	return out
}

// PresentNewsletterSubscribe maps the live-updates subscription.
//
// The duration is rendered in whole SECONDS, truncating, which is what the use
// case reported before it: the protocol answers a lease measured in seconds and
// a sub-second remainder would be noise a caller cannot act on.
func PresentNewsletterSubscribe(status string, lease time.Duration) NewsletterSubscribeResponse {
	return NewsletterSubscribeResponse{
		Status:          status,
		DurationSeconds: int64(lease.Seconds()),
	}
}

// PresentNewsletterAck maps an operation that changes something and answers
// with no payload.
func PresentNewsletterAck(status string) NewsletterAckResponse {
	return NewsletterAckResponse{Status: status}
}

// presentTime renders a timestamp as RFC 3339 in UTC, or null when the domain
// has no value for it.
//
// The zero time is null and not "0001-01-01T00:00:00Z", and not the protocol's
// own "0" either: both are real instants on the wire, and a client that parses
// either gets a channel created before the Gregorian calendar or on
// 1970-01-01 instead of a missing field. The second of those was measured on
// every deleted channel.
func presentTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
