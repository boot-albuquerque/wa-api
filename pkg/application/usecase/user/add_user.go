package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
)

// Error codes and messages of this use case, as named constants: the tests
// that lock the contract assert the SAME strings production returns
// (ADR-0004).
const (
	hmacKeyTooShortCode  = "hmac_key_too_short"
	hmacKeyTooShortMsg   = "HMAC key must be at least 32 characters long"
	hmacEncryptFailedMsg = "failed to encrypt HMAC key"

	s3SecretEncryptFailedMsg = "failed to encrypt S3 secret key"

	invalidEventTypeCode   = "invalid_event_type"
	invalidEventTypeMsgFmt = "invalid event type: %s"
)

// AddUserUseCase adiciona um novo usuário
type AddUserUseCase struct {
	users      appport.UserRepository
	encryptor  appport.HmacKeyEncryptor
	s3Cipher   appport.S3SecretCipher
	logger     appport.Logger
}

// NewAddUserUseCase cria uma nova instância.
//
// The encryptor and s3Cipher are dependencies and not package-level helpers
// because the AES key lives in the process configuration
// (appCtx.GlobalEncryptionKey), which the application layer must not reach
// for. Each port matches a column writer: encryptor → users.hmac_key
// (F158), s3Cipher → users.s3_secret_key (F163). Separate ports because
// the two columns have different stored types (BYTEA vs TEXT envelope).
func NewAddUserUseCase(users appport.UserRepository, encryptor appport.HmacKeyEncryptor, s3Cipher appport.S3SecretCipher, logger appport.Logger) *AddUserUseCase {
	return &AddUserUseCase{users: users, encryptor: encryptor, s3Cipher: s3Cipher, logger: logger}
}

// Execute adiciona um novo usuário
func (uc *AddUserUseCase) Execute(ctx context.Context, req domain.AddUserRequest) (*domain.UserResponse, error) {
	// Validate required fields
	if req.Name == "" || req.Token == "" {
		return nil, apperr.New("missing_name_or_token", apperr.CategoryValidation, "name and token are required", false, nil)
	}

	// Set defaults
	if req.ProxyConfig == nil {
		req.ProxyConfig = &domain.ProxyConfig{}
	}
	if req.S3Config == nil {
		req.S3Config = &domain.S3Config{}
	}

	webhookUseProxy := true
	if req.ProxyConfig.WebhookUseProxy != nil {
		webhookUseProxy = *req.ProxyConfig.WebhookUseProxy
	}

	// Encrypt the HMAC key if provided.
	//
	// The ORDER is the contract: validate, then encrypt, then write. A failure
	// to encrypt returns BEFORE CreateUser, so the user is not created with an
	// empty key nor with the key in plaintext — the column is read back
	// through auth.DecryptHMACKey to sign every per-user webhook, and
	// plaintext there is not valid AES-GCM.
	var encryptedHmacKey []byte
	if req.HmacKey != "" {
		if len(req.HmacKey) < domain.MinHmacKeyLength {
			return nil, apperr.New(hmacKeyTooShortCode, apperr.CategoryValidation, hmacKeyTooShortMsg, false, nil)
		}
		// The plaintext key never reaches a log or an error: only its length does.
		encrypted, err := uc.encryptor.EncryptHmacKey(req.HmacKey)
		if err != nil {
			uc.logger.Error(ctx, hmacEncryptFailedMsg, "keyLength", len(req.HmacKey), "error", err)
			return nil, fmt.Errorf("%s: %w", hmacEncryptFailedMsg, err)
		}
		encryptedHmacKey = encrypted
	}

	// Encrypt the S3 secret key if provided (F163, ADR-0009).
	//
	// Same ORDER contract as the HMAC key above: encrypt before write. A
	// failure returns BEFORE CreateUser, so the column never holds plaintext.
	// The in-memory S3 client (below) receives the PLAINTEXT — the AWS SDK
	// signs with it, and the envelope would produce a client that fails every
	// request.
	var s3SecretEnvelope string
	if req.S3Config != nil && req.S3Config.SecretKey != "" {
		envelope, err := uc.s3Cipher.EncryptS3Secret(req.S3Config.SecretKey)
		if err != nil {
			uc.logger.Error(ctx, s3SecretEncryptFailedMsg, "error", err)
			return nil, fmt.Errorf("%s: %w", s3SecretEncryptFailedMsg, err)
		}
		s3SecretEnvelope = envelope
	}

	// Validate events
	if req.Events != "" {
		eventList := strings.Split(req.Events, ",")
		for _, event := range eventList {
			event = strings.TrimSpace(event)
			if event == "" {
				continue
			}
			if !isValidEvent(event) {
				return nil, apperr.New(invalidEventTypeCode, apperr.CategoryValidation,
					fmt.Sprintf(invalidEventTypeMsgFmt, event), false, nil)
			}
		}
	}

	// Generate ID
	id, err := dbpkg.GenerateRandomID()
	if err != nil {
		uc.logger.Error(ctx, "Failed to generate ID", "error", err)
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	s3ForRecord := *req.S3Config
	if s3SecretEnvelope != "" {
		s3ForRecord.SecretKey = s3SecretEnvelope
	}
	created, err := uc.users.CreateUser(ctx, domain.UserRecord{
		ID:              id,
		Name:            req.Name,
		Token:           req.Token,
		Webhook:         req.Webhook,
		Expiration:      req.Expiration,
		Events:          req.Events,
		ProxyURL:        req.ProxyConfig.ProxyURL,
		WebhookUseProxy: webhookUseProxy,
		S3:              s3ForRecord,
		HmacKey:         encryptedHmacKey,
		History:         req.History,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateToken) {
			return nil, ErrDuplicateToken
		}
		uc.logger.Error(ctx, "Failed to insert user", "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}
	if !created {
		return nil, ErrDuplicateToken
	}

	// Initialize S3 if enabled
	if req.S3Config.Enabled {
		s3Config := &storage.S3Config{
			Enabled:       req.S3Config.Enabled,
			Endpoint:      req.S3Config.Endpoint,
			Region:        req.S3Config.Region,
			Bucket:        req.S3Config.Bucket,
			AccessKey:     req.S3Config.AccessKey,
			SecretKey:     req.S3Config.SecretKey,
			PathStyle:     req.S3Config.PathStyle,
			PublicURL:     req.S3Config.PublicURL,
			MediaDelivery: req.S3Config.MediaDelivery,
			RetentionDays: req.S3Config.RetentionDays,
		}
		_ = storage.GetS3Manager().InitializeS3Client(id, s3Config)
	}

	// Build response
	proxyConfig := map[string]interface{}{
		"enabled":         req.ProxyConfig.ProxyURL != "",
		"proxyUrl":        req.ProxyConfig.ProxyURL,
		"webhookUseProxy": webhookUseProxy,
	}

	s3Config := map[string]interface{}{
		"enabled":        req.S3Config.Enabled,
		"endpoint":       req.S3Config.Endpoint,
		"region":         req.S3Config.Region,
		"bucket":         req.S3Config.Bucket,
		"access_key":     "***",
		"path_style":     req.S3Config.PathStyle,
		"public_url":     req.S3Config.PublicURL,
		"media_delivery": req.S3Config.MediaDelivery,
		"retention_days": req.S3Config.RetentionDays,
	}

	return &domain.UserResponse{
		ID:             id,
		Name:           req.Name,
		Token:          req.Token,
		Webhook:        req.Webhook,
		Expiration:     int64(req.Expiration),
		ProxyConfig:    proxyConfig,
		S3Config:       s3Config,
		Events:         req.Events,
		HmacConfigured: req.HmacKey != "",
	}, nil
}

// isValidEvent reports whether the event name is one of
// domain.SupportedEventTypes.
//
// Rejecting an unknown event is a NEW public contract, decided deliberately
// (HOUSEKEEP F159) — it is not the recovery of an older behaviour. There is
// no older behaviour to recover: this function used to return true for every
// input, so the 400 above was dead code, and the pre-migration handler did
// not validate events at all. Callers that today send a misspelled event and
// get 200 will start getting 400.
//
// The alternative — dropping the unknown entry and carrying on, which the
// sibling UpdateWebhook route does — was rejected: a rejection is visible to
// the integrator at the moment of the call and is fixable there, while a
// silent drop only surfaces later, to the operator, as an event that never
// arrives.
//
// The validator is domain.IsValidEventType and not the identical list in
// pkg/infra/constants (HOUSEKEEP F168) because this is the application layer:
// importing infra from here would invert the dependency direction, and
// pkg/domain depends on nobody.
func isValidEvent(event string) bool {
	return domain.IsValidEventType(event)
}
