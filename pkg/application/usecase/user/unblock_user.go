package user

import (
	"context"
	"fmt"
	"strings"
	"wa-api/pkg/domain/apperr"

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
	Details      string
	JID          string
	Blocklist    []string
	DHash        string
	RequestedJID string
}

// Execute desbloqueia um usuário
func (uc *UnblockUserUseCase) Execute(ctx context.Context, userID string, req domain.UnblockUserRequest) (*UnblockResult, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
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
