package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"disparazap/internal/shared/domain"
	"disparazap/internal/infra/storage"
)

// EditUserUseCase edita um usuário existente
type EditUserUseCase struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewEditUserUseCase cria uma nova instância
func NewEditUserUseCase(db *sqlx.DB, logger zerolog.Logger) *EditUserUseCase {
	return &EditUserUseCase{db: db, logger: logger}
}

// Execute edita um usuário
func (uc *EditUserUseCase) Execute(ctx context.Context, req domain.EditUserRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	// Check if user exists
	var count int
	if err := uc.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM users WHERE id = $1", req.UserID); err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("user not found")
	}

	// Validate events if provided
	if req.Events != "" {
		eventList := strings.Split(req.Events, ",")
		for _, event := range eventList {
			event = strings.TrimSpace(event)
			if event == "" {
				continue
			}
			if !isValidEvent(event) {
				return fmt.Errorf("invalid event type: %s", event)
			}
		}
	}

	// Build dynamic UPDATE query
	query := "UPDATE users SET "
	args := []interface{}{}
	argIndex := 1

	// Helper to add field
	addField := func(fieldName string, value interface{}, condition bool) {
		if condition {
			if argIndex > 1 {
				query += ", "
			}
			query += fieldName + " = $" + strconv.Itoa(argIndex)
			args = append(args, value)
			argIndex++
		}
	}

	// Add fields to update
	addField("name", req.Name, req.Name != "")
	addField("token", req.Token, req.Token != "")
	addField("webhook", req.Webhook, req.Webhook != "")
	addField("expiration", req.Expiration, req.Expiration != 0)
	addField("events", req.Events, req.Events != "")
	addField("history", req.History, req.History != 0)

	// Handle proxy config
	if req.ProxyConfig != nil {
		if req.ProxyConfig.Enabled {
			addField("proxy_url", req.ProxyConfig.ProxyURL, true)
		} else {
			addField("proxy_url", "", true)
		}
		if req.ProxyConfig.WebhookUseProxy != nil {
			addField("webhook_use_proxy", *req.ProxyConfig.WebhookUseProxy, true)
		}
	}

	// Handle S3 config
	if req.S3Config != nil {
		addField("s3_enabled", req.S3Config.Enabled, true)
		addField("s3_endpoint", req.S3Config.Endpoint, true)
		addField("s3_region", req.S3Config.Region, true)
		addField("s3_bucket", req.S3Config.Bucket, true)
		addField("s3_access_key", req.S3Config.AccessKey, true)
		addField("s3_secret_key", req.S3Config.SecretKey, true)
		addField("s3_path_style", req.S3Config.PathStyle, true)
		addField("s3_public_url", req.S3Config.PublicURL, true)
		addField("media_delivery", req.S3Config.MediaDelivery, true)
		addField("s3_retention_days", req.S3Config.RetentionDays, true)
	}

	// If no fields to update
	if argIndex == 1 {
		return fmt.Errorf("no fields to update")
	}

	// Add WHERE clause
	query += " WHERE id = $" + strconv.Itoa(argIndex)
	args = append(args, req.UserID)

	// Execute update
	_, err := uc.db.ExecContext(ctx, query, args...)
	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to update user")
		return fmt.Errorf("database error: %w", err)
	}

	// Update S3Manager if needed
	if req.S3Config != nil {
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
			_ = storage.GetS3Manager().InitializeS3Client(req.UserID, s3Config)
		} else {
			storage.GetS3Manager().RemoveClient(req.UserID)
		}
	}

	return nil
}
