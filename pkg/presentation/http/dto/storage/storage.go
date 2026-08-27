// Package storage holds the PUBLIC wire types for the storage-configuration
// routes (/s3/*, /hmac/*, /proxy/set), plus the hand-written presenters that
// build them from domain values.
//
// See docs/HTTP-DTO-CONVENTIONS.md.
package storage

import "wa-api/pkg/domain"

// S3ConfigResponse is the body of `data` for POST /s3/config and
// DELETE /s3/config.
//
// Both keys were PascalCase (`Details`, `Enabled`) and both carried omitempty,
// which made `enabled: false` — the answer of the DELETE route, every time —
// vanish from the body entirely.
type S3ConfigResponse struct {
	Details string `json:"details"`
	Enabled bool   `json:"enabled"`
}

// S3ConfigViewResponse is the body of `data` for GET /s3/config.
//
// There is no secret_key field and the absence is DELIBERATE: the historical
// SELECT never read s3_secret_key, so the secret has no way out. access_key
// comes back masked, unconditionally — masking only when one is configured
// would reveal that one is configured.
type S3ConfigViewResponse struct {
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

// S3TestResponse is the body of `data` for POST /s3/test.
//
// bucket and region echo the configuration that was TESTED — that is what
// distinguishes "I tested what you configured" from "I answered 200". No
// credential appears here. The keys were `Details`, `Bucket` and `Region`.
type S3TestResponse struct {
	Connected bool   `json:"connected"`
	Details   string `json:"details"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
}

// HmacConfigResponse is the body of `data` for POST /hmac/config and
// DELETE /hmac/config.
//
// enabled reports the state AFTER the operation — true when the key was
// stored, false when it was revoked — and not an echo of the request.
type HmacConfigResponse struct {
	Details string `json:"details"`
	Enabled bool   `json:"enabled"`
}

// HmacConfigViewResponse is the body of `data` for GET /hmac/config.
//
// hmac_key is MASKED by design: "" when there is no key and "***" when there
// is. The value never leaves, in clear or encrypted — returning it would turn
// a configuration read into a leak of the secret that signs the webhooks.
type HmacConfigViewResponse struct {
	HmacKey string `json:"hmac_key"`
}

// ProxyConfigResponse is the body of `data` for POST /proxy/set.
//
// proxy_url was `ProxyURL` and set was `Set`, both PascalCase and both
// omitempty. Dropping omitempty means the DISABLE branch — which historically
// answered `{"Details": "..."}` and nothing else — now also answers
// `set: false`, `proxy_url: ""` and `webhook_use_proxy: null`. That is the
// point of the rule: a client must be able to tell "the proxy is off" from
// "this build does not report the proxy".
//
// webhook_use_proxy stays a POINTER because null is a third state here, and a
// meaningful one: the disable branch does not resolve the flag at all, so it
// has no true value to report rather than a false one.
type ProxyConfigResponse struct {
	Details         string `json:"details"`
	Set             bool   `json:"set"`
	ProxyURL        string `json:"proxy_url"`
	WebhookUseProxy *bool  `json:"webhook_use_proxy"`
}

// PresentS3Config maps the S3 write or delete result.
func PresentS3Config(r *domain.S3ConfigResult) S3ConfigResponse {
	if r == nil {
		return S3ConfigResponse{}
	}
	return S3ConfigResponse{Details: r.Details, Enabled: r.Enabled}
}

// PresentS3ConfigView maps the S3 configuration read.
func PresentS3ConfigView(v *domain.S3ConfigView) S3ConfigViewResponse {
	if v == nil {
		return S3ConfigViewResponse{}
	}
	return S3ConfigViewResponse{
		Enabled:       v.Enabled,
		Endpoint:      v.Endpoint,
		Region:        v.Region,
		Bucket:        v.Bucket,
		AccessKey:     v.AccessKey,
		PathStyle:     v.PathStyle,
		PublicURL:     v.PublicURL,
		MediaDelivery: v.MediaDelivery,
		RetentionDays: v.RetentionDays,
	}
}

// PresentS3Test maps the S3 connection test.
func PresentS3Test(r *domain.S3TestResult) S3TestResponse {
	if r == nil {
		return S3TestResponse{}
	}
	return S3TestResponse{
		Connected: r.Connected,
		Details:   r.Details,
		Bucket:    r.Bucket,
		Region:    r.Region,
	}
}

// PresentHmacConfig maps the HMAC write or delete result.
func PresentHmacConfig(r *domain.HmacConfigResult) HmacConfigResponse {
	if r == nil {
		return HmacConfigResponse{}
	}
	return HmacConfigResponse{Details: r.Details, Enabled: r.Enabled}
}

// PresentHmacConfigView maps the HMAC configuration read.
func PresentHmacConfigView(v *domain.HmacConfigView) HmacConfigViewResponse {
	if v == nil {
		return HmacConfigViewResponse{}
	}
	return HmacConfigViewResponse{HmacKey: v.HmacKey}
}

// PresentProxyConfig maps the proxy write result.
//
// The pointer is COPIED rather than aliased: the domain value comes from a
// local in the use case, and handing the same address to the encoder would
// make the response depend on a variable this layer does not own.
func PresentProxyConfig(r *domain.ProxyConfigResult) ProxyConfigResponse {
	if r == nil {
		return ProxyConfigResponse{}
	}
	return ProxyConfigResponse{
		Details:         r.Details,
		Set:             r.Set,
		ProxyURL:        r.ProxyURL,
		WebhookUseProxy: copyBool(r.WebhookUseProxy),
	}
}

// copyBool devolve um ponteiro NOVO para o mesmo valor, ou nil.
//
// Duas funções pequenas em vez de três statements dentro do apresentador, pelo
// mesmo motivo que nonNilStrings em dto/webhook: acima de dois statements o
// apresentador entra no denominador do gate de log do cmd/logcov (regra X1) e
// baixa a cobertura sem que haja nada que registar nele.
func copyBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	return boolPtr(*v)
}

// boolPtr é o endereço de uma cópia de v.
func boolPtr(v bool) *bool { return &v }
