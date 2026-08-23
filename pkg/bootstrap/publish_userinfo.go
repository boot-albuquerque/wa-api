package bootstrap

import (
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// tokenCacheTTL is the TTL for NEW entries in the authentication cache.
//
// It mirrors middleware.userCacheTTL (auth.go:70) by value, not by import:
// middleware lives in pkg/presentation, and importing it from pkg/bootstrap
// would invert the dependency direction. The two constants MUST agree; a
// test in publish_userinfo_test.go enforces that.
const tokenCacheTTL = 10 * time.Minute

// publishUserInfo writes values to BOTH userinfo caches, ensuring they agree.
//
// This is the SINGLE POINT through which all user-info mutations flow.
// Before this function, the ten write sites (six by userID, four by token)
// each touched one cache and left the other stale — the F164 defect.
//
//   - appCtx.UserInfoCache is keyed by userID with cache.NoExpiration.
//   - userinfocache is keyed by token. Existing entries preserve their
//     REMAINING TTL; new entries get tokenCacheTTL. Entries are NEVER
//     written with cache.NoExpiration: that is the safety net that bounds
//     how long a revoked token keeps authenticating (sec/F11, auth.go:66).
//
// When token is empty, it is resolved from values.Get("Token"). If neither
// source yields a token, only the userID cache is written, and a debug log
// says why — this is expected for paths that run before the user has
// authenticated (e.g. connectOnStartup for sessions that never completed
// pairing).
func publishUserInfo(userID, token string, values Values) {
	appCtx.UserInfoCache.Set(userID, values, cache.NoExpiration)

	if token == "" {
		token = values.Get(userInfoTokenField)
	}
	if token == "" {
		log.Debug().Str("userid", userID).
			Msg("publishUserInfo: no token available; only the user-id cache was written")
		return
	}

	ttl := resolveTokenCacheTTL(token)
	userinfocache.Set(token, values, ttl)
}

// resolveTokenCacheTTL returns the TTL to use for a token cache entry.
//
// If the entry already exists and has a non-zero expiration, the REMAINING
// time is preserved — resetting the TTL on every field update would keep a
// revoked token alive indefinitely through a stream of configuration
// changes. If no entry exists, tokenCacheTTL is used.
func resolveTokenCacheTTL(token string) time.Duration {
	_, expiration, found := userinfocache.GetWithExpiration(token)
	if found && !expiration.IsZero() {
		remaining := time.Until(expiration)
		if remaining > 0 {
			return remaining
		}
	}
	return tokenCacheTTL
}
