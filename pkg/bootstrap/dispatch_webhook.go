package bootstrap

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/auth"
	"wa-api/pkg/infra/helpers"
	"wa-api/pkg/infra/storage"
)

// ProxyConfig holds per-user proxy settings for WhatsApp and webhook delivery.
type ProxyConfig struct {
	Enabled         bool   `json:"enabled"`
	ProxyURL        string `json:"proxyURL"`
	WebhookUseProxy *bool  `json:"webhookUseProxy,omitempty"`
}

var Find = helpers.Find

// Update entry in User map
func updateUserInfo(values interface{}, field string, value string) interface{} {
	log.Debug().Str("field", field).Str("value", value).Msg("User info updated")
	// Copy-on-write: the map inside Values is shared — it lives in
	// appCtx.UserInfoCache and is handed to request goroutines via the request
	// context. Mutating it in place races with concurrent readers (Values.Get)
	// and can crash the process with "concurrent map read and map write".
	// Build a fresh map and return a new Values; callers persist it via
	// appCtx.UserInfoCache.Set. Use a comma-ok assertion so a nil or unexpected value
	// can't panic — it falls back to the zero Values (nil map), handled below.
	old, _ := values.(Values)
	m := make(map[string]string, len(old.M)+1)
	for k, v := range old.M {
		m[k] = v
	}
	m[field] = value
	return Values{M: m}
}

// callHook functions extracted to dispatch_callhook.go

func ProcessOutgoingMedia(userID string, contactJID string, messageID string, data []byte, mimeType string, fileName string, db *sqlx.DB) (map[string]interface{}, error) {
	// Check if S3 is enabled for this user
	var s3Config struct {
		Enabled       bool   `db:"s3_enabled"`
		MediaDelivery string `db:"media_delivery"`
	}
	err := db.Get(&s3Config, "SELECT s3_enabled, media_delivery FROM users WHERE id = $1", userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get S3 config")
		s3Config.Enabled = false
		s3Config.MediaDelivery = "base64"
	}

	// Process S3 upload if enabled
	if s3Config.Enabled && (s3Config.MediaDelivery == "s3" || s3Config.MediaDelivery == "both") {
		ensureS3ClientForUser(userID)
		// Process S3 upload (outgoing messages are always in outbox)
		s3Data, err := storage.GetS3Manager().ProcessMediaForS3(
			context.Background(),
			userID,
			contactJID,
			messageID,
			data,
			mimeType,
			fileName,
			false, // isIncoming = false for sent messages
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upload media to S3")
			// Continue even if S3 upload fails
		} else {
			return s3Data, nil
		}
	}

	return nil, nil
}

// HMAC crypto delegates to pkg/infra/auth
func generateHmacSignature(payload, encryptedKey []byte) (string, error) {
	return auth.GenerateHmacSignature(payload, encryptedKey, []byte(appCtx.GlobalEncryptionKey))
}
func encryptHMACKey(plainText string) ([]byte, error) {
	return auth.EncryptHMACKey(plainText, appCtx.GlobalEncryptionKey)
}
