package user

import (
	"context"
	"fmt"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// UnblockUserUseCase desbloqueia um usuário
type UnblockUserUseCase struct {
	blocklist appport.BlocklistManager
	jids      appport.JIDResolver
	logger    appport.Logger
}

// NewUnblockUserUseCase cria uma nova instância
func NewUnblockUserUseCase(bm appport.BlocklistManager, jr appport.JIDResolver, logger appport.Logger) *UnblockUserUseCase {
	return &UnblockUserUseCase{blocklist: bm, jids: jr, logger: logger}
}

// UnblockResult representa o resultado da operação de desbloqueio
type UnblockResult struct {
	Details      string   `json:"Details"`
	JID          string   `json:"JID"`
	Blocklist    []string `json:"Blocklist"`
	DHash        string   `json:"DHash"`
	RequestedJID string   `json:"RequestedJID,omitempty"`
}

// Execute desbloqueia um usuário
func (uc *UnblockUserUseCase) Execute(ctx context.Context, userID string, req domain.UnblockUserRequest) (*UnblockResult, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	// Parse target JID
	target := strings.TrimSpace(req.JID)
	if target == "" {
		target = strings.TrimSpace(req.Phone)
	}
	if target == "" {
		return nil, fmt.Errorf("missing Phone or JID")
	}

	jid, err := uc.jids.ResolveQualifiedJID(ctx, target)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "target", target)
		return nil, fmt.Errorf("could not parse Phone or JID: %w", err)
	}

	update, err := uc.blocklist.UpdateBlocklist(ctx, userID, jid, false)
	if err != nil {
		uc.logger.Error(ctx, "Failed to unblock user", "error", err, "jid", string(jid))
		return nil, fmt.Errorf("failed to unblock user: %w", err)
	}

	result := &UnblockResult{
		Details:   "User unblocked",
		JID:       string(update.ResolvedJID),
		Blocklist: update.Entries,
		DHash:     update.DHash,
	}

	if update.ResolvedJID != update.RequestedJID {
		result.RequestedJID = string(update.RequestedJID)
	}

	return result, nil
}
