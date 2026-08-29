package chat

import (
	"context"
	"testing"
	"time"

	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/noise"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// Este arquivo cobre SendList (CAP-22) no nível em que a TRADUÇÃO fica
// visível: o EMBRULHO da mensagem (DocumentWithCaptionMessage >
// FutureProofMessage > ListMessage, e não o ViewOnceMessage óbvio que o
// histórico marca como ERRADO), o nó BIZ sem o qual o servidor não processa
// a mensagem como lista, e a tradução de domain.ListSection/ListRow para
// waE2E.ListMessage_Section/Row. Acima daqui nada disso é observável — o use
// case entrega seções de domínio já normalizadas e não conhece protobuf.

const listChatJID = "5511987654321@s.whatsapp.net"

func listAdapter(f *testkit.Fake) *ChatMessengerAdapter {
	return NewChatMessengerAdapter(testkit.GetterWith(map[string]client.Client{"u1": f}))
}

// sendListCapturing envia payload e devolve a mensagem que chegou a
// SendMessage, mais os extras.
func sendListCapturing(t *testing.T, payload domain.ListPayload, _ *domain.ReplyContext, id string) (*waE2E.Message, []noise.SendRequestExtra) {
	t.Helper()

	var sent *waE2E.Message
	var gotExtra []noise.SendRequestExtra
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, m *waE2E.Message, extra ...noise.SendRequestExtra) (noise.SendResponse, error) {
			sent, gotExtra = m, extra
			return noise.SendResponse{ID: "list-wire-id", Timestamp: time.Unix(1755500140, 0)}, nil
		},
	}

	_, err := listAdapter(f).SendList(context.Background(), "u1", listChatJID, payload, nil, nil, id)
	if err != nil {
		t.Fatalf("SendList: %v", err)
	}
	return sent, gotExtra
}

// listMessage extrai o ListMessage do embrulho esperado, falhando com
// mensagem útil se a mensagem não tiver a forma
// DocumentWithCaptionMessage > FutureProofMessage > ListMessage.
func listMessage(t *testing.T, m *waE2E.Message) *waE2E.ListMessage {
	t.Helper()
	doc := m.GetDocumentWithCaptionMessage()
	if doc == nil {
		t.Fatalf("a mensagem enviada nao tem DocumentWithCaptionMessage: %+v", m)
	}
	inner := doc.GetMessage()
	if inner == nil {
		t.Fatalf("DocumentWithCaptionMessage sem Message interno: %+v", doc)
	}
	lm := inner.GetListMessage()
	if lm == nil {
		t.Fatalf("o Message interno nao tem ListMessage: %+v", inner)
	}
	return lm
}

func TestChatMessengerAdapter_SendList_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))

	_, err := a.SendList(context.Background(), "u1", listChatJID, domain.ListPayload{Body: "corpo"}, nil, nil, "")

	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendList code = %q, quero no_session", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendList_Wrapper trava o EMBRULHO histórico: o
// óbvio (ViewOnceMessage > FutureProofMessage) é o que o histórico marca
// como ERRADO — sem o embrulho correto a lista chega como texto simples ou
// nem chega.
func TestChatMessengerAdapter_SendList_Wrapper(t *testing.T) {
	sent, _ := sendListCapturing(t, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Title: "Sec", Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}, nil, "")

	if sent.GetViewOnceMessage() != nil {
		t.Fatal("mensagem enviada usa o embrulho ERRADO (ViewOnceMessage)")
	}
	lm := listMessage(t, sent)
	if lm.GetDescription() != "Escolha" {
		t.Errorf("Description = %q, quero %q", lm.GetDescription(), "Escolha")
	}
}

// TestChatMessengerAdapter_SendList_ListTypeIsSingleSelect trava o
// ListType, que o contrato histórico fixa em SINGLE_SELECT sempre — nunca é
// campo do payload público.
func TestChatMessengerAdapter_SendList_ListTypeIsSingleSelect(t *testing.T) {
	sent, _ := sendListCapturing(t, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Title: "Sec", Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}, nil, "")

	lm := listMessage(t, sent)
	if got := lm.GetListType(); got != waE2E.ListMessage_SINGLE_SELECT {
		t.Errorf("ListType = %v, quero SINGLE_SELECT", got)
	}
}

// TestChatMessengerAdapter_SendList_SectionsAndRowsTranslated trava a
// tradução, campo por campo, preservando a ORDEM de seções e linhas.
func TestChatMessengerAdapter_SendList_SectionsAndRowsTranslated(t *testing.T) {
	sent, _ := sendListCapturing(t, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Ver opcoes",
		Title:      "Cabecalho",
		Footer:     "Rodape",
		Sections: []domain.ListSection{
			{Title: "Primeira", Rows: []domain.ListRow{
				{Title: "Item A", Description: "desc A", RowID: "id-a"},
				{Title: "Item B", RowID: "id-b"},
			}},
			{Title: "Segunda", Rows: []domain.ListRow{{Title: "Item C", RowID: "id-c"}}},
		},
	}, nil, "")

	lm := listMessage(t, sent)
	if got := lm.GetButtonText(); got != "Ver opcoes" {
		t.Errorf("ButtonText = %q, quero %q", got, "Ver opcoes")
	}
	if got := lm.GetTitle(); got != "Cabecalho" {
		t.Errorf("Title = %q, quero %q", got, "Cabecalho")
	}
	if got := lm.GetFooterText(); got != "Rodape" {
		t.Errorf("FooterText = %q, quero %q", got, "Rodape")
	}

	sections := lm.GetSections()
	if len(sections) != 2 {
		t.Fatalf("Sections tem %d, quero 2", len(sections))
	}
	if sections[0].GetTitle() != "Primeira" || sections[1].GetTitle() != "Segunda" {
		t.Fatalf("ordem das secoes nao preservada: %q, %q", sections[0].GetTitle(), sections[1].GetTitle())
	}

	rowsA := sections[0].GetRows()
	if len(rowsA) != 2 {
		t.Fatalf("secao 1 tem %d linha(s), quero 2", len(rowsA))
	}
	if rowsA[0].GetTitle() != "Item A" || rowsA[0].GetRowID() != "id-a" || rowsA[0].GetDescription() != "desc A" {
		t.Errorf("linha 0 da secao 1: %+v", rowsA[0])
	}
	if rowsA[1].GetTitle() != "Item B" || rowsA[1].GetRowID() != "id-b" || rowsA[1].GetDescription() != "" {
		t.Errorf("linha 1 da secao 1: %+v", rowsA[1])
	}

	rowsB := sections[1].GetRows()
	if len(rowsB) != 1 || rowsB[0].GetTitle() != "Item C" || rowsB[0].GetRowID() != "id-c" {
		t.Fatalf("secao 2: %+v", rowsB)
	}
}

// TestChatMessengerAdapter_SendList_TitleAndFooterAreOptional: Title e
// Footer vazios não viram ponteiro-para-string-vazia no wire — ficam nil,
// como o histórico (`if t := ...; t != "" { ... }`).
func TestChatMessengerAdapter_SendList_TitleAndFooterAreOptional(t *testing.T) {
	sent, _ := sendListCapturing(t, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}, nil, "")

	lm := listMessage(t, sent)
	if lm.Title != nil {
		t.Errorf("Title = %q, quero nil (sem TopText no payload)", lm.GetTitle())
	}
	if lm.FooterText != nil {
		t.Errorf("FooterText = %q, quero nil (sem FooterText no payload)", lm.GetFooterText())
	}
	if lm.GetSections()[0].Title != nil {
		t.Errorf("Section.Title = %q, quero nil (sem titulo de secao no payload)", lm.GetSections()[0].GetTitle())
	}
}

// TestChatMessengerAdapter_SendList_BizNodeIsAlwaysSent trava o nó
// biz > list(type="product_list", v="2") — sem ele o servidor não processa a
// mensagem como lista interativa.
func TestChatMessengerAdapter_SendList_BizNodeIsAlwaysSent(t *testing.T) {
	_, extra := sendListCapturing(t, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}, nil, "")

	if len(extra) != 1 {
		t.Fatalf("SendMessage recebeu %d extra(s), quero 1", len(extra))
	}
	if extra[0].AdditionalNodes == nil {
		t.Fatal("AdditionalNodes nil: o no' BIZ nao foi enviado e a lista nao e' processada")
	}
	nodes := *extra[0].AdditionalNodes
	if len(nodes) != 1 || nodes[0].Tag != "biz" {
		t.Fatalf("no' raiz = %+v, quero um unico no' \"biz\"", nodes)
	}
	list, ok := nodes[0].Content.([]waBinary.Node)
	if !ok || len(list) != 1 || list[0].Tag != "list" {
		t.Fatalf("conteudo do \"biz\" = %+v, quero um unico no' \"list\"", nodes[0].Content)
	}
	if got := list[0].Attrs["type"]; got != "product_list" {
		t.Errorf("list.type = %v, quero \"product_list\"", got)
	}
	if got := list[0].Attrs["v"]; got != "2" {
		t.Errorf("list.v = %v, quero \"2\"", got)
	}
}

// TestChatMessengerAdapter_SendList_CallerIDIsForwarded: o id pedido pelo
// chamador vai no extra, junto do nó BIZ — e a AUSÊNCIA dele não pode
// derrubar o nó.
func TestChatMessengerAdapter_SendList_CallerIDIsForwarded(t *testing.T) {
	payload := domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}

	_, comID := sendListCapturing(t, payload, nil, "id-do-cliente")
	if got := string(comID[0].ID); got != "id-do-cliente" {
		t.Errorf("extra.ID = %q, quero o id do chamador", got)
	}
	if comID[0].AdditionalNodes == nil {
		t.Error("com id do chamador, o no' BIZ sumiu")
	}

	_, semID := sendListCapturing(t, payload, nil, "")
	if got := string(semID[0].ID); got != "" {
		t.Errorf("extra.ID = %q sem id do chamador, quero vazio", got)
	}
	if semID[0].AdditionalNodes == nil {
		t.Error("sem id do chamador, o no' BIZ sumiu")
	}
}

// TestChatMessengerAdapter_SendList_ResultComesFromTheWire: ID e Timestamp
// são os que a sessão REALMENTE usou, nunca fabricados localmente.
func TestChatMessengerAdapter_SendList_ResultComesFromTheWire(t *testing.T) {
	sentAt := time.Unix(1755500140, 0)
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{ID: "id-do-wire", Timestamp: sentAt}, nil
		},
	}

	got, err := listAdapter(f).SendList(context.Background(), "u1", listChatJID, domain.ListPayload{
		Body:       "Escolha",
		ButtonText: "Select",
		Sections:   []domain.ListSection{{Rows: []domain.ListRow{{Title: "Item", RowID: "item-1"}}}},
	}, nil, nil, "id-do-cliente")
	if err != nil {
		t.Fatalf("SendList: %v", err)
	}
	if got.ID != "id-do-wire" {
		t.Errorf("ID = %q, quero o do wire (e nao o do chamador)", got.ID)
	}
	if !got.Timestamp.Equal(sentAt) {
		t.Errorf("Timestamp = %v, quero %v", got.Timestamp, sentAt)
	}
}

// TestChatMessengerAdapter_SendList_InvalidJIDNeverSends: JID que não
// parseia é recusado ANTES de qualquer envio.
func TestChatMessengerAdapter_SendList_InvalidJIDNeverSends(t *testing.T) {
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...noise.SendRequestExtra) (noise.SendResponse, error) {
			t.Error("SendList chamou client.SendMessage com JID invalido")
			return noise.SendResponse{}, nil
		},
	}

	_, err := listAdapter(f).SendList(context.Background(), "u1", "@@@", domain.ListPayload{
		Body: "Escolha",
	}, nil, nil, "")
	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
}
