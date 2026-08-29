package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const (
	hmacDeletedDetails  = "HMAC configuration deleted successfully"
	hmacDeleteFailedMsg = "failed to delete HMAC configuration"
)

// DeleteHmacConfigUseCase revoga a chave HMAC por usuário.
type DeleteHmacConfigUseCase struct {
	sessions appport.SessionGuard
	keys     appport.HmacKeyStore
	cache    appport.UserInfoHmacCache
	logger   appport.Logger
}

// NewDeleteHmacConfigUseCase cria uma nova instância do usecase.
func NewDeleteHmacConfigUseCase(
	sg appport.SessionGuard,
	keys appport.HmacKeyStore,
	cache appport.UserInfoHmacCache,
	l appport.Logger,
) *DeleteHmacConfigUseCase {
	return &DeleteHmacConfigUseCase{sessions: sg, keys: keys, cache: cache, logger: l}
}

// Execute apaga a chave do banco E do cache — nesta ordem, e as duas partes
// são obrigatórias.
//
// Limpar só o banco não revoga nada: a entrada de appCtx.UserInfoCache é
// `cache.NoExpiration`, e pkg/bootstrap/lifecycle_webhook.go:178 assina cada
// webhook por usuário com o "HmacKeyEncrypted" que está nela. A chave
// comprometida continuaria assinando até o processo reiniciar, com resposta
// 200 idêntica à da revogação bem-sucedida (HOUSEKEEP F157).
//
// Limpar o cache ANTES do banco seria o defeito espelhado: uma falha de
// gravação deixaria a revogação pela metade, e o restart traria a chave de
// volta do banco. Por isso o cache só é tocado depois de a escrita ter dado
// certo.
func (uc *DeleteHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if err := uc.keys.DeleteHmacKey(ctx, txtID); err != nil {
		uc.logger.Error(ctx, hmacDeleteFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", hmacDeleteFailedMsg, err)
	}

	uc.cache.SetHmacKey(txtID, nil)

	uc.logger.Info(ctx, hmacDeletedDetails, "txtID", txtID)
	return &domain.HmacConfigResult{Details: hmacDeletedDetails, Enabled: false}, nil
}
