package user

import (
	"context"
	"encoding/json"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetUserUseCase obtém informações de usuários do WhatsApp
type GetUserUseCase struct {
	contacts appport.ContactDirectory
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetUserUseCase cria uma nova instância
func NewGetUserUseCase(cd appport.ContactDirectory, jr appport.JIDResolver, logger appport.Logger) *GetUserUseCase {
	return &GetUserUseCase{contacts: cd, jids: jr, logger: logger}
}

// Execute obtém informações de usuários
func (uc *GetUserUseCase) Execute(ctx context.Context, userID string, req domain.CheckUserRequest) (json.RawMessage, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	// Telefone que não parseia é pulado, não é erro — comportamento
	// preservado do upstream.
	var jids []domain.JID
	for _, phone := range req.Phone {
		jid, err := uc.jids.ResolveQualifiedJID(ctx, phone)
		if err != nil {
			uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "phone", phone)
			continue
		}
		jids = append(jids, jid)
	}

	resp, err := uc.contacts.GetUserInfo(ctx, userID, jids)
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
