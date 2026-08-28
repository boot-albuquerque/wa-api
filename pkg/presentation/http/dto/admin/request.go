package admin

import (
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// The request bodies of /admin/users.
//
// EVERY key here is snake_case, request bodies included — the naming rule of
// docs/HTTP-DTO-CONVENTIONS.md §8 is not a response-only rule. That is a
// BREAKING change for this family and it is deliberate: the routes used to
// read `proxyConfig`, `s3Config`, `hmacKey`, `proxyUrl`, `webhookUseProxy`,
// `accessKey`, `secretKey`, `pathStyle`, `publicUrl`, `mediaDelivery` and
// `retentionDays` in camelCase while answering in snake_case, and the
// mismatch was itself a measured defect (F210) that had been patched by
// accepting BOTH spellings on the way in. Aligning the two names removes the
// need for the alias, and the hard-cutover rule forbids keeping the old
// spelling alongside the new one.
//
// A body that still uses the old names now loses those fields. It does not
// pass unnoticed: decodeRequest reports unknown fields, which is a warning
// log by default and a 400 with WA_API_STRICT_UNKNOWN_FIELDS on.

const (
	// missingNameOrTokenCode is the same code AddUserUseCase returns for the
	// same condition. Rejecting earlier must not change what the client
	// branches on.
	missingNameOrTokenCode = "missing_name_or_token"
	missingNameOrTokenMsg  = "name and token are required"

	// invalidEngineCode is returned when `engine` is neither empty nor one
	// of domain.EngineNoise / domain.EngineWaHeadless.
	invalidEngineCode = "invalid_engine"
)

// ProxyConfigRequest is the `proxy_config` object of a create/edit body.
type ProxyConfigRequest struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxy_url"`

	// Pointer: "did not mention it" has to stay distinct from "sent false",
	// because absent means "keep the default of true" and false means "stop
	// sending webhooks through the proxy".
	WebhookUseProxy *bool `json:"webhook_use_proxy"`
}

// ToDomain builds the internal configuration value.
func (r *ProxyConfigRequest) ToDomain() *domain.ProxyConfig {
	if r == nil {
		return nil
	}
	return &domain.ProxyConfig{
		Enabled:         r.Enabled,
		ProxyURL:        r.ProxyURL,
		WebhookUseProxy: r.WebhookUseProxy,
	}
}

// S3ConfigRequest is the `s3_config` object of a create/edit body.
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

// ToDomain builds the internal configuration value.
func (r *S3ConfigRequest) ToDomain() *domain.S3Config {
	if r == nil {
		return nil
	}
	return &domain.S3Config{
		Enabled:       r.Enabled,
		Endpoint:      r.Endpoint,
		Region:        r.Region,
		Bucket:        r.Bucket,
		AccessKey:     r.AccessKey,
		SecretKey:     r.SecretKey,
		PathStyle:     r.PathStyle,
		PublicURL:     r.PublicURL,
		MediaDelivery: r.MediaDelivery,
		RetentionDays: r.RetentionDays,
	}
}

// AddUserRequest is the body of POST /admin/users.
//
// The configuration blocks are POINTERS because absent has to stay distinct
// from "sent empty": the use case fills a default for a missing block, and a
// zero-valued struct would look like a request to disable everything.
type AddUserRequest struct {
	Name       string `json:"name"`
	Token      string `json:"token"`
	Webhook    string `json:"webhook"`
	Expiration int    `json:"expiration"`
	Events     string `json:"events"`
	HmacKey    string `json:"hmac_key"`
	History    int    `json:"history"`

	// Engine is the transport chosen for this session:
	// domain.EngineNoise or domain.EngineWaHeadless. Empty defaults to
	// domain.EngineNoise. Replaces decisão 94's startup-time, env-var
	// driven selection — the choice is now per-session and made by the
	// caller before the session (or its QR code) is created.
	Engine string `json:"engine"`

	ProxyConfig *ProxyConfigRequest `json:"proxy_config"`
	S3Config    *S3ConfigRequest    `json:"s3_config"`
}

// Validate refuses a body that cannot produce a domain command. It returns
// *apperr.AppError so the boundary answers 400 with a stable error.code
// instead of a generic 500.
func (r AddUserRequest) Validate() error {
	if r.Name == "" || r.Token == "" {
		return apperr.New(missingNameOrTokenCode, apperr.CategoryValidation,
			missingNameOrTokenMsg, false, nil)
	}
	if _, ok := domain.EngineValido(r.Engine, domain.EngineNoise); !ok {
		return apperr.New(invalidEngineCode, apperr.CategoryValidation,
			"engine must be \""+domain.EngineNoise+"\" or \""+domain.EngineWaHeadless+"\"", false, nil)
	}
	return nil
}

// ToDomain produces the use case input. Only called after Validate.
func (r AddUserRequest) ToDomain() domain.AddUserInput {
	engine, _ := domain.EngineValido(r.Engine, domain.EngineNoise)
	return domain.AddUserInput{
		Name:        r.Name,
		Token:       r.Token,
		Webhook:     r.Webhook,
		Expiration:  r.Expiration,
		Events:      r.Events,
		HmacKey:     r.HmacKey,
		History:     r.History,
		Engine:      engine,
		ProxyConfig: r.ProxyConfig.ToDomain(),
		S3Config:    r.S3Config.ToDomain(),
	}
}

// EditUserRequest is the body of PUT /admin/users/{id}.
//
// The user id is NOT here: it comes from the path, and a field for it in the
// body would be a second, contradictable source for the same thing. That is
// why ToDomain takes it as an argument.
//
// Every string field keeps value semantics on purpose: for this route an
// empty string has always meant "not informed", never "set it to empty"
// (F206). History is the exception and stays a POINTER (F218), because there
// 0 is a legitimate value — it disables the per-chat history limit — and with
// a plain int the API could raise the limit but never turn it off.
type EditUserRequest struct {
	Name       string `json:"name"`
	Token      string `json:"token"`
	Webhook    string `json:"webhook"`
	Expiration int    `json:"expiration"`
	Events     string `json:"events"`
	History    *int   `json:"history"`

	ProxyConfig *ProxyConfigRequest `json:"proxy_config"`
	S3Config    *S3ConfigRequest    `json:"s3_config"`
}

// Validate accepts every body: whether the change is empty is decided
// downstream, by the repository, which is the only layer that knows which
// columns a given update would touch. It answers `no_fields_to_update`.
//
// The method exists anyway so that every request DTO in this package has the
// same shape, and so that a future rule has an obvious place to live.
func (r EditUserRequest) Validate() error { return nil }

// ToDomain produces the use case input, with the id read from the path.
func (r EditUserRequest) ToDomain(userID string) domain.EditUserInput {
	return domain.EditUserInput{
		UserID:      userID,
		Name:        r.Name,
		Token:       r.Token,
		Webhook:     r.Webhook,
		Expiration:  r.Expiration,
		Events:      r.Events,
		History:     r.History,
		ProxyConfig: r.ProxyConfig.ToDomain(),
		S3Config:    r.S3Config.ToDomain(),
	}
}
