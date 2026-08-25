package domain

import "time"

// MuteChatRequest represents a request to mute or unmute a chat.
//
// When Mute is true, MuteDuration controls for how long:
//   - nil  → mute forever ("always")
//   - 8h   → mute for 8 hours
//   - 168h → mute for 1 week
//
// When Mute is false, MuteDuration is ignored (unmute is immediate).
type MuteChatRequest struct {
	ChatTarget
	Jid          string         `json:"jid"`
	Mute         bool           `json:"mute"`
	MuteDuration *time.Duration `json:"mute_duration,omitempty"`
}

func (r *MuteChatRequest) ResolveChat() { ResolveChatField(&r.Jid, r.ChatAlias) }

// MuteChatResult represents the result of muting/unmuting a chat.
type MuteChatResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
