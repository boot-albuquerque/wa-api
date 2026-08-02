package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/storage"
)

// EditUserUseCase edita um usuário existente
type EditUserUseCase struct {
	users  appport.UserRepository
	logger appport.Logger
}

// NewEditUserUseCase cria uma nova instância
func NewEditUserUseCase(users appport.UserRepository, logger appport.Logger) *EditUserUseCase {
	return &EditUserUseCase{users: users, logger: logger}
}

// Execute edita um usuário
func (uc *EditUserUseCase) Execute(ctx context.Context, req domain.EditUserRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	// Check if user exists
	exists, err := uc.users.UserExists(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	if !exists {
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

	// Quais campos mudam é decisão do use case; como isso vira uma instrução
	// SQL é do adapter. Ponteiro nil significa "não informado".
	upd := domain.UserUpdate{}
	if req.Name != "" {
		upd.Name = &req.Name
	}
	if req.Token != "" {
		upd.Token = &req.Token
	}
	if req.Webhook != "" {
		upd.Webhook = &req.Webhook
	}
	if req.Expiration != 0 {
		upd.Expiration = &req.Expiration
	}
	if req.Events != "" {
		upd.Events = &req.Events
	}
	if req.History != 0 {
		upd.History = &req.History
	}
	if req.ProxyConfig != nil {
		proxyURL := ""
		if req.ProxyConfig.Enabled {
			proxyURL = req.ProxyConfig.ProxyURL
		}
		upd.ProxyURL = &proxyURL
		upd.WebhookUseProxy = req.ProxyConfig.WebhookUseProxy
	}
	if req.S3Config != nil {
		upd.S3 = req.S3Config
	}

	if err := uc.users.UpdateUser(ctx, req.UserID, upd); err != nil {
		if errors.Is(err, ErrDuplicateToken) {
			return ErrDuplicateToken
		}
		if errors.Is(err, domain.ErrNoFieldsToUpdate) {
			return err
		}
		uc.logger.Error(ctx, "Failed to update user", "error", err)
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
