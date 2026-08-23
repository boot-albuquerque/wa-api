package bootstrap

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

// userInfoRepublisher implements appport.UserInfoRepublisher.
//
// It INVALIDATES rather than patches. The admin edit can change name, token,
// webhook, expiration, events, proxy, S3 and history in a single call, so
// republishing field by field would need this adapter to know every field the
// use case touched — and a field forgotten here is a stale value that never
// expires (F200). Dropping the entries makes the next read go to the database,
// which is the only place that is certainly right.
type userInfoRepublisher struct{ db *sqlx.DB }

var _ appport.UserInfoRepublisher = userInfoRepublisher{}

// RepublishUser drops the user-id entry and every token entry of this user.
//
// The token entries are found by SCANNING the token cache for entries whose
// user id matches, and not by being told which token to drop. That is
// deliberate: the caller would have to read the PREVIOUS token before the
// write to know it, and any token it failed to pass would keep authenticating
// (F201). Scanning finds them all, including tokens left by earlier edits.
func (r userInfoRepublisher) RepublishUser(ctx context.Context, userID string) {
	appCtx.UserInfoCache.Delete(userID)

	removidos := 0
	for token, item := range userinfocache.Items() {
		values, ok := item.Object.(Values)
		if !ok {
			continue
		}
		if values.Get(userInfoIDField) == userID {
			userinfocache.Delete(token)
			removidos++
		}
	}

	// Reload eagerly so the very next event handler — which reads the cache
	// and NOT the database — sees the new values without waiting for a miss.
	if err := ensureUserInfoCached(r.db, userID); err != nil {
		log.Warn().Err(err).Str("userid", userID).
			Msg("could not reload user info after an edit; the next cache miss loads it from the database")
		return
	}
	log.Info().Str("userid", userID).Int("token_entries_dropped", removidos).
		Msg("user info republished after edit")
}
