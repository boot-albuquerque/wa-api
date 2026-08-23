package chat

import (
	"context"
	"testing"
	"time"

	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo cobre SendTemplate (CAP-15) no nível em que a TRADUÇÃO fica
// visível: qual `waE2E.Hydrated*Button` cada domain.TemplateButton vira, o
// texto do ID numerado automaticamente, a ordem dos botões e os quatro campos
// de HydratedFourRowTemplate. Acima daqui nada disso é observável — o use case
// entrega botões de domínio e não conhece protobuf.
//
// A asserção sobre o ID numerado é sobre o TEXTO, e não sobre "existe algum
// valor": o histórico escrevia `proto.String(string(id))` no ramo `default`, e
// `string(1)` em Go é "\x01" — um caractere de controle, não "1". Um teste que
// só checasse presença passaria com o defeito no lugar. Ver HOUSEKEEP F140.

const templateChatJID = "5511987654321@s.whatsapp.net"

func templateAdapter(f *testkit.Fake) *ChatMessengerAdapter {
	return NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f}))
}

// sendTemplateCapturing envia payload e devolve a mensagem que chegou a
// SendMessage, mais os extras.
func sendTemplateCapturing(t *testing.T, payload domain.TemplatePayload, _ *domain.ReplyContext, id string) (*waE2E.Message, []wanoise.SendRequestExtra) {
	t.Helper()

	var sent *waE2E.Message
	var gotExtra []wanoise.SendRequestExtra
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sent, gotExtra = m, extra
			return wanoise.SendResponse{ID: "template-wire-id", Timestamp: time.Unix(1755500130, 0)}, nil
		},
	}

	if _, err := templateAdapter(f).SendTemplate(context.Background(), "u1", templateChatJID, payload, nil, nil, id); err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	if sent == nil {
		t.Fatal("SendMessage nao foi chamado")
	}
	return sent, gotExtra
}

// hydratedButtons extrai os botões do template hidratado, falhando com
// mensagem útil se a mensagem não tiver a forma esperada.
func hydratedButtons(t *testing.T, m *waE2E.Message) []*waE2E.HydratedTemplateButton {
	t.Helper()
	if m.GetTemplateMessage() == nil {
		t.Fatalf("a mensagem enviada nao e' um TemplateMessage: %+v", m)
	}
	tpl := m.GetTemplateMessage().GetHydratedTemplate()
	if tpl == nil {
		t.Fatal("TemplateMessage sem HydratedTemplate")
	}
	return tpl.GetHydratedButtons()
}

func TestChatMessengerAdapter_SendTemplate_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))

	_, err := a.SendTemplate(context.Background(), "u1", templateChatJID,
		domain.TemplatePayload{Content: "c", Footer: "f"}, nil, nil, "")

	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendTemplate code = %q, quero no_session", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendTemplate_HydratedFourRowTemplateFields trava os
// QUATRO campos que o histórico preenchia (`git show 41bc8e2^:handlers.go`,
// linha 3226) e a AUSÊNCIA dos que ele não preenchia. TemplateId="1" é
// literal no histórico e vive aqui como constante nomeada (ADR-0004).
func TestChatMessengerAdapter_SendTemplate_HydratedFourRowTemplateFields(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "Escolha uma opcao",
		Footer:  "Equipe wa-api",
		Buttons: []domain.TemplateButton{{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply}},
	}, nil, "")

	tpl := sent.GetTemplateMessage().GetHydratedTemplate()
	if tpl == nil {
		t.Fatalf("a mensagem enviada nao e' um template hidratado: %+v", sent)
	}
	if got := tpl.GetHydratedContentText(); got != "Escolha uma opcao" {
		t.Errorf("HydratedContentText = %q, quero o Content do payload", got)
	}
	if got := tpl.GetHydratedFooterText(); got != "Equipe wa-api" {
		t.Errorf("HydratedFooterText = %q, quero o Footer do payload", got)
	}
	if got := tpl.GetTemplateID(); got != "1" {
		t.Errorf("TemplateID = %q, quero %q (o literal do historico)", got, "1")
	}
	if n := len(tpl.GetHydratedButtons()); n != 1 {
		t.Fatalf("HydratedButtons tem %d entrada(s), quero 1", n)
	}
	// O histórico nunca preenchia o oneof de título nem MaskLinkedDevices;
	// preenchê-los mudaria a forma da mensagem no aparelho de quem recebe.
	if tpl.GetHydratedTitleText() != "" {
		t.Errorf("HydratedTitleText = %q; o historico nunca o preenchia", tpl.GetHydratedTitleText())
	}
	if tpl.GetImageMessage() != nil || tpl.GetVideoMessage() != nil ||
		tpl.GetDocumentMessage() != nil || tpl.GetLocationMessage() != nil {
		t.Error("o oneof de titulo foi preenchido; o historico nunca o preenchia")
	}
}

// TestChatMessengerAdapter_SendTemplate_ButtonTypes é o teste por TIPO: cada
// um dos três produz o seu protobuf, com os campos que o histórico preenchia
// para ele e nenhum outro.
func TestChatMessengerAdapter_SendTemplate_ButtonTypes(t *testing.T) {
	t.Run("quickreply", func(t *testing.T) {
		sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
			Content: "c", Footer: "f",
			Buttons: []domain.TemplateButton{
				{DisplayText: "Confirmar", ID: "cta-42", Type: domain.TemplateButtonQuickReply},
			},
		}, nil, "")

		btns := hydratedButtons(t, sent)
		if n := len(btns); n != 1 {
			t.Fatalf("quero 1 botao, got %d", n)
		}
		qr := btns[0].GetQuickReplyButton()
		if qr == nil {
			t.Fatalf("o botao nao virou QuickReplyButton: %+v", btns[0].GetHydratedButton())
		}
		if got := qr.GetDisplayText(); got != "Confirmar" {
			t.Errorf("DisplayText = %q, quero %q", got, "Confirmar")
		}
		if got := qr.GetID(); got != "cta-42" {
			t.Errorf("ID = %q, quero %q", got, "cta-42")
		}
	})

	t.Run("url", func(t *testing.T) {
		sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
			Content: "c", Footer: "f",
			Buttons: []domain.TemplateButton{
				{DisplayText: "Abrir site", URL: "https://example.invalid/promo", Type: domain.TemplateButtonURL},
			},
		}, nil, "")

		btns := hydratedButtons(t, sent)
		url := btns[0].GetUrlButton()
		if url == nil {
			t.Fatalf("o botao nao virou UrlButton: %+v", btns[0].GetHydratedButton())
		}
		if got := url.GetDisplayText(); got != "Abrir site" {
			t.Errorf("DisplayText = %q, quero %q", got, "Abrir site")
		}
		if got := url.GetURL(); got != "https://example.invalid/promo" {
			t.Errorf("URL = %q, quero o Url do botao de dominio", got)
		}
		// O histórico não preenchia ConsentedUsersURL nem
		// WebviewPresentation neste botão.
		if url.GetConsentedUsersURL() != "" {
			t.Errorf("ConsentedUsersURL = %q; o historico nunca o preenchia", url.GetConsentedUsersURL())
		}
	})

	t.Run("call", func(t *testing.T) {
		sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
			Content: "c", Footer: "f",
			Buttons: []domain.TemplateButton{
				{DisplayText: "Ligar agora", PhoneNumber: "+5511987654321", Type: domain.TemplateButtonCall},
			},
		}, nil, "")

		btns := hydratedButtons(t, sent)
		call := btns[0].GetCallButton()
		if call == nil {
			t.Fatalf("o botao nao virou CallButton: %+v", btns[0].GetHydratedButton())
		}
		if got := call.GetDisplayText(); got != "Ligar agora" {
			t.Errorf("DisplayText = %q, quero %q", got, "Ligar agora")
		}
		if got := call.GetPhoneNumber(); got != "+5511987654321" {
			t.Errorf("PhoneNumber = %q, quero o PhoneNumber do botao de dominio", got)
		}
	})
}

// TestChatMessengerAdapter_SendTemplate_UnknownTypeFallsBackToQuickReply: o
// ramo `default` do histórico. Um Type desconhecido não é recusado nem
// descartado — vira quickreply.
func TestChatMessengerAdapter_SendTemplate_UnknownTypeFallsBackToQuickReply(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{
			{DisplayText: "Talvez", Type: "tipo-que-nao-existe"},
		},
	}, nil, "")

	btns := hydratedButtons(t, sent)
	if n := len(btns); n != 1 {
		t.Fatalf("quero 1 botao, got %d", n)
	}
	qr := btns[0].GetQuickReplyButton()
	if qr == nil {
		t.Fatalf("Type desconhecido nao caiu em QuickReplyButton: %+v", btns[0].GetHydratedButton())
	}
	if got := qr.GetDisplayText(); got != "Talvez" {
		t.Errorf("DisplayText = %q, quero %q", got, "Talvez")
	}
	if got := qr.GetID(); got != "1" {
		t.Errorf("ID = %q, quero %q — o ramo default tem de numerar com strconv.Itoa, "+
			"nao com string(int), que produz o caractere de controle \\x01 (HOUSEKEEP F140)", got, "1")
	}
}

// TestChatMessengerAdapter_SendTemplate_AutomaticButtonNumbering é O TESTE do
// defeito histórico. Assere o TEXTO do ID de cada botão sem ID próprio: o
// primeiro é "1", o segundo é "2". Uma asserção de presença passaria com
// `string(rune(id))` no lugar de `strconv.Itoa(id)`, que produz "\x01" e
// "\x02" — bytes de controle que nenhum cliente reconhece.
func TestChatMessengerAdapter_SendTemplate_AutomaticButtonNumbering(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{
			{DisplayText: "Primeiro", Type: domain.TemplateButtonQuickReply},
			{DisplayText: "Segundo", Type: domain.TemplateButtonQuickReply},
		},
	}, nil, "")

	btns := hydratedButtons(t, sent)
	if n := len(btns); n != 2 {
		t.Fatalf("quero 2 botoes, got %d", n)
	}

	want := []struct{ text, id string }{{"Primeiro", "1"}, {"Segundo", "2"}}
	for i, w := range want {
		qr := btns[i].GetQuickReplyButton()
		if qr == nil {
			t.Fatalf("botao[%d] nao e' QuickReplyButton: %+v", i, btns[i].GetHydratedButton())
		}
		if got := qr.GetDisplayText(); got != w.text {
			t.Errorf("botao[%d] DisplayText = %q, quero %q (a ORDEM tem de ser preservada)", i, got, w.text)
		}
		if got := qr.GetID(); got != w.id {
			t.Errorf("botao[%d] ID = %q (bytes %v), quero exatamente %q. "+
				"`string(int)` em Go converte para RUNE: string(1) e' \"\\x01\", nao \"1\" "+
				"(HOUSEKEEP F140)", i, got, []byte(got), w.id)
		}
	}
}

// TestChatMessengerAdapter_SendTemplate_ExplicitIDBeatsNumbering: o ID do
// payload vence a numeração automática, e o CONTADOR segue andando — o botão
// seguinte recebe "2", não "1". É a semântica do histórico, em que `id++`
// acontecia fora do `if`.
func TestChatMessengerAdapter_SendTemplate_ExplicitIDBeatsNumbering(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{
			{DisplayText: "Primeiro", ID: "id-do-cliente", Type: domain.TemplateButtonQuickReply},
			{DisplayText: "Segundo", Type: domain.TemplateButtonQuickReply},
		},
	}, nil, "")

	btns := hydratedButtons(t, sent)
	if n := len(btns); n != 2 {
		t.Fatalf("quero 2 botoes, got %d", n)
	}
	if got := btns[0].GetQuickReplyButton().GetID(); got != "id-do-cliente" {
		t.Errorf("botao[0] ID = %q, quero o ID do payload", got)
	}
	if got := btns[1].GetQuickReplyButton().GetID(); got != "2" {
		t.Errorf("botao[1] ID = %q, quero %q: o contador anda mesmo quando o botao "+
			"anterior trouxe ID proprio", got, "2")
	}
}

// TestChatMessengerAdapter_SendTemplate_NumberingCountsNonQuickReplyButtons:
// url e call não usam o número, mas CONSOMEM uma posição do contador — o
// histórico incrementava `id` fora do switch. Um quickreply depois de um url
// recebe "2".
func TestChatMessengerAdapter_SendTemplate_NumberingCountsNonQuickReplyButtons(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{
			{DisplayText: "Site", URL: "https://example.invalid/a", Type: domain.TemplateButtonURL},
			{DisplayText: "Confirmar", Type: domain.TemplateButtonQuickReply},
		},
	}, nil, "")

	btns := hydratedButtons(t, sent)
	if n := len(btns); n != 2 {
		t.Fatalf("quero 2 botoes, got %d", n)
	}
	if btns[0].GetUrlButton() == nil {
		t.Fatalf("botao[0] nao e' UrlButton: %+v", btns[0].GetHydratedButton())
	}
	if got := btns[1].GetQuickReplyButton().GetID(); got != "2" {
		t.Errorf("botao[1] ID = %q, quero %q: o botao de url consome uma posicao do contador", got, "2")
	}
}

// TestChatMessengerAdapter_SendTemplate_ButtonOrderIsPreserved trava a ordem
// com os três tipos misturados: é a ordem em que os botões aparecem no
// aparelho de quem recebe.
func TestChatMessengerAdapter_SendTemplate_ButtonOrderIsPreserved(t *testing.T) {
	sent, _ := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{
			{DisplayText: "A", PhoneNumber: "+551100000000", Type: domain.TemplateButtonCall},
			{DisplayText: "B", Type: domain.TemplateButtonQuickReply},
			{DisplayText: "C", URL: "https://example.invalid/c", Type: domain.TemplateButtonURL},
		},
	}, nil, "")

	btns := hydratedButtons(t, sent)
	if n := len(btns); n != 3 {
		t.Fatalf("quero 3 botoes, got %d", n)
	}
	if got := btns[0].GetCallButton(); got == nil || got.GetDisplayText() != "A" {
		t.Errorf("botao[0] = %+v, quero o CallButton \"A\"", btns[0].GetHydratedButton())
	}
	if got := btns[1].GetQuickReplyButton(); got == nil || got.GetDisplayText() != "B" {
		t.Errorf("botao[1] = %+v, quero o QuickReplyButton \"B\"", btns[1].GetHydratedButton())
	}
	if got := btns[2].GetUrlButton(); got == nil || got.GetDisplayText() != "C" {
		t.Errorf("botao[2] = %+v, quero o UrlButton \"C\"", btns[2].GetHydratedButton())
	}
}

// TestChatMessengerAdapter_SendTemplate_IDForwardedAndServerIDWins: o id
// pedido pelo chamador vira RequestExtra.ID, e o resultado devolve o ID e o
// Timestamp que a sessão REALMENTE usou.
func TestChatMessengerAdapter_SendTemplate_IDForwardedAndServerIDWins(t *testing.T) {
	sentAt := time.Unix(1755500130, 0)
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, _ *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			if len(extra) != 1 || string(extra[0].ID) != "id-do-cliente" {
				t.Errorf("RequestExtra = %+v, quero exatamente um com ID \"id-do-cliente\"", extra)
			}
			return wanoise.SendResponse{ID: "id-que-o-sdk-usou", Timestamp: sentAt}, nil
		},
	}

	res, err := templateAdapter(f).SendTemplate(context.Background(), "u1", templateChatJID,
		domain.TemplatePayload{Content: "c", Footer: "f",
			Buttons: []domain.TemplateButton{{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply}}},
		nil, nil, "id-do-cliente")

	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	if res.ID != "id-que-o-sdk-usou" {
		t.Errorf("ID = %q, quero o que a sessao devolveu", res.ID)
	}
	if !res.Timestamp.Equal(sentAt) {
		t.Errorf("Timestamp = %v, quero %v", res.Timestamp, sentAt)
	}
}

// TestChatMessengerAdapter_SendTemplate_NoIDSendsNoRequestExtra: sem id, nada
// de RequestExtra — o SDK gera o dele, mesma disciplina de SendLocation.
func TestChatMessengerAdapter_SendTemplate_NoIDSendsNoRequestExtra(t *testing.T) {
	_, extra := sendTemplateCapturing(t, domain.TemplatePayload{
		Content: "c", Footer: "f",
		Buttons: []domain.TemplateButton{{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply}},
	}, nil, "")

	if len(extra) != 0 {
		t.Errorf("RequestExtra = %+v, quero nenhum quando o id de entrada e' vazio", extra)
	}
}

// TestChatMessengerAdapter_SendTemplate_SendFailurePropagates: quando o envio
// falha, o erro sobe e o resultado é zero — nunca um sucesso fabricado.
func TestChatMessengerAdapter_SendTemplate_SendFailurePropagates(t *testing.T) {
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{}, testkit.ErrSynthetic
		},
	}

	res, err := templateAdapter(f).SendTemplate(context.Background(), "u1", templateChatJID,
		domain.TemplatePayload{Content: "c", Footer: "f",
			Buttons: []domain.TemplateButton{{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply}}}, nil, nil, "")

	if err == nil {
		t.Fatal("envio falho nao propagou erro")
	}
	if res.ID != "" || !res.Timestamp.IsZero() {
		t.Errorf("envio falho devolveu resultado preenchido: %+v", res)
	}
}
