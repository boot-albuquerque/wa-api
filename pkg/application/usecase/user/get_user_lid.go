package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetUserLIDUseCase obtém o LID para um JID
type GetUserLIDUseCase struct {
	contacts appport.ContactDirectory
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetUserLIDUseCase cria uma nova instância
func NewGetUserLIDUseCase(cd appport.ContactDirectory, jr appport.JIDResolver, logger appport.Logger) *GetUserLIDUseCase {
	return &GetUserLIDUseCase{contacts: cd, jids: jr, logger: logger}
}

// LIDResult representa o resultado com JID e LID
type LIDResult struct {
	JID string `json:"jid"`
	LID string `json:"lid"`
}

// Execute obtém o LID para um JID
func (uc *GetUserLIDUseCase) Execute(ctx context.Context, userID string, req domain.GetUserLIDRequest) (*LIDResult, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	// Parse JID
	jid, err := uc.jids.ResolveQualifiedJID(ctx, req.JID)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "jid", req.JID)
		return nil, fmt.Errorf("invalid jid format: %w", err)
	}

	// Get LID from store
	lid, err := uc.contacts.GetLIDForPN(ctx, userID, jid)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get LID", "error", err, "jid", req.JID)
		return nil, fmt.Errorf("LID not found: %w", err)
	}

	if lid == "" {
		return nil, fmt.Errorf("LID not found for this number")
	}

	return &LIDResult{
		JID: string(jid),
		LID: string(lid),
	}, nil
}
