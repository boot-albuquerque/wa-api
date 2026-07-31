package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"disparazap/internal/shared/domain"
	dbpkg "disparazap/internal/infra/db"
	"disparazap/internal/infra/storage"
)

// AddUserUseCase adiciona um novo usuário
type AddUserUseCase struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewAddUserUseCase cria uma nova instância
func NewAddUserUseCase(db *sqlx.DB, logger zerolog.Logger) *AddUserUseCase {
	return &AddUserUseCase{db: db, logger: logger}
}

// Execute adiciona um novo usuário
func (uc *AddUserUseCase) Execute(ctx context.Context, req domain.AddUserRequest) (*domain.UserResponse, error) {
	// Validate required fields
	if req.Name == "" || req.Token == "" {
		return nil, fmt.Errorf("name and token are required")
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

	// Encrypt HMAC key if provided
	var encryptedHmacKey []byte
	if req.HmacKey != "" {
		if len(req.HmacKey) < 32 {
			return nil, fmt.Errorf("HMAC key must be at least 32 characters long")
		}
		// Note: encryptHMACKey is defined in handlers.go - you'll need to refactor this
		// For now, we'll create a simple approach
		encrypted, err := encryptHMACKeyFunc(req.HmacKey)
		if err != nil {
			uc.logger.Error().Err(err).Msg("Failed to encrypt HMAC key")
			return nil, fmt.Errorf("failed to encrypt HMAC key: %w", err)
		}
		encryptedHmacKey = encrypted
	}

	// Check for existing user
	var count int
	if err := uc.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM users WHERE token = $1", req.Token); err != nil {
		uc.logger.Error().Err(err).Msg("Database error checking token")
		return nil, fmt.Errorf("database error: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("user with this token already exists")
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
				return nil, fmt.Errorf("invalid event type: %s", event)
			}
		}
	}

	// Generate ID
	id, err := dbpkg.GenerateRandomID()
	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to generate ID")
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	// Insert user
	_, err = uc.db.ExecContext(ctx,
		`INSERT INTO users (id, name, token, webhook, expiration, events, jid, qrcode, proxy_url,
		 webhook_use_proxy, s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key,
		 s3_path_style, s3_public_url, media_delivery, s3_retention_days, hmac_key, history)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
		id, req.Name, req.Token, req.Webhook, req.Expiration, req.Events, "", "",
		req.ProxyConfig.ProxyURL, webhookUseProxy,
		req.S3Config.Enabled, req.S3Config.Endpoint, req.S3Config.Region, req.S3Config.Bucket,
		req.S3Config.AccessKey, req.S3Config.SecretKey, req.S3Config.PathStyle, req.S3Config.PublicURL,
		req.S3Config.MediaDelivery, req.S3Config.RetentionDays, encryptedHmacKey, req.History)

	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to insert user")
		return nil, fmt.Errorf("database error: %w", err)
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

// Placeholder functions - these will be injected or refactored
func encryptHMACKeyFunc(key string) ([]byte, error) {
	// This will be replaced with the actual encryption from handlers.go
	return []byte(key), nil
}

func isValidEvent(event string) bool {
	// This will check against domain.SupportedEventTypes
	return true
}
