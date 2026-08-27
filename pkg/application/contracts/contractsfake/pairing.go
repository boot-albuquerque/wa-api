package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// --- PairingQRReader ---------------------------------------------------

// PairingQRReaderCall é uma chamada a PairingQR.
type PairingQRReaderCall struct {
	Ctx   context.Context
	TxtID string
}

// PairingQRReader é o fake de port.PairingQRReader.
//
// Zero-value: a sessão é válida (SessionGuard embutido) e o QR é a string
// vazia SEM erro — que é o estado real entre duas rotações do código, e não
// uma falha. Um dublê que devolvesse erro aí seria mais estrito que a
// produção (pkg/infra/wa-noise/adapters/pairing/qr.go), e esconderia o
// caminho que os testes precisam de exercitar.
type PairingQRReader struct {
	SessionGuard

	PairingQRFunc  func(ctx context.Context, txtID string) (string, error)
	PairingQRCalls []PairingQRReaderCall
}

var _ port.PairingQRReader = (*PairingQRReader)(nil)

// PairingQR implementa port.PairingQRReader.
func (f *PairingQRReader) PairingQR(ctx context.Context, txtID string) (string, error) {
	f.PairingQRCalls = append(f.PairingQRCalls, PairingQRReaderCall{Ctx: ctx, TxtID: txtID})
	if f.PairingQRFunc != nil {
		return f.PairingQRFunc(ctx, txtID)
	}
	return "", nil
}

// --- SessionStarter ----------------------------------------------------

// SessionStarterCheckOwnershipCall é uma chamada a CheckOwnership.
type SessionStarterCheckOwnershipCall struct {
	Ctx   context.Context
	TxtID string
}

// SessionStarterStartSessionCall é uma chamada a StartSession.
type SessionStarterStartSessionCall struct {
	Ctx   context.Context
	TxtID string
	Token string
}

// SessionStarter é o fake de port.SessionStarter.
//
// Zero-value: a posse é concedida e o arranque é gravado sem fazer nada. Não
// embute SessionGuard, pela mesma razão que a porta não o embute — conectar é
// o que CRIA a sessão (ver o comentário de port.SessionStarter).
//
// StartSession é SÍNCRONO aqui, e a produção lança uma goroutine
// (pkg/bootstrap/pairing_providers.go). A divergência é deliberada e é a
// única que este dublê tem: um teste que precisasse de sincronizar com a
// goroutine mediria o escalonador, não o handler. O que o dublê NÃO faz é
// simplificar a regra que interessa — a ordem CheckOwnership-antes-de-
// StartSession fica visível nas duas listas de chamadas.
type SessionStarter struct {
	CheckOwnershipFunc  func(ctx context.Context, txtID string) error
	CheckOwnershipCalls []SessionStarterCheckOwnershipCall

	StartSessionFunc  func(ctx context.Context, txtID, token string)
	StartSessionCalls []SessionStarterStartSessionCall
}

var _ port.SessionStarter = (*SessionStarter)(nil)

// CheckOwnership implementa port.SessionStarter.
func (f *SessionStarter) CheckOwnership(ctx context.Context, txtID string) error {
	f.CheckOwnershipCalls = append(f.CheckOwnershipCalls, SessionStarterCheckOwnershipCall{Ctx: ctx, TxtID: txtID})
	if f.CheckOwnershipFunc != nil {
		return f.CheckOwnershipFunc(ctx, txtID)
	}
	return nil
}

// StartSession implementa port.SessionStarter.
func (f *SessionStarter) StartSession(ctx context.Context, txtID, token string) {
	f.StartSessionCalls = append(f.StartSessionCalls, SessionStarterStartSessionCall{Ctx: ctx, TxtID: txtID, Token: token})
	if f.StartSessionFunc != nil {
		f.StartSessionFunc(ctx, txtID, token)
	}
}
