package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Error codes and messages of this use case, as named constants: the tests
// that lock the contract assert the SAME strings production returns
// (ADR-0004).
//
// The two codes are distinct on purpose. An absent `mode` and a misspelled
// `mode` are errors with DIFFERENT corrections — one adds the field, the
// other fixes its value — and a client branching on error.code could not
// tell them apart while both answered invalid_sync_mode.
const (
	missingSyncModeCode = "missing_sync_mode"
	missingSyncModeMsg  = "missing mode in payload"

	invalidSyncModeCode   = "invalid_sync_mode"
	invalidSyncModeMsgFmt = "mode must be one of: if_unsynced, incremental, full; got %q"
)

// Accepted values of SyncContactRosterRequest.Mode. The comparison is
// case-SENSITIVE, like every other enum of this API: "FULL" is invalid.
const (
	SyncModeIfUnsynced  = "if_unsynced"
	SyncModeIncremental = "incremental"
	SyncModeFull        = "full"
)

// validSyncContactRosterModes holds the only values accepted in
// SyncContactRosterRequest.Mode.
var validSyncContactRosterModes = map[string]bool{
	SyncModeIfUnsynced:  true,
	SyncModeIncremental: true,
	SyncModeFull:        true,
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
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	// Absent, null and "" are indistinguishable once the payload is decoded
	// into a Go string, so the three collapse into the same answer: the field
	// is missing. Only a NON-EMPTY value outside the set is "invalid".
	if req.Mode == "" {
		uc.logger.Error(ctx, "missing contact roster sync mode", "txtID", txtID)
		return nil, apperr.New(missingSyncModeCode, apperr.CategoryValidation,
			missingSyncModeMsg, false, nil)
	}

	if !validSyncContactRosterModes[req.Mode] {
		uc.logger.Error(ctx, "invalid contact roster sync mode", "txtID", txtID, "mode", req.Mode)
		return nil, apperr.New(invalidSyncModeCode, apperr.CategoryValidation,
			fmt.Sprintf(invalidSyncModeMsgFmt, req.Mode), false, nil)
	}

	if err := uc.appState.SyncContactRoster(ctx, txtID, req.Mode); err != nil {
		uc.logger.Error(ctx, "contact roster sync failed", "txtID", txtID, "mode", req.Mode, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "contact roster sync requested", "txtID", txtID, "mode", req.Mode)
	return &domain.SyncContactRosterResult{Details: "contact roster sync requested", Mode: req.Mode}, nil
}
