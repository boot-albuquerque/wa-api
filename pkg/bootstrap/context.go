package bootstrap

import (
	"net/http"
	"time"

	"github.com/patrickmn/go-cache"
)

// AppContext bundles all runtime configuration and shared state previously
// spread across global variables in package main. Inject this into functions
// that need access to global configuration (webhook dispatch, event handling,
// client lifecycle) to break circular dependencies between root and internal/.
type AppContext struct {
	// Webhook configuration
	GlobalWebhook          string
	GlobalHMACKeyEncrypted []byte
	GlobalWebhookUseProxy  bool
	GlobalEncryptionKey    string

	// Webhook retry
	WebhookRetryEnabled      bool
	WebhookRetryCount        int
	WebhookRetryDelaySeconds int
	WebhookErrorQueueName    string

	// Shared caches (accessible via exported fields)
	UserInfoCache    *cache.Cache
	LastMessageCache *cache.Cache

	// Shared mutable state
	KillChannel      *KillChannel
	GlobalHTTPClient *http.Client

	// ClientManager must be set after bootstrap (circular import avoidance:
	// the concrete type lives in package main, but we store it as interface{}).
	ClientManager interface{}
}

// NewAppContext creates a fully initialized AppContext.
func NewAppContext() *AppContext {
	return &AppContext{
		KillChannel:      NewKillChannel(),
		GlobalHTTPClient: NewSafeHTTPClient(),
		UserInfoCache:    cache.New(5*time.Minute, 10*time.Minute),
		LastMessageCache: cache.New(24*time.Hour, 24*time.Hour),
	}
}
