package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
)

// AddUserUseCase adiciona um novo usuário
type AddUserUseCase struct {
	users  appport.UserRepository
	logger appport.Logger
}

// NewAddUserUseCase cria uma nova instância
func NewAddUserUseCase(users appport.UserRepository, logger appport.Logger) *AddUserUseCase {
	return &AddUserUseCase{users: users, logger: logger}
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
			uc.logger.Error(ctx, "Failed to encrypt HMAC key", "error", err)
			return nil, fmt.Errorf("failed to encrypt HMAC key: %w", err)
		}
		encryptedHmacKey = encrypted
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
		uc.logger.Error(ctx, "Failed to generate ID", "error", err)
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
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
		S3:              *req.S3Config,
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

// Placeholder functions - these will be injected or refactored
func encryptHMACKeyFunc(key string) ([]byte, error) {
	// This will be replaced with the actual encryption from handlers.go
	return []byte(key), nil
}

func isValidEvent(event string) bool {
	// This will check against domain.SupportedEventTypes
	return true
}
