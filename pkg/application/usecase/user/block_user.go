package user

import (
	"context"
	"fmt"
	"strings"
	"wa-api/pkg/domain/apperr"

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
	Details      string
	JID          string
	Blocklist    []string
	DHash        string
	RequestedJID string
}

// Execute bloqueia um usuário
func (uc *BlockUserUseCase) Execute(ctx context.Context, userID string, req domain.BlockUserRequest) (*BlockResult, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "error", err, "user_id", userID)
		return nil, err
	}

	// Parse target JID
	target := strings.TrimSpace(req.JID)
	if target == "" {
		target = strings.TrimSpace(req.Phone)
	}
	if target == "" {
		return nil, apperr.New("missing_phone_or_jid", apperr.CategoryValidation, "missing Phone or JID", false, nil)
	}

	// F203, decisão 35=a do canal: resolução LENIENTE, que aplica o servidor
	// por omissão. Antes disto o campo chamava-se `Phone` e recusava um
	// telefone — medido em campo, {"Phone":"5511000000001"} devolvia 400
	// invalid_phone_or_jid, enquanto /chat/send/text aceitava o mesmo formato.
	// Um campo com esse nome que exige @s.whatsapp.net é contrato
	// surpreendente, e a mensagem de erro nem dizia o que faltava.
	jid, err := uc.jids.ResolveJID(ctx, target)
	if err != nil {
		uc.logger.Warn(ctx, "Failed to parse JID", "error", err, "target", target)
		return nil, apperr.New("invalid_phone_or_jid", apperr.CategoryValidation,
			"could not parse Phone or JID", false, err)
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
