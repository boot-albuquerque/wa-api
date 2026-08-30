// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// As structs deste ficheiro JÁ NÃO SÃO o formato de fio. Elas são
// Go-idiomáticas por dentro — PascalCase, sem etiquetas `json` — e quem serve
// HTTP passa por pkg/presentation/http/dto/session.
// Ver docs/HTTP-DTO-CONVENTIONS.md.

// ConnectRequest representa o payload de conexão.
type ConnectRequest struct {
	Subscribe []string
	Immediate bool
}

// ConnectResult representa o resultado da conexão.
type ConnectResult struct {
	Webhook string
	Jid     string
	Events  string
	Details string
}

// DisconnectRequest representa o payload de desconexão.
type DisconnectRequest struct {
	Phone string
}

// DisconnectResult representa o resultado da desconexão.
type DisconnectResult struct {
	Details string
}

// GetQRResult representa o resultado de obtenção do QR code.
//
// CodeAgeSeconds NÃO vem do PairingQRReader/do use case — HOUSEKEEP F375/
// F377: nem noise nem headless sabem dizer quando o código atual
// EXPIRA (a rotação é decidida pelo servidor do WhatsApp, com variância
// medida de 10-60s — F374), então prometer isso seria inventar um número. O
// que a API PODE dizer com honestidade é POR QUANTO TEMPO já devolveu o
// MESMO código — puramente descritivo, sem promessa sobre o futuro. Quem
// preenche este campo é o handler HTTP (o único ponto de vida longa comum
// aos dois engines — ver GetQRHandler em handler_session.go), não o use
// case, que é reconstruído a cada chamada.
type GetQRResult struct {
	QRCode         string
	CodeAgeSeconds int
}

// LogoutRequest representa o payload de logout.
type LogoutRequest struct {
	Phone string
}

// LogoutResult representa o resultado do logout.
type LogoutResult struct {
	Details string
}

// PairPhoneRequest representa o payload de pareamento por telefone.
//
// `Engine` é obrigatório desde 2026-08-28 — ver pkg/pairing para a ordem em
// que é validado (invalid_engine -> engine_mismatch -> capability_not_supported
// -> engine_unavailable). Sem etiquetas `json`: o formato de fio é
// `pkg/presentation/http/dto/session.PairPhoneRequest`, que já usa `phone`
// minúsculo — não há alias `phone_number` nem `Phone` maiúsculo neste
// contrato, ao contrário de outra worktree que resolveu este mesmo problema
// de forma diferente.
type PairPhoneRequest struct {
	Phone  string
	Engine string
}

// PairPhoneResult representa o resultado do pareamento por telefone.
type PairPhoneResult struct {
	LinkingCode string
}

// ProxySummary é o resumo de proxy que GET /session/status reporta.
//
// Era um map[string]interface{} montado no use case, e por isso as suas chaves
// não apareciam em auditoria de etiqueta nenhuma — foi assim que `proxyUrl`
// sobreviveu em camelCase. Struct para que o apresentador quebre a compilação
// quando um campo mudar de nome.
type ProxySummary struct {
	Enabled  bool
	ProxyURL string
}

// S3Summary é o resumo de S3 que GET /session/status reporta.
//
// NÃO carrega credencial nenhuma, e a ausência é deliberada: o status é
// consultado em laço por qualquer cliente autenticado.
type S3Summary struct {
	Enabled       bool
	Endpoint      string
	Region        string
	Bucket        string
	PathStyle     bool
	PublicURL     string
	MediaDelivery string
	RetentionDays int
}

// GetStatusResult representa o resultado de obtenção do status.
type GetStatusResult struct {
	ID             string
	Name           string
	Connected      bool
	LoggedIn       bool
	Token          string
	Jid            string
	Webhook        string
	Events         string
	ProxyURL       string
	Qrcode         string
	History        string
	ProxyConfig    ProxySummary
	S3Config       S3Summary
	HMACConfigured bool
}

// SetStatusMessageRequest representa o payload de definição de status.
type SetStatusMessageRequest struct {
	Body string
}

// SetStatusMessageResult representa o resultado da definição de status.
type SetStatusMessageResult struct {
	Details string
}

// PublishStatusImageRequest is the payload for POST /status/set/image.
// Image is the same union as SendImageRequest.Image: data URI or http(s) URL.
type PublishStatusImageRequest struct {
	Image         string
	Caption       string
	ID            string
	MimeType      string
	JPEGThumbnail []byte
}

// PublishStatusImageResult is the result of POST /status/set/image.
type PublishStatusImageResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// PublishStatusVideoRequest is the payload for POST /status/set/video.
// Video is the same union as SendVideoRequest.Video: data URI or http(s) URL.
type PublishStatusVideoRequest struct {
	Video         string
	Caption       string
	ID            string
	MimeType      string
	JPEGThumbnail []byte
}

// PublishStatusVideoResult is the result of POST /status/set/video.
type PublishStatusVideoResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// PublishStatusAudioRequest is the payload for POST /status/set/audio.
// Audio is the same union as SendAudioRequest.Audio: data URI or http(s) URL.
type PublishStatusAudioRequest struct {
	Audio    string
	ID       string
	MimeType string
}

// PublishStatusAudioResult is the result of POST /status/set/audio.
type PublishStatusAudioResult struct {
	MessageID string
	Timestamp int64
	Status    string
}

// RequestHistorySyncRequest representa o payload de requisição de sincronização de histórico.
type RequestHistorySyncRequest struct {
	Count              int
	ChatJid            string
	OldestMsgID        string
	OldestMsgFromMe    bool
	OldestMsgTimestamp int64
}

// RequestHistorySyncResult representa o resultado da sincronização de histórico.
type RequestHistorySyncResult struct {
	Details            string
	Timestamp          int64
	Count              int
	ChatJid            string
	OldestMsgID        string
	OldestMsgFromMe    bool
	OldestMsgTimestamp int64
}

// SyncContactRosterRequest representa o payload de sincronização forçada da
// agenda de contatos (patch de app-state critical_unblock_low). Capacidade
// distinta de RequestHistorySyncRequest: não mexe em histórico de mensagens.
type SyncContactRosterRequest struct {
	// Mode escolhe o custo do pull: "if_unsynced" (no-op se já sincronizado),
	// "incremental" (fetch barato, não apaga versão) ou "full" (re-snapshot
	// completo, caro).
	Mode string
}

// SyncContactRosterResult representa o resultado da sincronização forçada da
// agenda de contatos.
type SyncContactRosterResult struct {
	Details string
	Mode    string
}
