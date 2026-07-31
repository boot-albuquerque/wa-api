package usecase

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	appport "disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
)

// CheckUserUseCase verifica se usuários estão no WhatsApp
type CheckUserUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewCheckUserUseCase cria uma nova instância
func NewCheckUserUseCase(cp appport.ClientProvider, logger zerolog.Logger) *CheckUserUseCase {
	return &CheckUserUseCase{clientProvider: cp, logger: logger}
}

// CheckUserResult representa o resultado da verificação
type CheckUserResult struct {
	Query        string `json:"query"`
	IsInWhatsapp bool   `json:"is_in_whatsapp"`
	JID          string `json:"jid"`
	VerifiedName string `json:"verified_name"`
}

// Execute verifica se um usuário está no WhatsApp
func (uc *CheckUserUseCase) Execute(ctx context.Context, userID string, req domain.CheckUserRequest) ([]CheckUserResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	resp, err := client.IsOnWhatsApp(ctx, req.Phone)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to check WhatsApp users")
		return nil, fmt.Errorf("failed to check users: %w", err)
	}

	var results []CheckUserResult
	for _, item := range resp {
		verifiedName := ""
		if item.VerifiedName != nil {
			verifiedName = item.VerifiedName.Details.GetVerifiedName()
		}

		result := CheckUserResult{
			Query:        item.Query,
			IsInWhatsapp: item.IsIn,
			JID:          fmt.Sprintf("%s", item.JID),
			VerifiedName: verifiedName,
		}
		results = append(results, result)
	}

	return results, nil
}
