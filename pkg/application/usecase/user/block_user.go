package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// BlockUserUseCase bloqueia um usuário
type BlockUserUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewBlockUserUseCase cria uma nova instância
func NewBlockUserUseCase(cp appport.ClientProvider, logger zerolog.Logger) *BlockUserUseCase {
	return &BlockUserUseCase{clientProvider: cp, logger: logger}
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
		uc.logger.Warn().Err(err).Str("target", target).Msg("Failed to parse JID")
		return nil, fmt.Errorf("could not parse Phone or JID: %w", err)
	}

	// Normalize JID
	jid = normalizeBlocklistJID(jid)

	// Get resolved JID
	blocklistJID, blocklist, err := updateBlocklistWithResolvedJID(ctx, client, jid, events.BlocklistChangeActionBlock)
	if err != nil {
		uc.logger.Error().Err(err).Str("jid", jid.String()).Msg("Failed to block user")
		return nil, fmt.Errorf("failed to block user: %w", err)
	}

	blockedJIDs := []string{}
	if blocklist != nil {
		blockedJIDs = make([]string, len(blocklist.JIDs))
		for i, blockedJID := range blocklist.JIDs {
			blockedJIDs[i] = blockedJID.String()
		}
	}

	result := &BlockResult{
		Details:   "User blocked",
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

func normalizeBlocklistJID(jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server == types.LegacyUserServer {
		jid.Server = types.DefaultUserServer
	}
	return jid
}

func updateBlocklistWithResolvedJID(ctx context.Context, client interface{}, jid types.JID, action events.BlocklistChangeAction) (types.JID, *types.Blocklist, error) {
	// Type assertion to get the client
	waClient, ok := client.(*whatsmeow.Client)
	if !ok {
		return jid, nil, fmt.Errorf("invalid client type")
	}

	blocklistJID, err := resolveBlocklistPNJID(ctx, waClient, jid)
	if err != nil {
		return jid, nil, err
	}

	blocklist, err := waClient.UpdateBlocklist(ctx, blocklistJID, action)
	return blocklistJID, blocklist, err
}

func resolveBlocklistPNJID(ctx context.Context, client *whatsmeow.Client, jid types.JID) (types.JID, error) {
	jid = normalizeBlocklistJID(jid)
	switch jid.Server {
	case types.DefaultUserServer:
		return jid, nil
	case types.HiddenUserServer:
		pn, err := getCachedPNForLID(ctx, client, jid)
		if err != nil {
			return types.JID{}, err
		}
		return normalizeBlocklistJID(pn), nil
	default:
		return types.JID{}, fmt.Errorf("unsupported blocklist JID server %q", jid.Server)
	}
}

func getCachedPNForLID(ctx context.Context, client *whatsmeow.Client, jid types.JID) (types.JID, error) {
	if client.Store == nil || client.Store.LIDs == nil {
		return types.JID{}, fmt.Errorf("LID-to-PN mapping store is not available")
	}
	pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
	if err != nil {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s: %w", jid, err)
	}
	if pn.IsEmpty() {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s", jid)
	}
	return pn, nil
}
