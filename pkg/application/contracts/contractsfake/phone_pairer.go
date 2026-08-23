package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// --- PhonePairer -------------------------------------------------------

// PhonePairerIsPairedCall é uma chamada a IsPaired.
type PhonePairerIsPairedCall struct {
	Ctx   context.Context
	TxtID string
}

// PhonePairerRequestPairingCodeCall é uma chamada a RequestPairingCode.
//
// O registro das chamadas é o que prova a ORDEM exigida pelo CAP-26: o teste
// da sessão já pareada assevera que este slice ficou VAZIO. Sem ele, inverter
// a guarda e o pedido de código passaria em todos os outros casos.
type PhonePairerRequestPairingCodeCall struct {
	Ctx   context.Context
	TxtID string
	Phone string
}

// PhonePairer é o fake de port.PhonePairer. Zero-value = sessão válida, não
// pareada, e código vazio — o teste que quiser código preenchido tem de
// dizê-lo, porque devolver código por default esconderia exatamente o defeito
// da F152.
type PhonePairer struct {
	SessionGuard

	IsPairedFunc  func(ctx context.Context, txtID string) (bool, error)
	IsPairedCalls []PhonePairerIsPairedCall

	RequestPairingCodeFunc  func(ctx context.Context, txtID, phone string) (string, error)
	RequestPairingCodeCalls []PhonePairerRequestPairingCodeCall
}

var _ port.PhonePairer = (*PhonePairer)(nil)

// IsPaired implementa port.PhonePairer.
func (f *PhonePairer) IsPaired(ctx context.Context, txtID string) (bool, error) {
	f.IsPairedCalls = append(f.IsPairedCalls, PhonePairerIsPairedCall{Ctx: ctx, TxtID: txtID})
	if f.IsPairedFunc != nil {
		return f.IsPairedFunc(ctx, txtID)
	}
	return false, nil
}

// RequestPairingCode implementa port.PhonePairer.
func (f *PhonePairer) RequestPairingCode(ctx context.Context, txtID, phone string) (string, error) {
	f.RequestPairingCodeCalls = append(f.RequestPairingCodeCalls, PhonePairerRequestPairingCodeCall{Ctx: ctx, TxtID: txtID, Phone: phone})
	if f.RequestPairingCodeFunc != nil {
		return f.RequestPairingCodeFunc(ctx, txtID, phone)
	}
	return "", nil
}
