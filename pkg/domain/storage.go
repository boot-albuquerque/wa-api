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

// S3ConfigView é a resposta de leitura de `GET /s3/config`.
//
// Os campos e as tags são os do SELECT histórico
// (`41bc8e2^:handlers.go:6322`), e a ausência de `secret_key` é DELIBERADA:
// aquele SELECT nunca leu `s3_secret_key`, então o segredo não tem por onde
// sair. AccessKey vem mascarada com MaskedS3AccessKey.
type S3ConfigView struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"access_key"`
	PathStyle     bool   `json:"path_style"`
	PublicURL     string `json:"public_url"`
	MediaDelivery string `json:"media_delivery"`
	RetentionDays int    `json:"retention_days"`
}

// MaskedS3AccessKey é o que `GET /s3/config` devolve no lugar da access key.
// O histórico mascara INCONDICIONALMENTE (`config.AccessKey = "***"`), e não
// só quando há uma configurada: mascarar por presença revelaria a presença.
const MaskedS3AccessKey = "***"

// Os três valores aceitos por `media_delivery`, mais o default.
//
// Constantes porque as MESMAS strings são a validação do use case, o default
// da coluna e a asserção do teste que trava o contrato (ADR-0004). O
// histórico as tinha como literais em três lugares
// (`41bc8e2^:handlers.go:6232`, `:6240`, e o UPDATE do DELETE em `:6497`).
const (
	MediaDeliveryBase64 = "base64"
	MediaDeliveryS3     = "s3"
	MediaDeliveryBoth   = "both"

	// DefaultMediaDelivery é o valor que um `media_delivery` vazio assume.
	DefaultMediaDelivery = MediaDeliveryBase64
)

// IsValidMediaDelivery reporta se v é um dos três valores aceitos. O vazio
// NÃO é aceito aqui: quem o trata é o caller, substituindo-o pelo default.
func IsValidMediaDelivery(v string) bool {
	return v == MediaDeliveryBase64 || v == MediaDeliveryS3 || v == MediaDeliveryBoth
}

// S3TestResult representa o resultado do teste de conexão S3.
//
// Bucket e Region ecoam a configuração TESTADA — é o corpo histórico
// (`41bc8e2^:handlers.go:6455`), e é o que distingue "testei o que você
// configurou" de "respondi 200". Nenhuma credencial aparece aqui.
type S3TestResult struct {
	Connected bool   `json:"connected"`
	Details   string `json:"Details,omitempty"`
	Bucket    string `json:"Bucket,omitempty"`
	Region    string `json:"Region,omitempty"`
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
