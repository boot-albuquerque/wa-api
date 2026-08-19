package domain

// S3ConfigRequest representa a requisição para configuração de S3.
type S3ConfigRequest struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	PathStyle     bool   `json:"path_style"`
	PublicURL     string `json:"public_url"`
	MediaDelivery string `json:"media_delivery"`
	RetentionDays int    `json:"retention_days"`
}

// S3ConfigResult representa o resultado de operação de S3.
type S3ConfigResult struct {
	Details string `json:"Details,omitempty"`
	Enabled bool   `json:"Enabled,omitempty"`
}

// S3TestRequest representa a requisição para teste de conexão S3.
type S3TestRequest struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	PathStyle bool   `json:"path_style"`
}

// S3TestResult representa o resultado do teste de conexão S3.
type S3TestResult struct {
	Connected bool   `json:"connected"`
	Details   string `json:"Details,omitempty"`
}

// HmacConfigRequest representa a requisição para configuração de HMAC.
//
// O campo é `hmac_key`, e é o único: é o contrato histórico da rota
// (`41bc8e2^:handlers.go:6767`, struct `hmacConfigStruct`). Os campos
// `enabled`/`key`/`secret` que estavam aqui nasceram com o stub da F151,
// nunca foram lidos por nada em produção e nunca existiram no fio.
type HmacConfigRequest struct {
	HmacKey string `json:"hmac_key"`
}

// HmacConfigResult representa o resultado de operação de HMAC.
//
// Enabled reporta o estado DEPOIS da operação — true quando a chave ficou
// gravada, false quando foi revogada —, não um eco do pedido.
type HmacConfigResult struct {
	Details string `json:"Details,omitempty"`
	Enabled bool   `json:"Enabled,omitempty"`
}

// HmacConfigView é a resposta de leitura de `GET /hmac/config`.
//
// HmacKey é MASCARADO por desenho: `""` quando não há chave e
// MaskedHmacKey quando há. O valor nunca sai, nem em claro nem cifrado —
// devolvê-lo transformaria uma leitura de configuração num vazamento do
// segredo que assina os webhooks (`41bc8e2^:handlers.go:6825`).
type HmacConfigView struct {
	HmacKey string `json:"hmac_key"`
}

// MaskedHmacKey é o que `GET /hmac/config` devolve no lugar da chave quando
// existe uma configurada. Constante, e não literal repetido entre o use case
// e o teste que o trava: são os dois lados da mesma regra.
const MaskedHmacKey = "***"

// MinHmacKeyLength é o piso de tamanho da chave HMAC em caracteres, herdado do
// handler histórico (`41bc8e2^:handlers.go:6785`).
const MinHmacKeyLength = 32

// ProxyConfigRequest representa a requisição para configuração de Proxy.
type ProxyConfigRequest struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Auth    string `json:"auth,omitempty"`
}

// ProxyConfigResult representa o resultado de operação de Proxy.
type ProxyConfigResult struct {
	Details string `json:"Details,omitempty"`
	Set     bool   `json:"Set,omitempty"`
}
