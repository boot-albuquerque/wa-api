package domain

import "time"

// SendPresenceRequest para POST /chat/presence
type SendPresenceRequest struct {
	Type string `json:"type"` // "available", "unavailable"
}

// SubscribePresenceRequest para POST /chat/presence/subscribe
type SubscribePresenceRequest struct {
	ChatTarget
	Phone string `json:"Phone"`
}

func (r *SubscribePresenceRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// ChatPresenceRequest para POST /chat/presence/chat
type ChatPresenceRequest struct {
	ChatTarget
	Phone string `json:"Phone"`
	State string `json:"State"` // "typing", "paused", "recording"
	Media string `json:"Media"` // optional media type
}

func (r *ChatPresenceRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// ReactRequest para POST /chat/react
type ReactRequest struct {
	ChatTarget
	Phone       string `json:"Phone"`
	Body        string `json:"Body"`        // emoji or "remove"
	Id          string `json:"Id"`          // message ID
	Participant string `json:"Participant"` // optional participant JID
}

func (r *ReactRequest) ResolveChat() { ResolveChatField(&r.Phone, r.ChatAlias) }

// MarkReadRequest para POST /chat/markread
type MarkReadRequest struct {
	Id          []string `json:"Id"`
	ChatPhone   string   `json:"ChatPhone"`   // new standardized field
	SenderPhone string   `json:"SenderPhone"` // new standardized field
	Chat        string   `json:"Chat"`        // legacy field
	Sender      string   `json:"Sender"`      // legacy field
}

// PresenceType é o estado de presença global da sessão, em termos de
// domínio. Os valores aceitos são exatamente os que SendPresenceRequest.Type
// já documentava.
type PresenceType string

const (
	// PresenceAvailable marca a sessão como online.
	PresenceAvailable PresenceType = "available"
	// PresenceUnavailable marca a sessão como offline.
	PresenceUnavailable PresenceType = "unavailable"
)

// Reaction descreve uma reação a mensagem em termos de domínio, sem nenhuma
// forma de protobuf. A montagem da mensagem para o SDK é do adapter.
type Reaction struct {
	// TargetMessageID é o ID da mensagem reagida, já sem o prefixo "me:".
	TargetMessageID string
	// FromMe indica que a mensagem reagida foi enviada pela própria sessão
	// (era o prefixo "me:" no ID recebido).
	FromMe bool
	// Participant é o autor da mensagem reagida em conversa de grupo.
	// Vazio quando FromMe, ou quando não foi informado/não pôde ser
	// resolvido.
	Participant JID
	// Text é o emoji da reação, ou vazio para remover a reação.
	Text string
	// SentAt é o instante atribuído à reação.
	SentAt time.Time
}

// MessageSendResult é o que o domínio precisa saber sobre uma mensagem
// aceita pelo servidor.
type MessageSendResult struct {
	Timestamp time.Time
	// ID é o identificador que o servidor efetivamente usou para a
	// mensagem. Vazio para operações (como SendReaction) cujo ID já é
	// conhecido do chamador antes do envio.
	ID string
}
