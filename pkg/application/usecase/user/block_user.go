package user

import (
	"context"
	"fmt"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// BlockUserUseCase bloqueia um usuário
type BlockUserUseCase struct {
	blocklist appport.BlocklistManager
	jids      appport.JIDResolver
	logger    appport.Logger
}

// NewBlockUserUseCase cria uma nova instância
func NewBlockUserUseCase(bm appport.BlocklistManager, jr appport.JIDResolver, logger appport.Logger) *BlockUserUseCase {
	return &BlockUserUseCase{blocklist: bm, jids: jr, logger: logger}
}

// BlockResult representa o resultado da operação de bloqueio
type BlockResult struct {
	Details      string   `json:"Details"`
	JID          string   `json:"JID"`
	Blocklist    []string `json:"Blocklist"`
	DHash        string   `json:"DHash"`
	RequestedJID string   `json:"RequestedJID,omitempty"`
}

// Execute bloqueia um usuário
func (uc *BlockUserUseCase) Execute(ctx context.Context, userID string, req domain.BlockUserRequest) (*BlockResult, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
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

	jid, err := uc.jids.ResolveQualifiedJID(ctx, target)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "target", target)
		return nil, fmt.Errorf("could not parse Phone or JID: %w", err)
	}

	// A normalização do JID e a tradução de LID para número são do adapter:
	// são regra do SDK. O resultado devolve os dois JIDs porque a resposta
	// distingue o pedido do efetivamente usado.
	update, err := uc.blocklist.UpdateBlocklist(ctx, userID, jid, true)
	if err != nil {
		uc.logger.Error(ctx, "Failed to block user", "error", err, "jid", string(jid))
		return nil, fmt.Errorf("failed to block user: %w", err)
	}

	result := &BlockResult{
		Details:   "User blocked",
		JID:       string(update.ResolvedJID),
		Blocklist: update.Entries,
		DHash:     update.DHash,
	}

	if update.ResolvedJID != update.RequestedJID {
		result.RequestedJID = string(update.RequestedJID)
	}

	return result, nil
}
