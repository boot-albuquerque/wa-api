package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// GetBlocklistUseCase retrieves the current blocklist
type GetBlocklistUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetBlocklistUseCase creates a new instance
func NewGetBlocklistUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetBlocklistUseCase {
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
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to get blocklist")
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

	uc.logger.Info().Str("user_id", userID).Int("count", len(jids)).Msg("Retrieved blocklist")

	return map[string]interface{}{
		"Blocklist": jids,
		"DHash":     dhash,
	}, nil
}
