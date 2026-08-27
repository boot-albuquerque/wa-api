package misc

import (
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain"
)

// Protocol -> domain normalization for channels.
//
// This is the seam the DTO migration needed. Before it, the adapter handed
// `*types.NewsletterMetadata` back as `any` and the HTTP layer serialized the
// vendor struct directly, which made the vendor's `json` tags the public
// contract of four routes. Mapping here means renaming a vendor field is a
// COMPILE ERROR in this file instead of a silent change of the wire.
//
// Every mapper takes the protocol value and returns the domain value; none of
// them validates. An unknown enumeration word is carried verbatim, because a
// response field copied from WhatsApp is not ours to close.

// mapNewsletterMetadata normalizes one channel. A nil input maps to nil so the
// caller does not branch.
func mapNewsletterMetadata(m *types.NewsletterMetadata) *domain.NewsletterMetadata {
	if m == nil {
		return nil
	}
	tm := m.ThreadMeta
	out := &domain.NewsletterMetadata{
		JID:   domain.JID(m.ID.String()),
		State: string(m.State.Type),

		CreatedAt:  tm.CreationTime.Time,
		InviteCode: tm.InviteCode,

		Name:        mapNewsletterText(tm.Name),
		Description: mapNewsletterText(tm.Description),

		SubscriberCount:   tm.SubscriberCount,
		VerificationState: string(tm.VerificationState),
		ReactionsMode:     string(tm.Settings.ReactionCodes.Value),

		Picture: mapNewsletterPicture(tm.Picture),
	}
	// Preview is a VALUE in the protocol where Picture is a pointer, and that
	// asymmetry is preserved rather than smoothed: `picture` was measured null
	// on GET /newsletter/list for channels that have one, while `preview` came
	// back as an object in EVERY measurement of 2026-08-26, deleted channels
	// included. Mapping preview unconditionally keeps that difference visible;
	// collapsing both to "null when empty" would claim a channel has no preview
	// when the protocol never said so.
	preview := tm.Preview
	out.Preview = mapNewsletterPicture(&preview)
	if m.ViewerMeta != nil {
		out.Viewer = &domain.NewsletterViewer{
			MuteState: string(m.ViewerMeta.Mute),
			Role:      string(m.ViewerMeta.Role),
		}
	}
	return out
}

// mapNewsletterMetadataList normalizes a listing, dropping the nil entries the
// protocol client can leave in it. The slice is allocated even when empty: `[]`
// and `null` are different answers to whoever reads the route.
func mapNewsletterMetadataList(list []*types.NewsletterMetadata) []domain.NewsletterMetadata {
	out := make([]domain.NewsletterMetadata, 0, len(list))
	for _, m := range list {
		if mapped := mapNewsletterMetadata(m); mapped != nil {
			out = append(out, *mapped)
		}
	}
	return out
}

// mapNewsletterText normalizes a name or a description.
func mapNewsletterText(t types.NewsletterText) domain.NewsletterText {
	return domain.NewsletterText{
		Text:      t.Text,
		ID:        t.ID,
		UpdatedAt: t.UpdateTime.Time,
	}
}

// mapNewsletterPicture normalizes an image locator.
func mapNewsletterPicture(p *types.ProfilePictureInfo) *domain.NewsletterPicture {
	if p == nil {
		return nil
	}
	return &domain.NewsletterPicture{
		URL:        p.URL,
		ID:         p.ID,
		Type:       p.Type,
		DirectPath: p.DirectPath,
	}
}

// mapNewsletterMessages normalizes a page of posts.
func mapNewsletterMessages(list []*types.NewsletterMessage) []domain.NewsletterMessage {
	out := make([]domain.NewsletterMessage, 0, len(list))
	for _, m := range list {
		if m == nil {
			continue
		}
		out = append(out, domain.NewsletterMessage{
			ServerID:       int(m.MessageServerID),
			ID:             string(m.MessageID),
			Type:           m.Type,
			Timestamp:      m.Timestamp,
			ViewCount:      m.ViewsCount,
			ReactionCounts: mapReactionCounts(m.ReactionCounts),
			Text:           newsletterMessageText(m.Message),
		})
	}
	return out
}

// mapReactionCounts copies the reaction tally, allocating even when empty so
// that the route answers `{}` rather than `null` for a post nobody reacted to.
func mapReactionCounts(src map[string]int) map[string]int {
	out := make(map[string]int, len(src))
	for emoji, count := range src {
		out[emoji] = count
	}
	return out
}

// newsletterMessageText extracts the readable content of a post.
//
// It is deliberately SHALLOW. The alternative was serving the protobuf tree,
// which is what the route did until this migration and which cannot satisfy the
// public naming rule — `extendedTextMessage`, `fileLength` and `directPath` are
// the vendor's names, not ours to rename. Extracting the text keeps the part a
// client actually renders and drops the part it could not have relied on
// anyway: the measurement of 2026-08-26 found the shape inside those keys
// varies with the client that published.
//
// A post whose content this function does not recognize yields "", which is
// honest: the route says the post exists, when, and how many reacted, and says
// nothing about content it did not read.
func newsletterMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetConversation(); text != "" {
		return text
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	return ""
}
