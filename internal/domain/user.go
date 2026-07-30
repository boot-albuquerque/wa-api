// Package domain contém as entidades centrais do domínio disparazaap-wuzapi.
package domain

// ListUsersRequest é o request para listar usuários
type ListUsersRequest struct {
	UserID string // Optional: if provided, lists a single user
}

// AddUserRequest é o request para adicionar um novo usuário
type AddUserRequest struct {
	Name        string       `json:"name"`
	Token       string       `json:"token"`
	Webhook     string       `json:"webhook,omitempty"`
	Expiration  int          `json:"expiration,omitempty"`
	Events      string       `json:"events,omitempty"`
	ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
	S3Config    *S3Config    `json:"s3Config,omitempty"`
	HmacKey     string       `json:"hmacKey,omitempty"`
	History     int          `json:"history,omitempty"`
}

// EditUserRequest é o request para editar um usuário existente
type EditUserRequest struct {
	UserID      string       `json:"-"` // from URL
	Name        string       `json:"name,omitempty"`
	Token       string       `json:"token,omitempty"`
	Webhook     string       `json:"webhook,omitempty"`
	Expiration  int          `json:"expiration,omitempty"`
	Events      string       `json:"events,omitempty"`
	ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
	S3Config    *S3Config    `json:"s3Config,omitempty"`
	History     int          `json:"history,omitempty"`
}

// DeleteUserRequest é o request para deletar um usuário
type DeleteUserRequest struct {
	UserID string
}

// CheckUserRequest é o request para verificar se um usuário está no WhatsApp
type CheckUserRequest struct {
	Phone []string `json:"phone"`
}

// GetUserLIDRequest é o request para obter o LID de um usuário
type GetUserLIDRequest struct {
	JID string // from URL
}

// BlockUserRequest é o request para bloquear um usuário
type BlockUserRequest struct {
	Phone string `json:"Phone,omitempty"`
	JID   string `json:"JID,omitempty"`
}

// UnblockUserRequest é o request para desbloquear um usuário
type UnblockUserRequest struct {
	Phone string `json:"Phone,omitempty"`
	JID   string `json:"JID,omitempty"`
}

// ProxyConfig representa a configuração de proxy
type ProxyConfig struct {
	Enabled         bool   `json:"enabled"`
	ProxyURL        string `json:"proxyUrl"`
	WebhookUseProxy *bool  `json:"webhookUseProxy,omitempty"`
}

// S3Config representa a configuração S3
type S3Config struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	PathStyle     bool   `json:"pathStyle"`
	PublicURL     string `json:"publicUrl"`
	MediaDelivery string `json:"mediaDelivery"`
	RetentionDays int    `json:"retentionDays"`
}

// UserResponse representa um usuário na resposta
type UserResponse struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Token          string                 `json:"token"`
	Webhook        string                 `json:"webhook"`
	JID            string                 `json:"jid,omitempty"`
	QRCode         string                 `json:"qrcode,omitempty"`
	Connected      bool                   `json:"connected"`
	LoggedIn       bool                   `json:"loggedIn,omitempty"`
	Expiration     int64                  `json:"expiration,omitempty"`
	ProxyConfig    map[string]interface{} `json:"proxy_config,omitempty"`
	S3Config       map[string]interface{} `json:"s3_config,omitempty"`
	Events         string                 `json:"events,omitempty"`
	HmacConfigured bool                   `json:"hmac_configured,omitempty"`
}
