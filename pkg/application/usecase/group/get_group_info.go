package group

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetGroupInfoUseCase encapsula a validação para obter informações de grupo
type GetGroupInfoUseCase struct {
	groups appport.GroupDirectory
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewGetGroupInfoUseCase cria uma nova instância do usecase
func NewGetGroupInfoUseCase(gd appport.GroupDirectory, jr appport.JIDResolver, l appport.Logger) *GetGroupInfoUseCase {
	return &GetGroupInfoUseCase{
		groups: gd,
		jids:   jr,
		logger: l,
	}
}

// Execute valida os campos e obtém as informações do grupo
func (uc *GetGroupInfoUseCase) Execute(ctx context.Context, txtID string, req domain.GetGroupInfoRequest) (*domain.GetGroupInfoResult, error) {
	// Validar GroupJID
	if req.GroupJID == "" {
		uc.logger.Warn(ctx, "missing groupJID in request", "txtID", txtID)
		return nil, apperr.New("missing_group_jid", apperr.CategoryValidation, "missing groupJID parameter", false, nil)
	}

	// Parse GroupJID
	group, err := uc.jids.ResolveJID(ctx, req.GroupJID)
	if err != nil {
		uc.logger.Warn(ctx, "could not parse group JID", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, apperr.New("invalid_group_jid", apperr.CategoryValidation, "could not parse Group JID", false, nil)
	}

	// Garantir que há sessão
	if err := uc.groups.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	// Obter informações do grupo
	groupInfo, err := uc.groups.GetGroupInfo(ctx, txtID, group)
	if err != nil {
		uc.logger.Error(ctx, "failed to get group info", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, fmt.Errorf("failed to get group info: %w", err)
	}

	result := &domain.GetGroupInfoResult{
		GroupInfo: groupInfo,
	}

	uc.logger.Info(ctx, "group info retrieved", "txtID", txtID, "groupJID", req.GroupJID)
	return result, nil
}
