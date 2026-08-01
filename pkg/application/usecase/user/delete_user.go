package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
)

// DeleteUserUseCase deleta um usuário
type DeleteUserUseCase struct {
	db     *sqlx.DB
	logger appport.Logger
}

// NewDeleteUserUseCase cria uma nova instância
func NewDeleteUserUseCase(db *sqlx.DB, logger appport.Logger) *DeleteUserUseCase {
	return &DeleteUserUseCase{db: db, logger: logger}
}

// Execute deleta um usuário
func (uc *DeleteUserUseCase) Execute(ctx context.Context, req domain.DeleteUserRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	result, err := uc.db.ExecContext(ctx, "DELETE FROM users WHERE id=$1", req.UserID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to delete user", "error", err, "user_id", req.UserID)
		return fmt.Errorf("database error: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		uc.logger.Error(ctx, "Failed to check rows affected", "error", err)
		return fmt.Errorf("failed to verify deletion: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	uc.logger.Info(ctx, "User deleted successfully", "user_id", req.UserID)
	return nil
}
