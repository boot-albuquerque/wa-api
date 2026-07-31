package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// DeleteUserCompleteUseCase completely deletes a user
type DeleteUserCompleteUseCase struct {
	db             *sql.DB
	clientProvider appport.ClientProvider
	clientManager  appport.HealthClientProvider
	logger         zerolog.Logger
	exPath         string
}

// NewDeleteUserCompleteUseCase creates a new instance
func NewDeleteUserCompleteUseCase(db *sql.DB, cp appport.ClientProvider, cm appport.HealthClientProvider, logger zerolog.Logger, exPath string) *DeleteUserCompleteUseCase {
	return &DeleteUserCompleteUseCase{
		db:             db,
		clientProvider: cp,
		clientManager:  cm,
		logger:         logger,
		exPath:         exPath,
	}
}

// Execute completely deletes a user
func (uc *DeleteUserCompleteUseCase) Execute(ctx context.Context, userID string) (*domain.DeleteUserCompleteResult, error) {
	if userID == "" {
		return nil, fmt.Errorf("missing ID")
	}

	// Check if user exists
	var exists bool
	err := uc.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&exists)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("database error checking user existence")
		return nil, fmt.Errorf("database error")
	}
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	// Get user info before deletion
	var uname, jid, token string
	err = uc.db.QueryRowContext(ctx, "SELECT name, jid, token FROM users WHERE id = $1", userID).Scan(&uname, &jid, &token)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("problem retrieving user information")
		// Continue anyway since we have the ID
	}

	// 1. Logout and disconnect instance via port (simulated through clientProvider)
	client, _ := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if client != nil {
		if client.IsConnected() {
			uc.logger.Info().Str("user_id", userID).Msg("Logging out user")
			_ = client.Logout(context.Background())
		}
		uc.logger.Info().Str("user_id", userID).Msg("Disconnecting from WhatsApp")
		client.Disconnect()
	}

	// 2. Query S3 config before deleting the user
	var s3Enabled bool
	err = uc.db.QueryRowContext(ctx, "SELECT s3_enabled FROM users WHERE id = $1", userID).Scan(&s3Enabled)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("problem retrieving user s3 configuration")
		// Continue anyway since we have the ID to delete local files
	}

	// 3. Remove from DB
	_, err = uc.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("database error deleting user")
		return nil, fmt.Errorf("database error")
	}

	// 4. Cleanup from memory (simulated - actual implementation would need concrete client manager)
	// This is delegated to the handler which has access to the global clientManager

	// 5. Remove media files
	userDirectory := filepath.Join(uc.exPath, "files", userID)
	if stat, err := os.Stat(userDirectory); err == nil && stat.IsDir() {
		uc.logger.Info().Str("dir", userDirectory).Msg("deleting media and history files from disk")
		err = os.RemoveAll(userDirectory)
		if err != nil {
			uc.logger.Error().Err(err).Str("dir", userDirectory).Msg("error removing media directory")
		}
	}

	// 6. Remove files from S3 (if enabled)
	// This would be delegated to storage provider in handlers
	if s3Enabled {
		uc.logger.Info().Str("user_id", userID).Msg("S3 deletion needed - to be handled by handler")
	}

	uc.logger.Info().Str("user_id", userID).Str("name", uname).Str("jid", jid).Msg("user deleted successfully")

	return &domain.DeleteUserCompleteResult{
		Code: 200,
		Data: domain.UserDeleteData{
			ID:   userID,
			Name: uname,
			JID:  jid,
		},
		Success: true,
		Details: "user instance removed completely",
	}, nil
}
