package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// CheckUserUseCase verifica se usuários estão no WhatsApp
type CheckUserUseCase struct {
	contacts appport.ContactDirectory
	logger   appport.Logger
}

// NewCheckUserUseCase cria uma nova instância
func NewCheckUserUseCase(cd appport.ContactDirectory, logger appport.Logger) *CheckUserUseCase {
	return &CheckUserUseCase{contacts: cd, logger: logger}
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
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	resp, err := uc.contacts.IsOnWhatsApp(ctx, userID, req.Phone)
	if err != nil {
		uc.logger.Error(ctx, "Failed to check WhatsApp users", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to check users: %w", err)
	}

	var results []CheckUserResult
	for _, item := range resp {
		results = append(results, CheckUserResult{
			Query:        item.Query,
			IsInWhatsapp: item.IsIn,
			JID:          item.JID,
			VerifiedName: item.VerifiedName,
		})
	}

	return results, nil
}
