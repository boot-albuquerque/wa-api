package events

import (
	"time"

	"wa-api/internal/noise/protocol/types"
)

type NewsletterJoin struct {
	types.NewsletterMetadata
}

type NewsletterLeave struct {
	ID   types.JID            `json:"id"`
	Role types.NewsletterRole `json:"role"`
}

type NewsletterMuteChange struct {
	ID   types.JID                 `json:"id"`
	Mute types.NewsletterMuteState `json:"mute"`
}

type NewsletterLiveUpdate struct {
	JID      types.JID
	Time     time.Time
	Messages []*types.NewsletterMessage
}
