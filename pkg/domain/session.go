// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// ConnectRequest representa o payload de conexão.
type ConnectRequest struct {
	Subscribe []string `json:"subscribe,omitempty"`
	Immediate bool     `json:"immediate,omitempty"`
}

// ConnectResult representa o resultado da conexão.
type ConnectResult struct {
	Webhook string `json:"webhook"`
	Jid     string `json:"jid"`
	Events  string `json:"events"`
	Details string `json:"details"`
}

// DisconnectRequest representa o payload de desconexão.
type DisconnectRequest struct {
	Phone string `json:"phone,omitempty"`
}

// DisconnectResult representa o resultado da desconexão.
type DisconnectResult struct {
	Details string `json:"details"`
}

// GetQRResult representa o resultado de obtenção do QR code.
type GetQRResult struct {
	QRCode string `json:"QRCode"`
}

// LogoutRequest representa o payload de logout.
type LogoutRequest struct {
	Phone string `json:"phone,omitempty"`
}

// LogoutResult representa o resultado do logout.
type LogoutResult struct {
	Details string `json:"details"`
}

// PairPhoneRequest representa o payload de pareamento por telefone.
type PairPhoneRequest struct {
	Phone string `json:"Phone"`
}

// PairPhoneResult representa o resultado do pareamento por telefone.
type PairPhoneResult struct {
	LinkingCode string `json:"LinkingCode"`
}

// GetStatusResult representa o resultado de obtenção do status.
type GetStatusResult struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Connected      bool                   `json:"connected"`
	LoggedIn       bool                   `json:"loggedIn"`
	Token          string                 `json:"token"`
	Jid            string                 `json:"jid"`
	Webhook        string                 `json:"webhook"`
	Events         string                 `json:"events"`
	ProxyURL       string                 `json:"proxy_url"`
	Qrcode         string                 `json:"qrcode"`
	History        string                 `json:"history"`
	ProxyConfig    map[string]interface{} `json:"proxy_config"`
	S3Config       map[string]interface{} `json:"s3_config"`
	HMACConfigured bool                   `json:"hmac_configured"`
}

// SetStatusMessageRequest representa o payload de definição de status.
type SetStatusMessageRequest struct {
	Body string `json:"Body"`
}

// SetStatusMessageResult representa o resultado da definição de status.
type SetStatusMessageResult struct {
	Details string `json:"details"`
}

// RequestHistorySyncRequest representa o payload de requisição de sincronização de histórico.
type RequestHistorySyncRequest struct {
	Count              int    `json:"count,omitempty"`
	ChatJid            string `json:"chat_jid,omitempty"`
	OldestMsgID        string `json:"oldest_msg_id,omitempty"`
	OldestMsgFromMe    bool   `json:"oldest_msg_from_me,omitempty"`
	OldestMsgTimestamp int64  `json:"oldest_msg_timestamp,omitempty"`
}

// RequestHistorySyncResult representa o resultado da sincronização de histórico.
type RequestHistorySyncResult struct {
	Details            string `json:"details"`
	Timestamp          int64  `json:"timestamp"`
	Count              int    `json:"count"`
	ChatJid            string `json:"chat_jid"`
	OldestMsgID        string `json:"oldest_msg_id"`
	OldestMsgFromMe    bool   `json:"oldest_msg_from_me"`
	OldestMsgTimestamp int64  `json:"oldest_msg_timestamp"`
}

// SyncContactRosterRequest representa o payload de sincronização forçada da
// agenda de contatos (patch de app-state critical_unblock_low). Capacidade
// distinta de RequestHistorySyncRequest: não mexe em histórico de mensagens.
type SyncContactRosterRequest struct {
	// Mode escolhe o custo do pull: "if_unsynced" (no-op se já sincronizado),
	// "incremental" (fetch barato, não apaga versão) ou "full" (re-snapshot
	// completo, caro).
	Mode string `json:"mode"`
}

// SyncContactRosterResult representa o resultado da sincronização forçada da
// agenda de contatos.
type SyncContactRosterResult struct {
	Details string `json:"details"`
	Mode    string `json:"mode"`
}
