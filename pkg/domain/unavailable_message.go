package domain

import "time"

// RequestUnavailableMessageRequest represents a request to send an unavailable message request
type RequestUnavailableMessageRequest struct {
	Chat   string `json:"chat"`
	Sender string `json:"sender"`
	ID     string `json:"id"`
}

// RequestUnavailableMessageResult represents the result of sending an unavailable message request
type RequestUnavailableMessageResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
}

// UnavailableMessageAck é a confirmação do servidor ao pedido de reenvio de
// uma mensagem indisponível.
type UnavailableMessageAck struct {
	RequestID string
	Timestamp time.Time
}
