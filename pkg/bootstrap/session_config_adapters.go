package bootstrap

import (
	"context"
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

// Fields of the userinfo entries that the session configuration touches.
//
// The same strings are written by ensureUserInfoCached (user_info_cache.go:88),
// by connectOnStartup (lifecycle.go:107) and by middleware.AuthAlice
// (auth.go:200), and "History" is read by saveMessageHistory
// (eventhandler_message.go:266) and by GetChatHistoryHandler
// (handler_chat_history.go:58). They are constants for that reason: a
// divergent literal here would produce a configuration that is saved and never
// observed.
const (
	userInfoHistoryField = "History"
	userInfoProxyField   = "Proxy"
	userInfoTokenField   = "Token"
)

// userInfoSessionCache implements appport.UserInfoHistoryCache and
// appport.UserInfoProxyCache.
//
// # Why this adapter writes to TWO caches
//
// This process keeps two userinfo caches, and the historical handler knew only
// one. They are distinct *cache.Cache values, keyed differently, and each has
// its own readers:
//
//   - appCtx.UserInfoCache (context.go:45) — keyed by USER ID, entries under
//     cache.NoExpiration. saveMessageHistory reads "History" from it to decide
//     whether an inbound message is persisted at all. A stale 0 here is
//     permanent: nothing refreshes that entry except ensureUserInfoCached,
//     which only runs on session attach.
//   - userinfocache (config.go:59) — keyed by TOKEN, entries under
//     userCacheTTL. It is what middleware.AuthAlice fills the request context
//     with, and therefore what seeds the chat-history gate
//     (handler_chat_history.go:58). A stale 0 here costs the revalidation
//     query that HOUSEKEEP F128 is about, once per request until the TTL runs
//     out.
//
// Publishing to only one of them would leave the other defect in place, and
// which one it left would depend on which cache you happened to think of.
//
// # Why the token comes from the context
//
// The token is a credential and does not belong in an application-layer
// signature. The request-scoped userinfo already carries it — AuthAlice puts
// the whole Values into the request context — so the key of the second cache
// is resolved HERE, at the edge, from the ctx the use case already had.
type userInfoSessionCache struct{}

var (
	_ appport.UserInfoHistoryCache = userInfoSessionCache{}
	_ appport.UserInfoProxyCache   = userInfoSessionCache{}
)

// SetHistory publishes the post-write history limit into both caches.
func (c userInfoSessionCache) SetHistory(ctx context.Context, userID string, history int) {
	c.publish(ctx, userID, userInfoHistoryField, strconv.Itoa(history))
}

// SetProxy publishes the post-write proxy URL into both caches.
func (c userInfoSessionCache) SetProxy(ctx context.Context, userID, proxyURL string) {
	c.publish(ctx, userID, userInfoProxyField, proxyURL)
}

// publish updates field in both userinfo caches.
func (c userInfoSessionCache) publish(ctx context.Context, userID, field, value string) {
	c.publishByUserID(userID, field, value)
	c.publishByToken(ctx, userID, field, value)
}

// publishByUserID republishes the appCtx.UserInfoCache entry of userID.
//
// A missing entry is a DELIBERATE no-op, like the HMAC and S3 adapters:
// building a partial entry from the one field this write knows about was the
// F70 defect, and the entry is rebuilt in full by ensureUserInfoCached.
func (userInfoSessionCache) publishByUserID(userID, field, value string) {
	cached, found := appCtx.UserInfoCache.Get(userID)
	if !found {
		log.Debug().Str("userid", userID).Str("field", field).
			Msg("no cached user info while updating the session configuration; the next miss loads it from the database")
		return
	}
	values, ok := cached.(Values)
	if !ok {
		log.Error().Str("userid", userID).Str("field", field).
			Msg("cached user info has an unexpected type; session configuration not published to the user-id cache")
		return
	}
	appCtx.UserInfoCache.Set(userID, withField(values, field, value), cache.NoExpiration)
	log.Info().Str("userid", userID).Str("field", field).
		Msg("user info cache updated with the session configuration")
}

// publishByToken republishes the authentication cache entry of this request.
//
// This is the half that closes F128: the entry it refreshes is the one
// AuthAlice hands to the next request, which is where the chat-history gate
// reads its seed value from.
//
// The REMAINING TTL is preserved rather than reset, and never replaced by
// cache.NoExpiration. That is not tidiness: userCacheTTL is what bounds how
// long a token survives in this cache after the users row behind it changes,
// so an entry written here without an expiry would keep authenticating a
// revoked token for the life of the process.
func (userInfoSessionCache) publishByToken(ctx context.Context, userID, field, value string) {
	requestInfo, ok := ctx.Value(appport.UserInfoKey).(Values)
	if !ok {
		log.Debug().Str("userid", userID).Str("field", field).
			Msg("request carries no user info; session configuration not published to the token cache")
		return
	}
	token := requestInfo.Get(userInfoTokenField)
	if token == "" {
		log.Debug().Str("userid", userID).Str("field", field).
			Msg("request user info carries no token; session configuration not published to the token cache")
		return
	}

	cached, expiration, found := userinfocache.GetWithExpiration(token)
	if !found {
		log.Debug().Str("userid", userID).Str("field", field).
			Msg("token not in the authentication cache; the next request loads it from the database")
		return
	}
	values, ok := cached.(Values)
	if !ok {
		log.Error().Str("userid", userID).Str("field", field).
			Msg("cached user info has an unexpected type; session configuration not published to the token cache")
		return
	}

	ttl := cache.NoExpiration
	if !expiration.IsZero() {
		ttl = time.Until(expiration)
		if ttl <= 0 {
			log.Debug().Str("userid", userID).Str("field", field).
				Msg("authentication cache entry already expired; not republishing it")
			return
		}
	}
	userinfocache.Set(token, withField(values, field, value), ttl)
	log.Info().Str("userid", userID).Str("field", field).
		Msg("authentication cache updated with the session configuration")
}

// withField returns a COPY of values with field set. Copying, and not mutating
// in place, because the map inside a cached Values is shared with whatever
// goroutine is already holding that entry — the request being served right
// now, among others.
func withField(values Values, field, value string) Values {
	updated := Values{M: make(map[string]string, len(values.M)+1)}
	for k, v := range values.M {
		updated.M[k] = v
	}
	updated.M[field] = value
	return updated
}
