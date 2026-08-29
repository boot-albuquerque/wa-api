package testkit

import (
	"context"
	"errors"

	"wa-api/internal/noise"
	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"

	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/noise/client"
)

// GetterWith devolve uma client.Getter que mapeia txtID para o cliente
// correspondente em clients. txtIDs ausentes devolvem nil (que é exatamente
// o comportamento de registry.ClientManager.GetNoiseClient).
func GetterWith(clients map[string]client.Client) client.Getter {
	return func(txtID string) client.Client {
		return clients[txtID]
	}
}

// ErrSynthetic é o erro usado nos testes de propagação de falha do SDK.
var ErrSynthetic = errors.New("synthetic SDK error")

// ErrClient é um client.Client mínimo que devolve Err para a operação
// nomeada em ErrOp — usado nos testes de propagação de erro do SDK.
type ErrClient struct {
	Fake
	ErrOp string
	Err   error
}

func (e *ErrClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...noise.SendRequestExtra) (noise.SendResponse, error) {
	if e.ErrOp == "SendMessage" {
		return noise.SendResponse{}, e.Err
	}
	return e.Fake.SendMessage(ctx, to, message, extra...)
}

// ErrStore simula um *store.Device com Contacts/LIDs para os adapters de
// usuário.
type ErrStore struct{ *store.Device }

// AppErrCode devolve o Code de um *apperr.AppError, ou "" se err for de outro
// tipo. Usado nas asserções de identidade de erro dos testes de adapter, que
// hoje vivem em vários subpacotes e antes compartilhavam este helper por
// morarem todos no mesmo pacote.
func AppErrCode(err error) string {
	if e, ok := err.(*apperr.AppError); ok {
		return e.Code
	}
	return ""
}
