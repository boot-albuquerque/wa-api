package usecase

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
)

// GetUserLIDUseCase obtém o LID para um JID
type GetUserLIDUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetUserLIDUseCase cria uma nova instância
func NewGetUserLIDUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetUserLIDUseCase {
	return &GetUserLIDUseCase{clientProvider: cp, logger: logger}
}

// LIDResult representa o resultado com JID e LID
type LIDResult struct {
	JID string `json:"jid"`
	LID string `json:"lid"`
}

// Execute obtém o LID para um JID
func (uc *GetUserLIDUseCase) Execute(ctx context.Context, userID string, req domain.GetUserLIDRequest) (*LIDResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	// Parse JID
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		uc.logger.Warn().Err(err).Str("jid", req.JID).Msg("Failed to parse JID")
		return nil, fmt.Errorf("invalid jid format: %w", err)
	}

	// Get LID from store
	lid, err := client.Store.LIDs.GetLIDForPN(ctx, jid)
	if err != nil {
		uc.logger.Error().Err(err).Str("jid", req.JID).Msg("Failed to get LID")
		return nil, fmt.Errorf("LID not found: %w", err)
	}

	if lid.IsEmpty() {
		return nil, fmt.Errorf("LID not found for this number")
	}

	return &LIDResult{
		JID: jid.String(),
		LID: lid.String(),
	}, nil
}
