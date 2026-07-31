package user

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"wa-api/pkg/domain"
)

// DeleteUserUseCase deleta um usuário
type DeleteUserUseCase struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewDeleteUserUseCase cria uma nova instância
func NewDeleteUserUseCase(db *sqlx.DB, logger zerolog.Logger) *DeleteUserUseCase {
	return &DeleteUserUseCase{db: db, logger: logger}
}

// Execute deleta um usuário
func (uc *DeleteUserUseCase) Execute(ctx context.Context, req domain.DeleteUserRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	result, err := uc.db.ExecContext(ctx, "DELETE FROM users WHERE id=$1", req.UserID)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", req.UserID).Msg("Failed to delete user")
		return fmt.Errorf("database error: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to check rows affected")
		return fmt.Errorf("failed to verify deletion: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	uc.logger.Info().Str("user_id", req.UserID).Msg("User deleted successfully")
	return nil
}
