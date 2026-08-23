package message_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo recebe os eixos que send_message_test.go cobria para
// SendTemplate enquanto ele era um use case de port.MessageComposer que só
// validava (CAP-15 migrou-o para port.SimpleMessenger, com envio de verdade).
// Mesma estrutura de send_poll_test.go.
//
// O que este bloco tem de próprio está na HOUSEKEEP F139: o DTO tinha perdido
// `Buttons`, e sem ele a capability não tem sentido. Por isso a asserção
// central aqui é que os botões chegam à porta INTEIROS e na ORDEM — a
// tradução de cada um para o protobuf é do adapter, e está medida em
// pkg/infra/wa-noise/adapters/chat/messenger_template_test.go.

const templatePhone = "5511987654321"

// templateButtons são os botões de referência desta suite: um de cada tipo,
// na ordem, para que a ordem preservada seja observável.
func templateButtons() []domain.TemplateButton {
	return []domain.TemplateButton{
		{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply},
		{DisplayText: "Site", URL: "https://example.invalid/a", Type: domain.TemplateButtonURL},
		{DisplayText: "Ligar", PhoneNumber: "+5511987654321", Type: domain.TemplateButtonCall},
	}
}

// templateJIDResolver é o dublê de port.JIDResolver que imita a regra REAL de
// pkg/infra/wa-noise/mapping/jid/parse.go:12 — número sem "@" recebe o
// servidor padrão; com "@", passa intacto. O dublê padrão de contractsfake
// devolve `raw` sem tocar, o que é MAIS PERMISSIVO que a produção e esconderia
// exatamente o defeito de destinatário que este teste existe para pegar
// (ARMADILHA 1).
func templateJIDResolver() *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			raw = strings.TrimPrefix(raw, "+")
			if strings.ContainsRune(raw, '@') {
				return domain.JID(raw), nil
			}
			return domain.JID(raw + "@s.whatsapp.net"), nil
		},
	}
}

func validTemplateRequest() domain.SendTemplateRequest {
	return domain.SendTemplateRequest{
		Phone:   templatePhone,
		Content: "Corpo",
		Footer:  "Rodape",
		Buttons: templateButtons(),
	}
}

// TestSendTemplate_MissingRequiredField: Phone, Content, Footer ou nenhum
// botão é recusado antes de qualquer porta ser tocada. As quatro regras e a
// ordem são as HISTÓRICAS (`git show 41bc8e2^:handlers.go`, função
// SendTemplate, linhas 3137-3155).
func TestSendTemplate_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendTemplateRequest
	}{
		{"Phone", domain.SendTemplateRequest{Content: "Corpo", Footer: "Rodape", Buttons: templateButtons()}},
		{"Content", domain.SendTemplateRequest{Phone: templatePhone, Footer: "Rodape", Buttons: templateButtons()}},
		{"Footer", domain.SendTemplateRequest{Phone: templatePhone, Content: "Corpo", Buttons: templateButtons()}},
		{"Buttons_ausente", domain.SendTemplateRequest{Phone: templatePhone, Content: "Corpo", Footer: "Rodape"}},
		{"Buttons_vazio", domain.SendTemplateRequest{Phone: templatePhone, Content: "Corpo", Footer: "Rodape", Buttons: []domain.TemplateButton{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request invalido (%s) foi aceito", tc.name)
			}
			if n := len(sm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sm.SendTemplateCalls); n != 0 {
				t.Errorf("validacao falhou mas SendTemplate foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendTemplate_OneButtonIsTheBoundary trava o lado ACEITO da regra de
// contagem. Sem ele, `len(Buttons) < 1` poderia virar `< 2` e todos os testes
// de recusa continuariam verdes — é o caminho de SUCESSO da guarda, que a
// ARMADILHA 2 deste repo diz para não deixar sem medição.
func TestSendTemplate_OneButtonIsTheBoundary(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validTemplateRequest()
	req.Buttons = []domain.TemplateButton{{DisplayText: "Sim", Type: domain.TemplateButtonQuickReply}}

	_, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("um botao e' o minimo ACEITO pelo historico, mas foi recusado: %v", err)
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es), quero exatamente 1", n)
	}
}

// TestSendTemplate_UnknownButtonTypeIsAccepted: o histórico tratava Type
// desconhecido como quickreply (ramo `default`), nunca como recusa. Recusar
// aqui rejeitaria payloads que a rota sempre aceitou — e a tradução do
// default está medida no adapter.
func TestSendTemplate_UnknownButtonTypeIsAccepted(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	req := validTemplateRequest()
	req.Buttons = []domain.TemplateButton{{DisplayText: "Que tipo?", Type: "tipo-que-nao-existe"}}

	_, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("Type desconhecido foi recusado: %v (o historico o tratava como quickreply)", err)
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es), quero exatamente 1", n)
	}
}

// TestSendTemplate_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendTemplate nunca é tocado.
func TestSendTemplate_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validTemplateRequest())

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador por identidade: got %#v", err)
	}
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendTemplate foi chamado %d vez(es)", n)
	}
}

// TestSendTemplate_InvalidPhoneNeverReachesSendTemplate: JID que não parseia
// é recusa, e a recusa acontece ANTES do envio.
func TestSendTemplate_InvalidPhoneNeverReachesSendTemplate(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	req := validTemplateRequest()
	req.Phone = "lixo"

	_, err := message.NewSendTemplateUseCase(sm, jr, logger).
		Execute(context.Background(), userID, req)

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendTemplate foi chamado %d vez(es)", n)
	}
}

// TestSendTemplate_PhoneResolvedWithDefaultServerRule trava a REGRA de
// resolução: o histórico passava Phone por parseJID
// (`41bc8e2^:handlers.go`, linha 3157), que é o ResolveJID de hoje — não o
// ResolveQualifiedJID, mais estrito. Trocar um pelo outro passaria a recusar
// entradas que a rota aceita, e nenhum outro teste deste arquivo notaria: o
// dublê responde aos dois.
func TestSendTemplate_PhoneResolvedWithDefaultServerRule(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{}

	_, err := message.NewSendTemplateUseCase(sm, jr, logger).
		Execute(context.Background(), userID, validTemplateRequest())

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(jr.ResolveJIDCalls); n != 1 {
		t.Fatalf("ResolveJID chamado %d vez(es), quero 1", n)
	}
	if got := jr.ResolveJIDCalls[0].Raw; got != templatePhone {
		t.Errorf("ResolveJID recebeu %q, quero o Phone cru", got)
	}
	if n := len(jr.ResolveQualifiedJIDCalls); n != 0 {
		t.Errorf("ResolveQualifiedJID chamado %d vez(es); a regra historica e' ResolveJID", n)
	}
}

// TestSendTemplate_CausalSuccess é o teste da causa: Content e Footer viram
// TemplatePayload, os botões chegam INTEIROS e na ORDEM (é a ordem em que
// aparecem no aparelho de quem recebe), e Status só vale StatusSent depois
// que SendTemplate devolve sucesso.
func TestSendTemplate_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC)
	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(context.Context, string, domain.JID, domain.TemplatePayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-template-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendTemplateUseCase(sm, templateJIDResolver(), logger).
		Execute(context.Background(), userID, validTemplateRequest())

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es), quero exatamente 1", n)
	}
	call := sm.SendTemplateCalls[0]
	if call.Target != domain.JID(templatePhone+"@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, templatePhone+"@s.whatsapp.net")
	}
	if call.Payload.Content != "Corpo" {
		t.Errorf("Content: got %q, want %q", call.Payload.Content, "Corpo")
	}
	if call.Payload.Footer != "Rodape" {
		t.Errorf("Footer: got %q, want %q", call.Payload.Footer, "Rodape")
	}

	want := templateButtons()
	if len(call.Payload.Buttons) != len(want) {
		t.Fatalf("Buttons: got %d botao(oes), want %d", len(call.Payload.Buttons), len(want))
	}
	for i := range want {
		if call.Payload.Buttons[i] != want[i] {
			t.Errorf("Buttons[%d]: got %+v, want %+v (a ORDEM e' a que aparece no aparelho)",
				i, call.Payload.Buttons[i], want[i])
		}
	}

	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-template-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta", result.MessageID)
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendTemplate_SendFailureNeverReportsSent é o teste da ORDEM: quando o
// envio falha, o use case não pode ter produzido um resultado de sucesso. É o
// eixo que o defeito original violava — devolvia "validated" sem nunca
// enviar.
func TestSendTemplate_SendFailureNeverReportsSent(t *testing.T) {
	sendErr := errors.New("porta: envio de template recusado pelo servidor")
	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(context.Context, string, domain.JID, domain.TemplatePayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validTemplateRequest())

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: got %#v", err)
	}
	if result != nil {
		t.Fatalf("envio falhou mas o use case devolveu resultado: %+v", result)
	}
}

// TestSendTemplate_MessageIDIsTheOneActuallySent: o MessageID publicado é o
// que a porta devolveu, mesmo quando diverge do id de entrada — e o id de
// entrada é repassado à porta para que o SDK possa usá-lo.
func TestSendTemplate_MessageIDIsTheOneActuallySent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(context.Context, string, domain.JID, domain.TemplatePayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validTemplateRequest()
	req.ID = "id-pedido-pelo-cliente"

	result, err := message.NewSendTemplateUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta", result.MessageID)
	}
	if len(sm.SendTemplateCalls) != 1 || sm.SendTemplateCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", sm.SendTemplateCalls)
	}
}
