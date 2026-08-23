package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// hmacLoadFailedMsg é a mensagem histórica do 500 de leitura
// (`41bc8e2^:handlers.go:6845`).
const hmacLoadFailedMsg = "failed to get HMAC configuration"

// hmacRetrievedMsg é o registro de sucesso da leitura. Não vai para o corpo:
// a resposta de `GET /hmac/config` é HmacConfigView, e só tem `hmac_key`.
const hmacRetrievedMsg = "HMAC configuration retrieved"

// GetHmacConfigUseCase lê o ESTADO da configuração de HMAC — nunca a chave.
type GetHmacConfigUseCase struct {
	sessions appport.SessionGuard
	keys     appport.HmacKeyStore
	logger   appport.Logger
}

// NewGetHmacConfigUseCase cria uma nova instância do usecase.
func NewGetHmacConfigUseCase(sg appport.SessionGuard, keys appport.HmacKeyStore, l appport.Logger) *GetHmacConfigUseCase {
	return &GetHmacConfigUseCase{sessions: sg, keys: keys, logger: l}
}

// Execute devolve `{"hmac_key": ""}` ou `{"hmac_key": "***"}`.
//
// O que sai é a PRESENÇA da chave, não a chave: o valor cifrado é o insumo da
// assinatura dos webhooks, e devolvê-lo entregaria ao chamador o material que
// a rota existe para proteger. O log carrega o mesmo booleano, não o valor.
func (uc *GetHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigView, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	encrypted, err := uc.keys.LoadHmacKey(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, hmacLoadFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", hmacLoadFailedMsg, err)
	}

	view := &domain.HmacConfigView{}
	if len(encrypted) > 0 {
		view.HmacKey = domain.MaskedHmacKey
	}

	uc.logger.Info(ctx, hmacRetrievedMsg, "txtID", txtID, "hasKey", len(encrypted) > 0)
	return view, nil
}
