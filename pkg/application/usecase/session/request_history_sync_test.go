package session_test

import (
	"context"
	"errors"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
)

// F198, segunda metade. /user/history/sync validava, logava
// "request history sync validated" e devolvia 200 com resultado VAZIO —
// nenhum pedido era feito. E havia uma terceira camada: o HANDLER passava um
// domain.RequestHistorySyncRequest{} vazio, portanto os cinco campos que o DTO
// documenta nunca eram lidos.

func TestRequestHistorySync_PedeComAAncoraDoCliente(t *testing.T) {
	h := &contractsfake.HistorySyncRequester{}

	res, err := session.NewRequestHistorySyncUseCase(h, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.RequestHistorySyncRequest{
			ChatJid:            "5511999999999@s.whatsapp.net",
			OldestMsgID:        "MSG-42",
			OldestMsgFromMe:    true,
			OldestMsgTimestamp: 1700000000,
			Count:              25,
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if len(h.RequestCalls) != 1 {
		t.Fatalf("porta chamada %d vez(es), quero 1 — a rota responde 200 sem pedir nada (F198)", len(h.RequestCalls))
	}

	got := h.RequestCalls[0]
	// A ÂNCORA inteira: pedir "as N antes desta" com a mensagem errada devolve
	// o pedaço errado do histórico, com 200, e ninguém nota.
	want := appport.HistoryAnchor{
		ChatJID:   "5511999999999@s.whatsapp.net",
		MessageID: "MSG-42",
		FromMe:    true,
		Timestamp: 1700000000,
	}
	if got.Anchor != want {
		t.Errorf("âncora = %+v, quero %+v", got.Anchor, want)
	}
	if got.Count != 25 {
		t.Errorf("count = %d, quero 25", got.Count)
	}
	if res.Details == "" {
		t.Error("Details vazio: o id do pedido é a única prova que o cliente tem de que algo foi enviado")
	}
}

// TestRequestHistorySync_CountOmitidoUsaORecomendado trava o valor por omissão
// e a sua origem: 50 é o que a própria biblioteca recomenda.
func TestRequestHistorySync_CountOmitidoUsaORecomendado(t *testing.T) {
	h := &contractsfake.HistorySyncRequester{}

	if _, err := session.NewRequestHistorySyncUseCase(h, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.RequestHistorySyncRequest{
			ChatJid: "c@s.whatsapp.net", OldestMsgID: "M1",
		}); err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if got := h.RequestCalls[0].Count; got != 50 {
		t.Fatalf("count = %d, quero 50 (recomendação da biblioteca)", got)
	}
}

// TestRequestHistorySync_SemAncoraRecusa é a mudança de contrato: pedir sem
// âncora não tem significado no protocolo, e aceitar seria a F198 outra vez —
// 200 e nada a acontecer.
func TestRequestHistorySync_SemAncoraRecusa(t *testing.T) {
	casos := map[string]domain.RequestHistorySyncRequest{
		"sem chat_jid":      {OldestMsgID: "M1"},
		"sem oldest_msg_id": {ChatJid: "c@s.whatsapp.net"},
		"sem nada":          {},
	}
	for nome, req := range casos {
		t.Run(nome, func(t *testing.T) {
			h := &contractsfake.HistorySyncRequester{}
			if _, err := session.NewRequestHistorySyncUseCase(h, &contractsfake.Logger{}).
				Execute(context.Background(), "u1", req); err == nil {
				t.Fatal("esperava recusa")
			}
			if len(h.RequestCalls) != 0 {
				t.Fatalf("porta chamada %d vez(es) sem âncora", len(h.RequestCalls))
			}
		})
	}
}

// TestRequestHistorySync_FalhaDoEnvioNaoViraSucesso: se o peer message não sai,
// a rota não pode responder 200.
func TestRequestHistorySync_FalhaDoEnvioNaoViraSucesso(t *testing.T) {
	boom := errors.New("peer message recusado")
	h := &contractsfake.HistorySyncRequester{
		RequestFunc: func(context.Context, string, appport.HistoryAnchor, int) (string, error) {
			return "", boom
		},
	}
	res, err := session.NewRequestHistorySyncUseCase(h, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.RequestHistorySyncRequest{
			ChatJid: "c@s.whatsapp.net", OldestMsgID: "M1",
		})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, quero envolver %v", err, boom)
	}
	if res != nil {
		t.Fatalf("resultado = %+v, quero nil", res)
	}
}
