package domain

// WebhookConfigRequest representa a requisição para configuração de webhook.
type WebhookConfigRequest struct {
	WebhookURL      string   `json:"webhook"`
	WebhookURLField string   `json:"webhookurl"` // Alternativa usada em SetWebhook
	Events          []string `json:"events,omitempty"`
	Active          bool     `json:"active"`
}

// WebhookConfigResult representa o resultado de operação de webhook.
type WebhookConfigResult struct {
	Webhook   string   `json:"webhook"`
	Events    []string `json:"events,omitempty"`
	Active    bool     `json:"active,omitempty"`
	Details   string   `json:"Details,omitempty"`
	Subscribe []string `json:"subscribe,omitempty"`
}

// WebhookHistoryRequest representa a requisição para configuração de histórico.
type WebhookHistoryRequest struct {
	History int `json:"history"`
}

// WebhookHistoryResult representa o resultado de operação de histórico.
type WebhookHistoryResult struct {
	Details string `json:"Details,omitempty"`
	History int    `json:"History,omitempty"`
}

// ChatMapping representa um mapeamento de chat para histórico.
type ChatMapping struct {
	UserID          string `json:"user_id" db:"user_id"`
	ChatJID         string `json:"chat_jid" db:"chat_jid"`
	LastMessageTime string `json:"last_message_time" db:"last_message_time"`
}

// ChatInfo representa informações de um chat no índice.
type ChatInfo struct {
	ChatJID     string `json:"chat_jid"`
	LastUpdated string `json:"last_updated"`
}
