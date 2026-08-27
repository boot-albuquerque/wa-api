package storage

import "wa-api/pkg/domain"

// S3ConfigRequest is the body of POST /s3/config.
//
// Every key here was already snake_case; what changed is that they no longer
// come from a domain struct.
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

// Validate: ConfigureS3UseCase owns the media_delivery enumeration and the
// required-field taxonomy.
func (r S3ConfigRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r S3ConfigRequest) ToDomain() domain.S3ConfigRequest {
	return domain.S3ConfigRequest{
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

// HmacConfigRequest is the body of POST /hmac/config.
//
// One field, and that is the whole historical contract of the route.
type HmacConfigRequest struct {
	HmacKey string `json:"hmac_key"`
}

// Validate: ConfigureHmacUseCase owns the minimum key length.
func (r HmacConfigRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r HmacConfigRequest) ToDomain() domain.HmacConfigRequest {
	return domain.HmacConfigRequest{HmacKey: r.HmacKey}
}

// ProxyConfigRequest is the body of POST /proxy/set.
//
// WebhookUseProxy is a POINTER so that "absent" and "explicitly false" stay
// distinguishable: absent PRESERVES the stored value, and collapsing the two
// into a bool would make every proxy write zero the flag for anyone who did
// not send it.
type ProxyConfigRequest struct {
	ProxyURL        string `json:"proxy_url"`
	Enable          bool   `json:"enable"`
	WebhookUseProxy *bool  `json:"webhook_use_proxy"`
}

// Validate: SetProxyUseCase owns the accepted URL schemes and the
// missing-URL refusal.
func (r ProxyConfigRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r ProxyConfigRequest) ToDomain() domain.ProxyConfigRequest {
	return domain.ProxyConfigRequest{
		ProxyURL:        r.ProxyURL,
		Enable:          r.Enable,
		WebhookUseProxy: r.WebhookUseProxy,
	}
}
