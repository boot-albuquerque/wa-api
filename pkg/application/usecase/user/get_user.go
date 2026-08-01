package user

import (
	"context"
	"encoding/json"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
)

// GetUserUseCase obtém informações de usuários do WhatsApp
type GetUserUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetUserUseCase cria uma nova instância
func NewGetUserUseCase(cp appport.ClientProvider, logger appport.Logger) *GetUserUseCase {
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
			uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "phone", phone)
			continue
		}
		jids = append(jids, jid)
	}

	resp, err := client.GetUserInfo(ctx, jids)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get user info", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Convert response to JSON
	data, err := json.Marshal(map[string]interface{}{"users": resp})
	if err != nil {
		uc.logger.Error(ctx, "Failed to marshal response", "error", err)
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	return data, nil
}
