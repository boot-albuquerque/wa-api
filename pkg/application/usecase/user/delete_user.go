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
	users  appport.UserRepository
	logger appport.Logger
}

// NewDeleteUserUseCase cria uma nova instância
func NewDeleteUserUseCase(users appport.UserRepository, logger appport.Logger) *DeleteUserUseCase {
	return &DeleteUserUseCase{users: users, logger: logger}
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

	uc.logger.Info(ctx, "User deleted successfully", "user_id", req.UserID)
	return nil
}
