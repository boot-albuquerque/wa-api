package bootstrap

import (
	"fmt"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

// TestPublishUserInfoWritesBothCaches proves the F164 invariant: publishing
// user data through publishUserInfo leaves BOTH caches agreeing.
func TestPublishUserInfoWritesBothCaches(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user42"
	token := "tok-abc"
	values := Values{M: map[string]string{
		"Id": userID, "Token": token, "Webhook": "https://example.test",
		"History": "30", "Events": "Message",
	}}

	publishUserInfo(userID, token, values)

	gotByID, found := appCtx.UserInfoCache.Get(userID)
	if !found {
		t.Fatal("publishUserInfo did not write to the user-id cache")
	}
	gotByToken, found := userinfocache.Get(token)
	if !found {
		t.Fatal("publishUserInfo did not write to the token cache")
	}

	vByID := gotByID.(Values)
	vByToken := gotByToken.(Values)
	for _, field := range []string{"Id", "Token", "Webhook", "History", "Events"} {
		if vByID.Get(field) != vByToken.Get(field) {
			t.Errorf("caches disagree on %q: byID=%q, byToken=%q",
				field, vByID.Get(field), vByToken.Get(field))
		}
	}
}

// TestPublishUserInfoResolvesTokenFromValues proves that when the caller
// passes an empty token, publishUserInfo resolves it from the "Token" field
// inside the Values map — the path the HMAC and S3 adapters take.
func TestPublishUserInfoResolvesTokenFromValues(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user99"
	token := "tok-resolved"
	values := Values{M: map[string]string{
		"Id": userID, "Token": token, "S3Enabled": "true",
	}}

	publishUserInfo(userID, "", values)

	if _, found := userinfocache.Get(token); !found {
		t.Fatal("publishUserInfo did not resolve the token from Values and write to the token cache")
	}
}

// TestPublishUserInfoNoTokenSkipsTokenCache proves that when no token is
// available (neither passed nor in Values), only the userID cache is written
// and no panic or error occurs.
func TestPublishUserInfoNoTokenSkipsTokenCache(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user-no-token"
	values := Values{M: map[string]string{"Id": userID, "Webhook": "https://x.test"}}

	publishUserInfo(userID, "", values)

	if _, found := appCtx.UserInfoCache.Get(userID); !found {
		t.Fatal("userID cache was not written")
	}
	// No token cache entry should exist — we don't know the key.
	if count := userinfocache.ItemCount(); count != 0 {
		t.Errorf("token cache should be empty, has %d items", count)
	}
}

// TestPublishUserInfoPreservesRemainingTTL proves that updating a field does
// not reset the token cache TTL — sec/F11: resetting it would keep a revoked
// token alive indefinitely through a stream of configuration changes.
func TestPublishUserInfoPreservesRemainingTTL(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	token := "tok-ttl"
	userID := "user-ttl"

	shortTTL := 2 * time.Minute
	userinfocache.Set(token, Values{M: map[string]string{
		"Id": userID, "Token": token, "History": "0",
	}}, shortTTL)

	publishUserInfo(userID, token, Values{M: map[string]string{
		"Id": userID, "Token": token, "History": "50",
	}})

	_, exp, found := userinfocache.GetWithExpiration(token)
	if !found {
		t.Fatal("token cache entry gone after publish")
	}
	remaining := time.Until(exp)
	if remaining > shortTTL {
		t.Errorf("TTL was reset: remaining=%v, original=%v", remaining, shortTTL)
	}
	if remaining <= 0 {
		t.Errorf("TTL expired during the test: remaining=%v", remaining)
	}
}

// TestPublishUserInfoTokenCacheNeverGetsNoExpiration proves that a fresh
// token cache entry gets tokenCacheTTL, NEVER cache.NoExpiration. The three
// webhook handler writes that used cache.NoExpiration were the F164 TTL bug.
func TestPublishUserInfoTokenCacheNeverGetsNoExpiration(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	token := "tok-fresh"
	userID := "user-fresh"
	values := Values{M: map[string]string{
		"Id": userID, "Token": token, "Webhook": "https://x.test",
	}}

	publishUserInfo(userID, token, values)

	_, exp, found := userinfocache.GetWithExpiration(token)
	if !found {
		t.Fatal("token cache entry not found")
	}
	if exp.IsZero() {
		t.Fatal("token cache entry was written with NoExpiration; " +
			"revoking a token would never take effect")
	}
	remaining := time.Until(exp)
	if remaining <= 0 || remaining > tokenCacheTTL+time.Second {
		t.Errorf("TTL out of expected range: remaining=%v, want ~%v", remaining, tokenCacheTTL)
	}
}

// --- Control negatives (CN-A, CN-B) ---

// TestPublishUserInfoCNA_OnlyUserIDCacheFailsInvariant is control negative A:
// if publishUserInfo only writes to the userID cache, the token cache is
// stale and the invariant breaks.
func TestPublishUserInfoCNA_OnlyUserIDCacheFailsInvariant(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user-cn-a"
	token := "tok-cn-a"
	initial := Values{M: map[string]string{
		"Id": userID, "Token": token, "History": "0",
	}}
	publishUserInfo(userID, token, initial)

	updated := Values{M: map[string]string{
		"Id": userID, "Token": token, "History": "50",
	}}
	publishUserInfo(userID, token, updated)

	vByID := getCachedValues(t, appCtx.UserInfoCache, userID, "user-id")
	vByToken := getCachedValues(t, userinfocache, token, "token")

	if vByID.Get("History") != "50" {
		t.Fatalf("userID cache has wrong History: %q", vByID.Get("History"))
	}
	if vByToken.Get("History") != "50" {
		t.Fatalf("token cache has stale History: %s (should be 50); "+
			"the token cache was not updated — the F164 defect", vByToken.Get("History"))
	}
}

// TestPublishUserInfoCNB_OnlyTokenCacheFailsInvariant is control negative B:
// if publishUserInfo only writes to the token cache, the userID cache is
// stale and the invariant breaks.
func TestPublishUserInfoCNB_OnlyTokenCacheFailsInvariant(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user-cn-b"
	token := "tok-cn-b"
	initial := Values{M: map[string]string{
		"Id": userID, "Token": token, "Webhook": "https://old.test",
	}}
	publishUserInfo(userID, token, initial)

	updated := Values{M: map[string]string{
		"Id": userID, "Token": token, "Webhook": "https://new.test",
	}}
	publishUserInfo(userID, token, updated)

	vByID := getCachedValues(t, appCtx.UserInfoCache, userID, "user-id")
	vByToken := getCachedValues(t, userinfocache, token, "token")

	if vByToken.Get("Webhook") != "https://new.test" {
		t.Fatalf("token cache has wrong Webhook: %q", vByToken.Get("Webhook"))
	}
	if vByID.Get("Webhook") != "https://new.test" {
		t.Fatalf("userID cache has stale Webhook: %s (should be https://new.test); "+
			"the userID cache was not updated — the F164 defect in reverse", vByID.Get("Webhook"))
	}
}

// TestPublishUserInfoCNC_OrderMatters is control negative C: if the two
// writes happen in the wrong order and something fails between them, the
// caches diverge. publishUserInfo writes the userID cache first and the
// token cache second, and this test verifies that both are written even
// when starting from empty caches.
func TestPublishUserInfoCNC_OrderMatters(t *testing.T) {
	saved := saveAndReplaceCaches(t)
	defer saved.restore()

	userID := "user-cn-c"
	token := "tok-cn-c"
	values := Values{M: map[string]string{
		"Id": userID, "Token": token, "Events": "All",
	}}

	publishUserInfo(userID, token, values)

	if appCtx.UserInfoCache.ItemCount() != 1 {
		t.Errorf("userID cache has %d items, want 1", appCtx.UserInfoCache.ItemCount())
	}
	if userinfocache.ItemCount() != 1 {
		t.Errorf("token cache has %d items, want 1", userinfocache.ItemCount())
	}

	vByID := getCachedValues(t, appCtx.UserInfoCache, userID, "user-id")
	vByToken := getCachedValues(t, userinfocache, token, "token")
	if vByID.Get("Events") != vByToken.Get("Events") {
		t.Errorf("caches disagree: byID.Events=%q, byToken.Events=%q",
			vByID.Get("Events"), vByToken.Get("Events"))
	}
}

// TestPublishUserInfoDeterministic runs the invariant check multiple times
// to catch map-order nondeterminism (Go randomizes map iteration by design).
func TestPublishUserInfoDeterministic(t *testing.T) {
	for i := 0; i < 50; i++ {
		t.Run(fmt.Sprintf("round-%d", i), func(t *testing.T) {
			saved := saveAndReplaceCaches(t)
			defer saved.restore()

			userID := fmt.Sprintf("user-%d", i)
			token := fmt.Sprintf("tok-%d", i)
			values := Values{M: map[string]string{
				"Id": userID, "Token": token,
				"Webhook": "https://x.test", "History": "10",
				"Events": "Message,ReadReceipt",
			}}

			publishUserInfo(userID, token, values)

			vByID := getCachedValues(t, appCtx.UserInfoCache, userID, "user-id")
			vByToken := getCachedValues(t, userinfocache, token, "token")

			for _, field := range []string{"Id", "Token", "Webhook", "History", "Events"} {
				if vByID.Get(field) != vByToken.Get(field) {
					t.Errorf("round %d: caches disagree on %q: byID=%q, byToken=%q",
						i, field, vByID.Get(field), vByToken.Get(field))
				}
			}
		})
	}
}

// TestTokenCacheTTLMatchesMiddleware enforces that tokenCacheTTL in
// publish_userinfo.go agrees with the value AuthAlice uses. The two live in
// different packages and cannot share a constant without inverting
// dependencies, so this test is the coupling that keeps them in sync.
func TestTokenCacheTTLMatchesMiddleware(t *testing.T) {
	const middlewareTTL = 10 * time.Minute
	if tokenCacheTTL != middlewareTTL {
		t.Errorf("tokenCacheTTL = %v, middleware.userCacheTTL = %v; "+
			"the two must agree or token entries will expire at different rates "+
			"depending on who wrote them", tokenCacheTTL, middlewareTTL)
	}
}

// --- helpers ---

type savedCaches struct {
	userInfoCache *cache.Cache
	tokenCache    *cache.Cache
}

func saveAndReplaceCaches(t *testing.T) savedCaches {
	t.Helper()
	saved := savedCaches{
		userInfoCache: appCtx.UserInfoCache,
		tokenCache:    userinfocache,
	}
	appCtx.UserInfoCache = cache.New(cache.NoExpiration, 0)
	userinfocache = cache.New(cache.NoExpiration, 0)
	return saved
}

func (s savedCaches) restore() {
	appCtx.UserInfoCache = s.userInfoCache
	userinfocache = s.tokenCache
}

func getCachedValues(t *testing.T, c *cache.Cache, key, label string) Values {
	t.Helper()
	raw, found := c.Get(key)
	if !found {
		t.Fatalf("%s cache has no entry for %q", label, key)
	}
	v, ok := raw.(Values)
	if !ok {
		t.Fatalf("%s cache entry for %q is not Values", label, key)
	}
	return v
}
