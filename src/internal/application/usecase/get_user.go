package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// GetUserUseCase obtém informações de usuários do WhatsApp
type GetUserUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetUserUseCase cria uma nova instância
func NewGetUserUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetUserUseCase {
	return &GetUserUseCase{clientProvider: cp, logger: logger}
}

// Execute obtém informações de usuários
func (uc *GetUserUseCase) Execute(ctx context.Context, userID string, req domain.CheckUserRequest) (json.RawMessage, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	var jids []types.JID
	for _, phone := range req.Phone {
		jid, err := types.ParseJID(phone)
		if err != nil {
			uc.logger.Warn().Err(err).Str("phone", phone).Msg("Failed to parse JID")
			continue
		}
		jids = append(jids, jid)
	}

	resp, err := client.GetUserInfo(ctx, jids)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to get user info")
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Convert response to JSON
	data, err := json.Marshal(map[string]interface{}{"users": resp})
	if err != nil {
		uc.logger.Error().Err(err).Msg("Failed to marshal response")
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	return data, nil
}
