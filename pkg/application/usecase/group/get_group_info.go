package group

import (
	"context"
	"fmt"

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
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	// Parse GroupJID
	group, err := uc.jids.ResolveJID(ctx, req.GroupJID)
	if err != nil {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	// Garantir que há sessão
	if err := uc.groups.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	// Obter informações do grupo
	groupInfo, err := uc.groups.GetGroupInfo(ctx, txtID, group)
	if err != nil {
		uc.logger.Error(ctx, "failed to get group info", "txtID", txtID, "groupJID", req.GroupJID, "error", err)
		return nil, fmt.Errorf("failed to get group info: %v", err)
	}

	result := &domain.GetGroupInfoResult{
		GroupInfo: groupInfo,
	}

	uc.logger.Info(ctx, "group info retrieved", "txtID", txtID, "groupJID", req.GroupJID)
	return result, nil
}
