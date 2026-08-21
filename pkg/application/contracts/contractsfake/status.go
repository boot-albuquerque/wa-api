package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// StatusMessageSetterCall é uma chamada a SetStatusMessage.
type StatusMessageSetterCall struct {
	Ctx   context.Context
	TxtID string
	Msg   string
}

// StatusMessageSetter é o dublê de port.StatusMessageSetter.
//
// Guarda a MENSAGEM e não só a contagem: o defeito da F198 era a rota
// responder 200 sem chamar nada, e o defeito seguinte mais provável é chamar
// com o texto errado — que uma contagem não distingue de sucesso.
type StatusMessageSetter struct {
	SessionGuard

	SetStatusMessageFunc  func(ctx context.Context, txtID, msg string) error
	SetStatusMessageCalls []StatusMessageSetterCall
}

var _ port.StatusMessageSetter = (*StatusMessageSetter)(nil)

// SetStatusMessage implementa port.StatusMessageSetter.
func (f *StatusMessageSetter) SetStatusMessage(ctx context.Context, txtID, msg string) error {
	f.SetStatusMessageCalls = append(f.SetStatusMessageCalls, StatusMessageSetterCall{Ctx: ctx, TxtID: txtID, Msg: msg})
	if f.SetStatusMessageFunc != nil {
		return f.SetStatusMessageFunc(ctx, txtID, msg)
	}
	return nil
}

// HistorySyncRequesterCall é uma chamada a RequestHistorySync.
type HistorySyncRequesterCall struct {
	Ctx    context.Context
	TxtID  string
	Anchor port.HistoryAnchor
	Count  int
}

// HistorySyncRequester é o dublê de port.HistorySyncRequester.
//
// Guarda a ÂNCORA inteira: o pedido é "as N mensagens antes DESTA", e mandar a
// âncora errada devolve o pedaço errado do histórico com 200 — indistinguível
// de sucesso para quem chamou.
type HistorySyncRequester struct {
	SessionGuard

	RequestFunc  func(ctx context.Context, txtID string, anchor port.HistoryAnchor, count int) (string, error)
	RequestCalls []HistorySyncRequesterCall
}

var _ port.HistorySyncRequester = (*HistorySyncRequester)(nil)

// RequestHistorySync implementa port.HistorySyncRequester.
func (f *HistorySyncRequester) RequestHistorySync(ctx context.Context, txtID string, anchor port.HistoryAnchor, count int) (string, error) {
	f.RequestCalls = append(f.RequestCalls, HistorySyncRequesterCall{Ctx: ctx, TxtID: txtID, Anchor: anchor, Count: count})
	if f.RequestFunc != nil {
		return f.RequestFunc(ctx, txtID, anchor, count)
	}
	return "REQ-1", nil
}
