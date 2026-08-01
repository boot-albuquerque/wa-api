package user

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetBlocklistUseCase retrieves the current blocklist
type GetBlocklistUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetBlocklistUseCase creates a new instance
func NewGetBlocklistUseCase(cp appport.ClientProvider, logger appport.Logger) *GetBlocklistUseCase {
	return &GetBlocklistUseCase{clientProvider: cp, logger: logger}
}

// Execute retrieves the blocklist
func (uc *GetBlocklistUseCase) Execute(ctx context.Context, userID string, _ domain.GetBlocklistRequest) (map[string]interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	blocklist, err := client.GetBlocklist(ctx)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get blocklist", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get blocklist: %w", err)
	}

	jids := []string{}
	dhash := ""
	if blocklist != nil {
		jids = make([]string, len(blocklist.JIDs))
		for i, blockedJID := range blocklist.JIDs {
			jids[i] = blockedJID.String()
		}
		dhash = blocklist.DHash
	}

	uc.logger.Info(ctx, "Retrieved blocklist", "user_id", userID, "count", len(jids))

	return map[string]interface{}{
		"Blocklist": jids,
		"DHash":     dhash,
	}, nil
}
