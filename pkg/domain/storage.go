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
type HmacConfigRequest struct {
	Enabled bool   `json:"enabled"`
	Key     string `json:"key"`
	Secret  string `json:"secret"`
}

// HmacConfigResult representa o resultado de operação de HMAC.
type HmacConfigResult struct {
	Details string `json:"Details,omitempty"`
	Enabled bool   `json:"Enabled,omitempty"`
}

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
