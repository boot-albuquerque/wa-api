// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

// Session representa uma sessão WhatsApp conectada no wa-api.
type Session struct {
	ID         string `json:"id"`
	JID        JID    `json:"jid"`
	Token      string `json:"token"`
	Connected  bool   `json:"connected"`
	WebhookURL string `json:"webhook_url"`
	QRCode     string `json:"qrcode"`
}

// User representa um usuário/admin do wa-api.
type User struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Webhook string `json:"webhook"`
	JID     JID    `json:"jid"`
	IsAdmin bool   `json:"is_admin"`
	Enabled bool   `json:"enabled"`
}

// Message representa uma mensagem WhatsApp.
type Message struct {
	ID        string `json:"id"`
	JID       JID    `json:"jid"`
	From      JID    `json:"from"`
	Body      string `json:"body"`
	Timestamp int64  `json:"timestamp"`
	MediaURL  string `json:"media_url,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
}

// Webhook representa a configuração de webhook de um usuário.
type Webhook struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// HistoryMessage representa uma mensagem no histórico.
type HistoryMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	JID       JID    `json:"jid"`
	From      JID    `json:"from"`
	Body      string `json:"body"`
	Timestamp int64  `json:"timestamp"`
	Direction string `json:"direction"`
	MediaURL  string `json:"media_url,omitempty"`
	Status    string `json:"status"`
}
