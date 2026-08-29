package user

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteUserUseCase deleta um usuário
type DeleteUserUseCase struct {
	users       appport.UserRepository
	republisher appport.UserInfoRepublisher
	logger      appport.Logger
}

// NewDeleteUserUseCase cria uma nova instância.
//
// republisher drops the cached token/user-id entries after a successful
// delete. See appport.UserInfoRepublisher and HOUSEKEEP F273: without it, a
// token belonging to a deleted user kept authenticating for up to
// tokenCacheTTL because the auth middleware reads the cache and never
// revalidates against the database on a hit.
func NewDeleteUserUseCase(users appport.UserRepository, republisher appport.UserInfoRepublisher, logger appport.Logger) *DeleteUserUseCase {
	return &DeleteUserUseCase{users: users, republisher: republisher, logger: logger}
}

// Execute deleta um usuário
func (uc *DeleteUserUseCase) Execute(ctx context.Context, req domain.DeleteUserInput) error {
	if req.UserID == "" {
		return apperr.New("missing_user_id", apperr.CategoryValidation, "user ID is required", false, nil)
	}

	deleted, err := uc.users.DeleteUser(ctx, req.UserID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to delete user", "error", err, "user_id", req.UserID)
		return fmt.Errorf("database error: %w", err)
	}
	if !deleted {
		return apperr.New("user_not_found", apperr.CategoryNotFound, "user not found", false, nil)
	}

	// ORDER matters (F273): the row is gone from `users` BEFORE the cache is
	// touched. Invalidating first would leave a window where a concurrent
	// request repopulates the cache from a row that still exists.
	uc.republisher.RepublishUser(ctx, req.UserID)

	uc.logger.Info(ctx, "User deleted successfully", "user_id", req.UserID)
	return nil
}
