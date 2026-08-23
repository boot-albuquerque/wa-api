package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Mensagens de resultado e de erro deste use case, como constantes: o teste
// que trava o contrato assere as MESMAS strings que a produção devolve
// (ADR-0004). "saved successfully" é o texto histórico
// (`41bc8e2^:handlers.go:6816`), não uma invenção desta fase.
const (
	hmacConfiguredDetails = "HMAC configuration saved successfully"
	hmacKeyTooShortMsg    = "HMAC key must be at least 32 characters long"
	hmacEncryptFailedMsg  = "failed to encrypt HMAC key"
	hmacSaveFailedMsg     = "failed to save HMAC configuration"
	hmacKeyTooShortCode   = "hmac_key_too_short"
)

// ConfigureHmacUseCase grava a chave HMAC por usuário.
type ConfigureHmacUseCase struct {
	sessions  appport.SessionGuard
	keys      appport.HmacKeyStore
	encryptor appport.HmacKeyEncryptor
	cache     appport.UserInfoHmacCache
	logger    appport.Logger
}

// NewConfigureHmacUseCase cria uma nova instância do usecase.
func NewConfigureHmacUseCase(
	sg appport.SessionGuard,
	keys appport.HmacKeyStore,
	encryptor appport.HmacKeyEncryptor,
	cache appport.UserInfoHmacCache,
	l appport.Logger,
) *ConfigureHmacUseCase {
	return &ConfigureHmacUseCase{
		sessions:  sg,
		keys:      keys,
		encryptor: encryptor,
		cache:     cache,
		logger:    l,
	}
}

// Execute valida, cifra, grava e só então publica no cache.
//
// A ORDEM é o contrato, não um detalhe de implementação:
//
//  1. validar — uma chave curta nunca chega ao cifrador;
//  2. cifrar — uma falha aqui NÃO grava, ou a coluna guardaria texto claro;
//  3. gravar — a fonte de verdade primeiro;
//  4. cache — por último, porque o cache é `cache.NoExpiration`: publicar
//     antes da gravação deixaria a chave assinando webhooks sem que ela
//     exista no banco, e o restart a faria sumir sem aviso.
//
// A chave em claro nunca entra em log nem em erro: o único dado dela que
// aparece é o tamanho.
func (uc *ConfigureHmacUseCase) Execute(ctx context.Context, txtID string, req domain.HmacConfigRequest) (*domain.HmacConfigResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if len(req.HmacKey) < domain.MinHmacKeyLength {
		uc.logger.Warn(ctx, hmacKeyTooShortMsg, "txtID", txtID, "keyLength", len(req.HmacKey))
		return nil, apperr.New(hmacKeyTooShortCode, apperr.CategoryValidation, hmacKeyTooShortMsg, false, nil)
	}

	encrypted, err := uc.encryptor.EncryptHmacKey(req.HmacKey)
	if err != nil {
		uc.logger.Error(ctx, hmacEncryptFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", hmacEncryptFailedMsg, err)
	}

	if err := uc.keys.SaveHmacKey(ctx, txtID, encrypted); err != nil {
		uc.logger.Error(ctx, hmacSaveFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", hmacSaveFailedMsg, err)
	}

	uc.cache.SetHmacKey(txtID, encrypted)

	uc.logger.Info(ctx, hmacConfiguredDetails, "txtID", txtID)
	return &domain.HmacConfigResult{Details: hmacConfiguredDetails, Enabled: true}, nil
}
