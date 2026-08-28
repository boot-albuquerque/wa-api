package message

// The chat MANAGEMENT surface of the message family: presence, mark-read,
// disappearing timer, star, download and status publishing. Same rules as
// send.go — no `omitempty`, pointer for optional-and-distinguishable.

// ActionResponse is the body of the `data` key for a route whose only answer
// is "the operation was accepted", with a human-readable line saying which.
//
// It replaces the `map[string]string{"Details": "..."}` literals the presence
// and disappearing-timer handlers built inline. Two things were wrong with
// those and only one is the name: a map literal has no type, so there was
// nowhere to hang a `json` tag and therefore nothing a review of the DTO layer
// could catch — the same defect the F190 entry recorded for /chat/react.
//
// `details` and not `Details`: the capital form fails the canonical key rule
// (docs/HTTP-DTO-CONVENTIONS.md §8), and this API has no external consumers to
// keep it for.
type ActionResponse struct {
	Details string `json:"details"`
}

// DownloadResponse is the body of the `data` key for every /chats/download
// route.
//
// `mimetype` and `data` were `Mimetype` and `Data`: the domain struct carried
// those tags because it WAS the wire type.
type DownloadResponse struct {
	Mimetype string `json:"mimetype"`
	// Data is a base64 data URL, not raw bytes: the historical route answered
	// a string a browser can put straight into a src attribute.
	Data string `json:"data"`
}

// StarMessageResponse is the body of the `data` key for POST /message/star.
type StarMessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ChatHistoryMessageResponse is one persisted message of GET /chat/history.
//
// QuotedMessageID is a pointer and is ALWAYS emitted: the domain field carried
// `omitempty`, which made "this message quotes nothing" and "this version
// stopped sending the key" the same bytes.
type ChatHistoryMessageResponse struct {
	ID              int     `json:"id"`
	UserID          string  `json:"user_id"`
	ChatJID         string  `json:"chat_jid"`
	SenderJID       string  `json:"sender_jid"`
	MessageID       string  `json:"message_id"`
	Timestamp       *string `json:"timestamp"`
	MessageType     string  `json:"message_type"`
	TextContent     string  `json:"text_content"`
	MediaLink       string  `json:"media_link"`
	QuotedMessageID *string `json:"quoted_message_id"`
	DataJSON        string  `json:"data_json"`
}

// MuteChatResponse is the body of the `data` key for POST /chat/mute
// (HOUSEKEEP F323).
type MuteChatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ArchiveChatResponse is the body of the `data` key for POST /chat/archive
// (HOUSEKEEP F323).
type ArchiveChatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// PinChatResponse is the body of the `data` key for POST /chat/pin
// (HOUSEKEEP F323).
type PinChatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// RequestUnavailableMessageResponse is the body of the `data` key for POST
// /chat/request-unavailable-message (HOUSEKEEP F323).
type RequestUnavailableMessageResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
}

// ChatIndexEntryResponse is one chat of the `chat_jid=index` listing.
//
// LastUpdated is already a preformatted RFC3339Nano string upstream — the
// formatting strips the Go monotonic-clock suffix the sqlite driver hands back
// for an aggregated column — so the presenter passes it through instead of
// reparsing it just to print it again.
type ChatIndexEntryResponse struct {
	ChatJID     string `json:"chat_jid"`
	LastUpdated string `json:"last_updated"`
}
