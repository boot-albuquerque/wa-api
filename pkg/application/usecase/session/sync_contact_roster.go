package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// validSyncContactRosterModes são os únicos valores aceitos em
// SyncContactRosterRequest.Mode.
var validSyncContactRosterModes = map[string]bool{
	"if_unsynced": true,
	"incremental": true,
	"full":        true,
}

// SyncContactRosterUseCase encapsula a validação e o disparo do pull forçado
// do patch de app-state que carrega a agenda de contatos. Capacidade distinta
// de RequestHistorySyncUseCase — não mexe em histórico de mensagens.
type SyncContactRosterUseCase struct {
	appState appport.AppStateSyncer
	logger   appport.Logger
}

// NewSyncContactRosterUseCase cria uma nova instância do usecase.
func NewSyncContactRosterUseCase(as appport.AppStateSyncer, l appport.Logger) *SyncContactRosterUseCase {
	return &SyncContactRosterUseCase{
		appState: as,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível, valida o modo pedido e força
// o pull da agenda de contatos.
func (uc *SyncContactRosterUseCase) Execute(ctx context.Context, txtID string, req domain.SyncContactRosterRequest) (*domain.SyncContactRosterResult, error) {
	if err := uc.appState.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	if !validSyncContactRosterModes[req.Mode] {
		uc.logger.Error(ctx, "invalid contact roster sync mode", "txtID", txtID, "mode", req.Mode)
		return nil, apperr.New("invalid_sync_mode", apperr.CategoryValidation,
			"mode must be one of: if_unsynced, incremental, full", false, nil)
	}

	if err := uc.appState.SyncContactRoster(ctx, txtID, req.Mode); err != nil {
		uc.logger.Error(ctx, "contact roster sync failed", "txtID", txtID, "mode", req.Mode, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "contact roster sync requested", "txtID", txtID, "mode", req.Mode)
	return &domain.SyncContactRosterResult{Details: "contact roster sync requested", Mode: req.Mode}, nil
}
