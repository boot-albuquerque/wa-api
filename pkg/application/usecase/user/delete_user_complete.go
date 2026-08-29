package user

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// userRemovedCompletelyDetails is the summary this use case reports on
// success. A named constant and not a literal: the contract test asserts the
// SAME string production returns (ADR-0004).
const userRemovedCompletelyDetails = "user instance removed completely"

// DeleteUserCompleteUseCase completely deletes a user
type DeleteUserCompleteUseCase struct {
	db          *sql.DB
	sessions    appport.SessionController
	republisher appport.UserInfoRepublisher
	logger      appport.Logger
	exPath      string
}

// NewDeleteUserCompleteUseCase creates a new instance.
//
// republisher drops the cached token/user-id entries after the row is
// removed from `users`. See appport.UserInfoRepublisher and HOUSEKEEP F273:
// this path deletes the row directly with SQL, same as DeleteUserUseCase, so
// it needs the same invalidation or the deleted user's token keeps
// authenticating until the cache entry's TTL runs out.
func NewDeleteUserCompleteUseCase(db *sql.DB, sc appport.SessionController, republisher appport.UserInfoRepublisher, logger appport.Logger, exPath string) *DeleteUserCompleteUseCase {
	return &DeleteUserCompleteUseCase{
		db:          db,
		sessions:    sc,
		republisher: republisher,
		logger:      logger,
		exPath:      exPath,
	}
}

// Execute completely deletes a user
func (uc *DeleteUserCompleteUseCase) Execute(ctx context.Context, userID string) (*domain.DeleteUserCompleteResult, error) {
	if userID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing ID", false, nil)
	}

	// Check if user exists
	var exists bool
	err := uc.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&exists)
	if err != nil {
		uc.logger.Error(ctx, "database error checking user existence", "error", err, "user_id", userID)
		return nil, fmt.Errorf("database error")
	}
	if !exists {
		return nil, apperr.New("user_not_found", apperr.CategoryNotFound, "user not found", false, nil)
	}

	// Get user info before deletion
	var uname, jid, token string
	err = uc.db.QueryRowContext(ctx, "SELECT name, jid, token FROM users WHERE id = $1", userID).Scan(&uname, &jid, &token)
	if err != nil {
		uc.logger.Error(ctx, "problem retrieving user information", "error", err, "user_id", userID)
		// Continue anyway since we have the ID
	}

	// 1. Logout and disconnect instance via port
	if err := uc.sessions.EnsureSession(ctx, userID); err == nil {
		connected, _ := uc.sessions.SessionStatus(ctx, userID)
		if connected {
			uc.logger.Info(ctx, "Logging out user", "user_id", userID)
			_ = uc.sessions.Logout(ctx, userID)
		}
		uc.logger.Info(ctx, "Disconnecting from WhatsApp", "user_id", userID)
		_ = uc.sessions.Disconnect(ctx, userID)
	}

	// 2. Query S3 config before deleting the user
	var s3Enabled bool
	err = uc.db.QueryRowContext(ctx, "SELECT s3_enabled FROM users WHERE id = $1", userID).Scan(&s3Enabled)
	if err != nil {
		uc.logger.Error(ctx, "problem retrieving user s3 configuration", "error", err, "user_id", userID)
		// Continue anyway since we have the ID to delete local files
	}

	// 3. Remove from DB
	_, err = uc.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		uc.logger.Error(ctx, "database error deleting user", "error", err, "user_id", userID)
		return nil, fmt.Errorf("database error")
	}

	// ORDER matters (F273): invalidate the cache only AFTER the row is gone,
	// so a concurrent read cannot repopulate the cache from a row that still
	// exists.
	uc.republisher.RepublishUser(ctx, userID)

	// 4. Cleanup from memory (simulated - actual implementation would need concrete client manager)
	// This is delegated to the handler which has access to the global clientManager

	// 5. Remove media files
	userDirectory := filepath.Join(uc.exPath, "files", userID)
	if stat, err := os.Stat(userDirectory); err == nil && stat.IsDir() {
		uc.logger.Info(ctx, "deleting media and history files from disk", "dir", userDirectory)
		err = os.RemoveAll(userDirectory)
		if err != nil {
			uc.logger.Error(ctx, "error removing media directory", "error", err, "dir", userDirectory)
		}
	}

	// 6. Remove files from S3 (if enabled)
	// This would be delegated to storage provider in handlers
	if s3Enabled {
		uc.logger.Info(ctx, "S3 deletion needed - to be handled by handler", "user_id", userID)
	}

	uc.logger.Info(ctx, "user deleted successfully", "user_id", userID, "name", uname, "jid", jid)

	return &domain.DeleteUserCompleteResult{
		User: domain.UserDeleteData{
			ID:   userID,
			Name: uname,
			JID:  jid,
		},
		Details: userRemovedCompletelyDetails,
	}, nil
}
