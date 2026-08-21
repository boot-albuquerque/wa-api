package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// UserInfoRepublisherCall é uma chamada a RepublishUser.
type UserInfoRepublisherCall struct {
	Ctx    context.Context
	UserID string
}

// UserInfoRepublisher é o dublê de port.UserInfoRepublisher.
//
// Regista as chamadas E a ORDEM face à escrita: o defeito da F200 era a
// edição não republicar, e o defeito que a correção poderia introduzir é
// republicar ANTES de a escrita ter sucesso — publicando na cache um valor
// que o banco não tem, e que nunca expira. Por isso o dublê guarda quantas
// escritas já tinham acontecido quando foi chamado.
type UserInfoRepublisher struct {
	RepublishFunc  func(ctx context.Context, userID string)
	RepublishCalls []UserInfoRepublisherCall

	// EscritasAoSerChamado é preenchido pelo teste com um contador externo,
	// para que a asserção de ordem não dependa de relógio.
	ContadorDeEscritas   func() int
	EscritasAoSerChamado []int
}

var _ port.UserInfoRepublisher = (*UserInfoRepublisher)(nil)

// RepublishUser implementa port.UserInfoRepublisher.
func (f *UserInfoRepublisher) RepublishUser(ctx context.Context, userID string) {
	f.RepublishCalls = append(f.RepublishCalls, UserInfoRepublisherCall{Ctx: ctx, UserID: userID})
	if f.ContadorDeEscritas != nil {
		f.EscritasAoSerChamado = append(f.EscritasAoSerChamado, f.ContadorDeEscritas())
	}
	if f.RepublishFunc != nil {
		f.RepublishFunc(ctx, userID)
	}
}
