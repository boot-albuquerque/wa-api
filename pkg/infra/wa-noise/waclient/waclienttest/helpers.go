package waclienttest

import (
	"context"
	"errors"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"

	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/waclient"
)

// GetterWith devolve uma waclient.Getter que mapeia txtID para o cliente
// correspondente em clients. txtIDs ausentes devolvem nil (que é exatamente
// o comportamento de registry.ClientManager.Getwa-noiseClient).
func GetterWith(clients map[string]waclient.Client) waclient.Getter {
	return func(txtID string) waclient.Client {
		return clients[txtID]
	}
}

// ErrSynthetic é o erro usado nos testes de propagação de falha do SDK.
var ErrSynthetic = errors.New("synthetic SDK error")

// ErrClient é um waclient.Client mínimo que devolve Err para a operação
// nomeada em ErrOp — usado nos testes de propagação de erro do SDK.
type ErrClient struct {
	Fake
	ErrOp string
	Err   error
}

func (e *ErrClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
	if e.ErrOp == "SendMessage" {
		return wanoise.SendResponse{}, e.Err
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
