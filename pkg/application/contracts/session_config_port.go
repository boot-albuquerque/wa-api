package port

import "context"

// HistoryConfigStore is the persistence port of POST /session/history, over
// the single column users.history (migration `add_history`,
// pkg/infra/db/migrations.go).
//
// That column is not decorative: it is what the chat-history gate revalidates
// against (GetChatHistoryUseCase.Execute, cachedHistory == 0 branch) and what
// saveMessageHistory (pkg/bootstrap/eventhandler_message.go:266) reads to
// decide whether an inbound message is persisted at all. Writing anywhere else
// would light up neither.
type HistoryConfigStore interface {
	// SaveHistoryLimit writes users.history for userID. The value is already
	// validated as non-negative by the use case.
	SaveHistoryLimit(ctx context.Context, userID string, history int) error
}

// ProxyConfigStore is the persistence port of POST /session/proxy, over the
// two columns the historical UPDATE wrote — users.proxy_url and
// users.webhook_use_proxy (`41bc8e2^:handlers.go:6155`).
//
// The two columns travel together because the historical handler never wrote
// one without the other: `webhook_use_proxy` decides whether webhook delivery
// is routed through the proxy the same statement is setting, and splitting
// them into two writes would create a window where the routing flag points at
// a proxy URL that is not there yet.
type ProxyConfigStore interface {
	// LoadWebhookUseProxy reads COALESCE(webhook_use_proxy, true) for userID.
	//
	// It exists because the historical handler PRESERVED the stored per-user
	// setting whenever the request omitted `webhook_use_proxy`
	// (`41bc8e2^:handlers.go:6119`): without this read, every proxy write
	// would silently reset the flag to the global default.
	LoadWebhookUseProxy(ctx context.Context, userID string) (bool, error)

	// SaveProxyConfig writes proxy_url and webhook_use_proxy for userID.
	// Disabling the proxy is the same statement with an EMPTY proxyURL, which
	// is exactly what the historical disable branch wrote (`proxy_url = ''`) —
	// not a NULL and not a deleted row.
	SaveProxyConfig(ctx context.Context, userID, proxyURL string, webhookUseProxy bool) error
}

// UserInfoHistoryCache publishes the post-write history limit into the
// userinfo caches.
//
// This is the port that closes HOUSEKEEP F128, and the reason it takes a ctx
// is not decoration: THIS PROCESS HAS TWO userinfo caches, keyed differently,
// and the write path has to reach both.
//
//   - appCtx.UserInfoCache (pkg/bootstrap/context.go:45) is keyed by USER ID
//     and holds entries under cache.NoExpiration. saveMessageHistory reads
//     "History" from it.
//   - the authentication cache (pkg/bootstrap/config.go:59, handed to
//     middleware.AuthAlice) is keyed by TOKEN and has a TTL. It is what fills
//     the request context, and therefore what seeds the chat-history gate.
//
// The token is a credential and must not appear in an application-layer
// signature; the request-scoped userinfo already carries it, so the ADAPTER
// resolves the second key from ctx. That is why ctx is a parameter here and
// not in UserInfoHmacCache / UserInfoS3Cache, whose fields only have
// user-id-keyed readers.
type UserInfoHistoryCache interface {
	// SetHistory publishes history for userID. A user with no cached entry in
	// a given cache is a no-op FOR THAT CACHE: building a partial entry was
	// the F70 defect.
	SetHistory(ctx context.Context, userID string, history int)
}

// UserInfoProxyCache publishes the post-write proxy URL into the same two
// userinfo caches, under the "Proxy" field.
//
// Measured, so that nobody reads more into it than is there: in this tree
// NOTHING reads "Proxy" back out of either cache today (`grep -rn 'Get("Proxy")'
// pkg/` finds no call site; the field is only ever written, by
// ensureUserInfoCached, connectOnStartup and AuthAlice). It is published
// because the historical handler published it and because the field exists in
// both entries — not because a reader was found.
type UserInfoProxyCache interface {
	// SetProxy publishes proxyURL for userID. An empty proxyURL is the
	// disabled state, which is what the historical disable branch cached.
	SetProxy(ctx context.Context, userID, proxyURL string)
}
