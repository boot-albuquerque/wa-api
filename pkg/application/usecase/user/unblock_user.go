package user

import (
	"context"
	"fmt"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// UnblockUserUseCase desbloqueia um usuário
type UnblockUserUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewUnblockUserUseCase cria uma nova instância
func NewUnblockUserUseCase(cp appport.ClientProvider, logger appport.Logger) *UnblockUserUseCase {
	return &UnblockUserUseCase{clientProvider: cp, logger: logger}
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
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	// Parse target JID
	target := strings.TrimSpace(req.JID)
	if target == "" {
		target = strings.TrimSpace(req.Phone)
	}
	if target == "" {
		return nil, fmt.Errorf("missing Phone or JID")
	}

	jid, err := types.ParseJID(target)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "target", target)
		return nil, fmt.Errorf("could not parse Phone or JID: %w", err)
	}

	// Normalize JID
	jid = normalizeBlocklistJID(jid)

	// Get resolved JID
	blocklistJID, blocklist, err := updateBlocklistWithResolvedJID(ctx, client, jid, events.BlocklistChangeActionUnblock)
	if err != nil {
		uc.logger.Error(ctx, "Failed to unblock user", "error", err, "jid", jid.String())
		return nil, fmt.Errorf("failed to unblock user: %w", err)
	}

	blockedJIDs := []string{}
	if blocklist != nil {
		blockedJIDs = make([]string, len(blocklist.JIDs))
		for i, blockedJID := range blocklist.JIDs {
			blockedJIDs[i] = blockedJID.String()
		}
	}

	result := &UnblockResult{
		Details:   "User unblocked",
		JID:       blocklistJID.String(),
		Blocklist: blockedJIDs,
		DHash:     "",
	}

	if blocklistJID != jid {
		result.RequestedJID = jid.String()
	}

	if blocklist != nil {
		result.DHash = blocklist.DHash
	}

	return result, nil
}
