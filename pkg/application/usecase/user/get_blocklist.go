package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetBlocklistUseCase retrieves the current blocklist
type GetBlocklistUseCase struct {
	blocklist appport.BlocklistManager
	logger    appport.Logger
}

// NewGetBlocklistUseCase creates a new instance
func NewGetBlocklistUseCase(bm appport.BlocklistManager, logger appport.Logger) *GetBlocklistUseCase {
	return &GetBlocklistUseCase{blocklist: bm, logger: logger}
}

// Execute retrieves the blocklist
func (uc *GetBlocklistUseCase) Execute(ctx context.Context, userID string, _ domain.GetBlocklistRequest) (map[string]interface{}, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	blocklist, err := uc.blocklist.GetBlocklist(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get blocklist", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get blocklist: %w", err)
	}

	uc.logger.Info(ctx, "Retrieved blocklist", "user_id", userID, "count", len(blocklist.JIDs))

	return map[string]interface{}{
		"Blocklist": blocklist.JIDs,
		"DHash":     blocklist.DHash,
	}, nil
}
