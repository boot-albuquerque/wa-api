package bootstrap

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/infra/auth"
	"wa-api/pkg/infra/storage"
)

// Fields of the UserInfoCache entry that the S3 configuration touches.
//
// The same strings are written by ensureUserInfoCached (user_info_cache.go:93)
// and by connectOnStartup (lifecycle.go:109), and read by
// eventhandler_message.go:81-82 to decide whether an inbound media goes to S3.
// They are constants for that reason: a divergent literal here would produce a
// configuration that is saved and never observed.
const (
	userInfoS3EnabledField     = "S3Enabled"
	userInfoMediaDeliveryField = "MediaDelivery"

	userInfoS3EnabledTrue  = "true"
	userInfoS3EnabledFalse = "false"
)

// s3SecretCipher implements appport.S3SecretCipher over the process-wide
// encryption key.
type s3SecretCipher struct{}

// EncryptS3Secret wraps the ADR-0009 envelope around the same AES-GCM the rest
// of the process uses (appCtx.GlobalEncryptionKey).
func (s3SecretCipher) EncryptS3Secret(plainSecret string) (string, error) {
	return auth.EncryptS3Secret(plainSecret, appCtx.GlobalEncryptionKey)
}

// DecryptS3Secret unwraps it. A stored value without the envelope prefix is an
// error here, and is never returned as a plaintext credential.
func (s3SecretCipher) DecryptS3Secret(storedSecret string) (string, error) {
	return auth.DecryptS3Secret(storedSecret, []byte(appCtx.GlobalEncryptionKey))
}

// s3ClientManager implements appport.S3ClientManager over the process-wide
// storage.S3Manager — the SAME registry that media upload reads
// (s3MediaUploader.ProcessMediaForS3 in wiring_delegates.go:70). An adapter
// over a private manager would make the revocation test pass while production
// kept uploading.
type s3ClientManager struct{}

// InitializeS3Client registers the client of userID. cfg.SecretKey is the
// PLAINTEXT secret: the AWS SDK signs with it.
func (s3ClientManager) InitializeS3Client(userID string, cfg appport.S3ConfigRecord) error {
	return storage.GetS3Manager().InitializeS3Client(userID, &storage.S3Config{
		Enabled:       cfg.Enabled,
		Endpoint:      cfg.Endpoint,
		Region:        cfg.Region,
		Bucket:        cfg.Bucket,
		AccessKey:     cfg.AccessKey,
		SecretKey:     cfg.SecretKey,
		PathStyle:     cfg.PathStyle,
		PublicURL:     cfg.PublicURL,
		MediaDelivery: cfg.MediaDelivery,
		RetentionDays: cfg.RetentionDays,
	})
}

// RemoveClient drops the client of userID from the registry.
func (s3ClientManager) RemoveClient(userID string) {
	storage.GetS3Manager().RemoveClient(userID)
}

// TestConnection reaches the configured endpoint with the registered client.
func (s3ClientManager) TestConnection(ctx context.Context, userID string) error {
	return storage.GetS3Manager().TestConnection(ctx, userID)
}

// userInfoS3Cache implements appport.UserInfoS3Cache over appCtx.UserInfoCache.
//
// It publishes ONLY "S3Enabled" and "MediaDelivery", and this is the point
// where this fork diverges from the historical handler, which also wrote
// S3AccessKey, S3SecretKey, S3Endpoint, S3Region, S3Bucket, S3PathStyle and
// S3PublicURL into the same entry (`41bc8e2^:handlers.go:6296-6305`).
//
// The reason is measured, not aesthetic: NOTHING in this tree reads those
// seven fields back. The only consumers of the cached S3 state are
// eventhandler_message.go:81-82, which reads exactly these two. The plaintext
// credential does have one legitimate in-memory holder — storage.S3Manager,
// where the AWS SDK requires it — and a second copy in a cache.NoExpiration
// entry that nobody consumes would be a copy of a secret kept alive for no
// reader at all.
//
// A missing entry is a DELIBERATE no-op, for the same reason as the HMAC
// adapter: whoever is not cached loads from the database on the next miss
// (ensureUserInfoCached), so there is no stale value to correct, and building
// a partial entry here was the F70 defect.
type userInfoS3Cache struct{}

// SetS3Config republishes the entry of userID with the post-operation state.
func (userInfoS3Cache) SetS3Config(userID string, enabled bool, mediaDelivery string) {
	cached, found := appCtx.UserInfoCache.Get(userID)
	if !found {
		log.Debug().Str("userid", userID).
			Msg("no cached user info while updating the S3 configuration; the next miss loads it from the database")
		return
	}
	values, ok := cached.(Values)
	if !ok {
		log.Error().Str("userid", userID).
			Msg("cached user info has an unexpected type; S3 configuration not published to the cache")
		return
	}

	enabledText := userInfoS3EnabledFalse
	if enabled {
		enabledText = userInfoS3EnabledTrue
	}

	updated := Values{M: make(map[string]string, len(values.M)+2)}
	for k, v := range values.M {
		updated.M[k] = v
	}
	updated.M[userInfoS3EnabledField] = enabledText
	updated.M[userInfoMediaDeliveryField] = mediaDelivery

	publishUserInfo(userID, "", updated)
	log.Info().Str("userid", userID).Bool("s3_enabled", enabled).Str("media_delivery", mediaDelivery).
		Msg("user info cache updated with the S3 configuration")
}
