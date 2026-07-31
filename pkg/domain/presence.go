package domain

// SendPresenceRequest para POST /chat/presence
type SendPresenceRequest struct {
	Type string `json:"type"` // "available", "unavailable"
}

// SubscribePresenceRequest para POST /chat/presence/subscribe
type SubscribePresenceRequest struct {
	Phone string `json:"Phone"`
}

// ChatPresenceRequest para POST /chat/presence/chat
type ChatPresenceRequest struct {
	Phone string `json:"Phone"`
	State string `json:"State"` // "typing", "paused", "recording"
	Media string `json:"Media"` // optional media type
}

// ReactRequest para POST /chat/react
type ReactRequest struct {
	Phone       string `json:"Phone"`
	Body        string `json:"Body"`        // emoji or "remove"
	Id          string `json:"Id"`          // message ID
	Participant string `json:"Participant"` // optional participant JID
}

// MarkReadRequest para POST /chat/markread
type MarkReadRequest struct {
	Id          []string `json:"Id"`
	ChatPhone   string   `json:"ChatPhone"`   // new standardized field
	SenderPhone string   `json:"SenderPhone"` // new standardized field
	Chat        string   `json:"Chat"`        // legacy field
	Sender      string   `json:"Sender"`      // legacy field
}
