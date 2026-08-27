package message

import (
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// The presenters of the chat MANAGEMENT surface. Same discipline as
// presenter.go: hand-written, field by field, one function per source type.

// Details of the four presence answers and the two disappearing-timer answers.
// They are constants and not literals at the call site because a message
// repeated in two places is the same bug waiting to diverge (ADR-0004).
const (
	DetailsPresenceSent           = "Presence sent"
	DetailsPresenceSubscribed     = "Presence subscription updated"
	DetailsChatPresenceSent       = "Chat presence sent"
	DetailsMessageMarkedRead      = "Message marked as read"
	DetailsDisappearingSet        = "Disappearing timer set"
	DetailsDefaultDisappearingSet = "Default disappearing timer set"
)

// PresentAction wraps a fixed acceptance line.
func PresentAction(details string) ActionResponse {
	return ActionResponse{Details: details}
}

// PresentDownload maps the result of any /chats/download route.
func PresentDownload(r *domain.DownloadResult) *DownloadResponse {
	if r == nil {
		return nil
	}
	return &DownloadResponse{
		Mimetype: r.Mimetype,
		Data:     r.Data,
	}
}

// PresentStarMessage maps POST /message/star.
func PresentStarMessage(r *domain.StarMessageResult) *StarMessageResponse {
	if r == nil {
		return nil
	}
	return &StarMessageResponse{
		Success: r.Success,
		Message: r.Message,
	}
}

// PresentPublishStatusImage maps POST /status/set/image.
func PresentPublishStatusImage(r *domain.PublishStatusImageResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentPublishStatusVideo maps POST /status/set/video.
func PresentPublishStatusVideo(r *domain.PublishStatusVideoResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentPublishStatusAudio maps POST /status/set/audio.
func PresentPublishStatusAudio(r *domain.PublishStatusAudioResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentChatHistoryMessage maps one persisted message.
func PresentChatHistoryMessage(m appport.ChatHistoryMessage) ChatHistoryMessageResponse {
	return ChatHistoryMessageResponse{
		ID:              m.ID,
		UserID:          m.UserID,
		ChatJID:         m.ChatJID,
		SenderJID:       m.SenderJID,
		MessageID:       m.MessageID,
		Timestamp:       presentInstant(m.Timestamp),
		MessageType:     m.MessageType,
		TextContent:     m.TextContent,
		MediaLink:       m.MediaLink,
		QuotedMessageID: presentOptionalString(m.QuotedMessageID),
		DataJSON:        m.DataJson,
	}
}

// PresentChatHistoryMessages maps the normal branch of GET /chat/history.
//
// The empty slice is allocated: `[]` and `null` are different values to every
// client, and only one of the two can be ranged over without a check.
func PresentChatHistoryMessages(msgs []appport.ChatHistoryMessage) []ChatHistoryMessageResponse {
	out := make([]ChatHistoryMessageResponse, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, PresentChatHistoryMessage(m))
	}
	return out
}

// PresentChatIndexEntry maps one chat of the `chat_jid=index` listing.
func PresentChatIndexEntry(e appport.ChatIndexEntry) ChatIndexEntryResponse {
	return ChatIndexEntryResponse{
		ChatJID:     e.ChatJID,
		LastUpdated: e.LastUpdated,
	}
}

// PresentChatIndex maps the `chat_jid=index` branch of GET /chat/history.
//
// The KEYS of this map are session ids chosen by whoever provisioned the user,
// not names this API controls, so they are the one place in the public payload
// where the snake_case rule cannot be asserted. The VALUES are presented like
// everything else.
func PresentChatIndex(index map[string][]appport.ChatIndexEntry) map[string][]ChatIndexEntryResponse {
	out := make(map[string][]ChatIndexEntryResponse, len(index))
	for user, entries := range index {
		presented := make([]ChatIndexEntryResponse, 0, len(entries))
		for _, e := range entries {
			presented = append(presented, PresentChatIndexEntry(e))
		}
		out[user] = presented
	}
	return out
}

// presentInstant renders a timestamp as RFC 3339 in UTC, or null when there is
// none. Same reasoning as presentTime in the group family: the zero time
// formatted is a real date on the wire, not a missing field.
func presentInstant(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
