package message_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// TestSendMessage_MissingRequiredField: Phone ou Body ausente é recusado
// antes de qualquer porta ser tocada.
func TestSendMessage_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendMessageRequest
	}{
		{"Phone", domain.SendMessageRequest{Body: "Ola"}},
		{"Body", domain.SendMessageRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm := &contractsfake.TextMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(tm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(tm.SendTextCalls); n != 0 {
				t.Errorf("validacao falhou mas SendText foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendMessage_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendText nunca é tocado.
func TestSendMessage_SessionFailurePropagates(t *testing.T) {
	tm := &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Errorf("sem sessao, mas SendText foi chamado %d vez(es)", n)
	}
}

// TestSendMessage_InvalidPhoneNeverReachesSendText: JID que não resolve é
// recusado como validation error e SendText jamais é chamado — o guardrail
// contra falso-sucesso de JID inválido do CAP-01.
func TestSendMessage_InvalidPhoneNeverReachesSendText(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	logger := &contractsfake.Logger{}

	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendMessageUseCase(tm, jr, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "lixo", Body: "Ola"})

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendText foi chamado %d vez(es)", n)
	}
}

// TestSendMessage_CausalSuccess é o teste da causa (política anti-regressão
// do HOUSEKEEP, ARMADILHA 2): um envio bem-sucedido OBRIGATORIAMENTE chama
// SendText, com o destinatário e o texto corretos, exatamente uma vez, e
// Status só vale StatusSent DEPOIS que a porta devolveu sucesso.
func TestSendMessage_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, target domain.JID, text string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola mundo"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es), quero exatamente 1", n)
	}
	call := tm.SendTextCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if call.Text != "Ola mundo" {
		t.Errorf("texto: got %q, want %q", call.Text, "Ola mundo")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-123")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendMessage_MessageIDIsTheOneActuallySent trava a decisão 3 do
// CAP-01: o MessageID publicado é o que a porta devolveu, mesmo quando
// diverge do id de entrada — nunca o id do request usado "às cegas".
func TestSendMessage_MessageIDIsTheOneActuallySent(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola", ID: "id-pedido-pelo-cliente"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q), nao o pedido pelo cliente", result.MessageID, "id-que-o-sdk-usou")
	}
	if len(tm.SendTextCalls) != 1 || tm.SendTextCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", tm.SendTextCalls)
	}
}

// TestSendMessage_DownstreamFailureNeverProducesSent: SendText falhando não
// pode virar Status=StatusSent nem resultado não-nil.
func TestSendMessage_DownstreamFailureNeverProducesSent(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola"})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	if !errors.Is(err, errDownstream) {
		t.Fatalf("causa nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil devolvido junto com erro: %+v", result)
	}
}

// --- CAP-01.1: domain.SendMessageRequest.LinkPreview ---------------------

// TestSendMessage_LinkPreviewDisabled_NeverTouchesFetcherOrChangesText é o
// teste de conservação (PARTE 4 do CAP-01.1, LP-1): sem LinkPreview, o
// fetcher de preview nunca é consultado e SendText recebe preview=nil — o
// caminho exato do CAP-01, sem mudança nenhuma.
func TestSendMessage_LinkPreviewDisabled_NeverTouchesFetcherOrChangesText(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	lpf := &contractsfake.LinkPreviewFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, lpf, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "Ola mundo, sem link"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(lpf.FetchLinkPreviewCalls); n != 0 {
		t.Errorf("LinkPreview=false, mas o fetcher foi consultado %d vez(es)", n)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es), quero exatamente 1", n)
	}
	if tm.SendTextCalls[0].Preview != nil {
		t.Errorf("preview: got %+v, want nil (caminho do CAP-01 sem LinkPreview)", tm.SendTextCalls[0].Preview)
	}
}

// TestSendMessage_LinkPreviewRequested_BuildsPreviewFromFetcher (LP-2):
// LinkPreview=true com URL no corpo repassa ao fetcher o Body inteiro e
// encaminha a domain.LinkPreviewData resolvida, ESTRUTURA completa, para
// SendText — não só o status.
func TestSendMessage_LinkPreviewRequested_BuildsPreviewFromFetcher(t *testing.T) {
	wantPreview := domain.LinkPreviewData{
		MatchedURL:    "https://exemplo.com/pagina",
		Title:         "Título da Página",
		Description:   "Descrição via Open Graph",
		ThumbnailJPEG: []byte{0xFF, 0xD8, 0xFF},
	}
	tm := &contractsfake.TextMessenger{}
	lpf := &contractsfake.LinkPreviewFetcher{
		FetchLinkPreviewFunc: func(_ context.Context, text string) (domain.LinkPreviewData, bool) {
			if text != "confere https://exemplo.com/pagina isto" {
				t.Errorf("texto repassado ao fetcher: got %q", text)
			}
			return wantPreview, true
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, lpf, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{
			Phone: "5511987654321", Body: "confere https://exemplo.com/pagina isto", LinkPreview: true,
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es), quero exatamente 1", n)
	}
	got := tm.SendTextCalls[0].Preview
	if got == nil {
		t.Fatal("preview nao foi repassado a SendText")
	}
	if !reflect.DeepEqual(*got, wantPreview) {
		t.Errorf("preview: got %+v, want %+v", *got, wantPreview)
	}
}

// TestSendMessage_LinkPreviewRequested_NoURLFound_FallsBackToPlainText:
// LinkPreview=true sem nenhuma URL no corpo (fetcher devolve found=false)
// não deve inventar um ExtendedTextMessage vazio — cai no mesmo caminho do
// CAP-01, preview=nil.
func TestSendMessage_LinkPreviewRequested_NoURLFound_FallsBackToPlainText(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	lpf := &contractsfake.LinkPreviewFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, lpf, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "sem link nenhum", LinkPreview: true})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(lpf.FetchLinkPreviewCalls); n != 1 {
		t.Fatalf("fetcher consultado %d vez(es), quero exatamente 1", n)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es), quero exatamente 1", n)
	}
	if tm.SendTextCalls[0].Preview != nil {
		t.Errorf("preview: got %+v, want nil (nenhuma URL no corpo)", tm.SendTextCalls[0].Preview)
	}
}

// TestSendMessage_LinkPreviewRequested_SenderRealContinuaObrigatorio (LP-3):
// LinkPreview=true NUNCA pode pular o envio real — SendText é chamado
// exatamente uma vez, e Status só vale StatusSent depois que ele devolve
// sucesso. Guardrail contra voltar a um estado "validado mas não enviado".
func TestSendMessage_LinkPreviewRequested_SenderRealContinuaObrigatorio(t *testing.T) {
	sentAt := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-preview", Timestamp: sentAt}, nil
		},
	}
	lpf := &contractsfake.LinkPreviewFetcher{
		FetchLinkPreviewFunc: func(context.Context, string) (domain.LinkPreviewData, bool) {
			return domain.LinkPreviewData{MatchedURL: "https://exemplo.com"}, true
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, lpf, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{
			Phone: "5511987654321", Body: "veja https://exemplo.com", LinkPreview: true,
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es), quero exatamente 1 — LinkPreview nao pode pular o envio real", n)
	}
	// LP-4: resultado vem do envio real, nao de valor inventado.
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-preview" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-id-preview")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendMessage_LinkPreviewRequested_DownstreamFailureNeverProducesSent
// (LP-5): com LinkPreview=true, uma falha de SendText (protocolo/upload)
// não pode virar StatusSent nem resultado não-nil — a mesma garantia do
// CAP-01 contra falso-sucesso, agora no caminho com preview.
func TestSendMessage_LinkPreviewRequested_DownstreamFailureNeverProducesSent(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	lpf := &contractsfake.LinkPreviewFetcher{
		FetchLinkPreviewFunc: func(context.Context, string) (domain.LinkPreviewData, bool) {
			return domain.LinkPreviewData{MatchedURL: "https://exemplo.com"}, true
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, lpf, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{
			Phone: "5511987654321", Body: "veja https://exemplo.com", LinkPreview: true,
		})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	if !errors.Is(err, errDownstream) {
		t.Fatalf("causa nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil devolvido junto com erro: %+v", result)
	}
}

// --- CAP-46A: domain.SendMessageRequest.ReplyTo -------------------------

// TestSendMessage_ReplyTo_ForwardedToPort: ReplyTo on the request is
// forwarded as-is to SendText — the use case does not interpret it.
func TestSendMessage_ReplyTo_ForwardedToPort(t *testing.T) {
	wantReply := &domain.ReplyContext{
		StanzaID:    "quoted-abc",
		Participant: "5511888888888@s.whatsapp.net",
		QuotedText:  "original text",
	}
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-reply"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{
			Phone: "5511987654321", Body: "my reply", ReplyTo: wantReply,
		})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText called %d time(s), want 1", n)
	}
	got := tm.SendTextCalls[0].ReplyTo
	if got == nil {
		t.Fatal("ReplyTo not forwarded to SendText")
	}
	if got.StanzaID != wantReply.StanzaID {
		t.Errorf("StanzaID = %q, want %q", got.StanzaID, wantReply.StanzaID)
	}
	if got.Participant != wantReply.Participant {
		t.Errorf("Participant = %q, want %q", got.Participant, wantReply.Participant)
	}
	if got.QuotedText != wantReply.QuotedText {
		t.Errorf("QuotedText = %q, want %q", got.QuotedText, wantReply.QuotedText)
	}
}

// TestSendMessage_WithoutReplyTo_SendTextReceivesNil: without ReplyTo,
// SendText receives nil — the anti-regression test ensuring the feature
// does not break the fourteen existing flows.
func TestSendMessage_WithoutReplyTo_SendTextReceivesNil(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendMessageUseCase(tm, &contractsfake.JIDResolver{}, &contractsfake.LinkPreviewFetcher{}, logger).
		Execute(context.Background(), userID, domain.SendMessageRequest{Phone: "5511987654321", Body: "plain"})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText called %d time(s), want 1", n)
	}
	if tm.SendTextCalls[0].ReplyTo != nil {
		t.Errorf("ReplyTo = %+v, want nil (no ReplyTo in request)", tm.SendTextCalls[0].ReplyTo)
	}
}
