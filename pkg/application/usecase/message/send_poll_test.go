package message_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo recebe os eixos que send_message_test.go cobria para SendPoll
// enquanto ele era um use case de port.MessageComposer que só validava
// (CAP-14 migrou-o para port.SimpleMessenger, com envio de verdade). Mesma
// estrutura de send_location_test.go.

// pollOptions são as opções de referência desta suite. Texto com acento e
// espaço de propósito: é o que atravessa SHA-256 no wire, e um dublê que
// normalizasse acento esconderia divergência de codificação.
func pollOptions() []string { return []string{"Almoço às 12h", "Almoço às 13h"} }

// TestSendPoll_MissingRequiredField: Group, Header ou menos de duas opções é
// recusado antes de qualquer porta ser tocada. A ordem e as três regras são
// as HISTÓRICAS (`git show 41bc8e2^:handlers.go`, função SendPoll).
func TestSendPoll_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendPollRequest
	}{
		{"Group", domain.SendPollRequest{Header: "Qual?", Options: pollOptions()}},
		{"Header", domain.SendPollRequest{Group: "120363313346913103@g.us", Options: pollOptions()}},
		{"Options_nenhuma", domain.SendPollRequest{Group: "120363313346913103@g.us", Header: "Qual?"}},
		{"Options_apenas_uma", domain.SendPollRequest{Group: "120363313346913103@g.us", Header: "Qual?", Options: []string{"sim"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request invalido (%s) foi aceito", tc.name)
			}
			if n := len(sm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sm.SendPollCalls); n != 0 {
				t.Errorf("validacao falhou mas SendPoll foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendPoll_TwoOptionsIsTheBoundary trava o lado ACEITO da regra de
// contagem. Sem ele, `len(Options) < 2` poderia virar `< 3` (ou `<= 2`) e
// todos os testes de recusa continuariam verdes — é o caminho de SUCESSO da
// guarda, que a ARMADILHA 2 deste repo diz para não deixar sem medição.
func TestSendPoll_TwoOptionsIsTheBoundary(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Qual?", Options: []string{"sim", "nao"},
		})

	if err != nil {
		t.Fatalf("duas opcoes é o minimo ACEITO pelo historico, mas foi recusado: %v", err)
	}
	if n := len(sm.SendPollCalls); n != 1 {
		t.Fatalf("SendPoll chamado %d vez(es), quero exatamente 1", n)
	}
}

// TestSendPoll_SessionFailurePropagates: sem sessão, o erro da porta chega ao
// chamador por identidade e SendPoll nunca é tocado.
func TestSendPoll_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Qual?", Options: pollOptions(),
		})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(sm.SendPollCalls); n != 0 {
		t.Errorf("sem sessao, mas SendPoll foi chamado %d vez(es)", n)
	}
}

// TestSendPoll_InvalidGroupNeverReachesSendPoll: JID que não resolve é
// recusado como validation error e SendPoll jamais é chamado.
func TestSendPoll_InvalidGroupNeverReachesSendPoll(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendPollUseCase(sm, jr, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "lixo", Header: "Qual?", Options: pollOptions(),
		})

	if err == nil {
		t.Fatal("grupo invalido foi aceito")
	}
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendPoll foi chamado %d vez(es)", n)
	}
}

// TestSendPoll_GroupResolvedWithDefaultServerRule trava a REGRA de resolução:
// o histórico passava Group por ValidateMessageFields -> ParseJID
// (`41bc8e2^:internal/interfaces/http/handlers/common.go:132`), que é o
// ResolveJID de hoje — não o ResolveQualifiedJID, mais estrito. Trocar um
// pelo outro passaria a recusar entradas que a rota aceita, e nenhum outro
// teste deste arquivo notaria: o dublê responde aos dois.
func TestSendPoll_GroupResolvedWithDefaultServerRule(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{}

	_, err := message.NewSendPollUseCase(sm, jr, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Qual?", Options: pollOptions(),
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(jr.ResolveJIDCalls); n != 1 {
		t.Fatalf("ResolveJID chamado %d vez(es), quero 1", n)
	}
	if got := jr.ResolveJIDCalls[0].Raw; got != "120363313346913103@g.us" {
		t.Errorf("ResolveJID recebeu %q, quero o Group cru", got)
	}
	if n := len(jr.ResolveQualifiedJIDCalls); n != 0 {
		t.Errorf("ResolveQualifiedJID chamado %d vez(es); a regra historica e' ResolveJID", n)
	}
}

// TestSendPoll_CausalSuccess é o teste da causa: Header vira PollPayload.Name,
// Options chega INTEIRO e na ORDEM (a ordem é o que casa hash com texto no
// voto), e Status só vale StatusSent depois que SendPoll devolve sucesso.
func TestSendPoll_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(context.Context, string, domain.JID, domain.PollPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-poll-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Que horas almoçamos?", Options: pollOptions(),
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(sm.SendPollCalls); n != 1 {
		t.Fatalf("SendPoll chamado %d vez(es), quero exatamente 1", n)
	}
	call := sm.SendPollCalls[0]
	if call.Target != domain.JID("120363313346913103@g.us") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "120363313346913103@g.us")
	}
	if call.Payload.Name != "Que horas almoçamos?" {
		t.Errorf("Name: got %q, want o Header (%q)", call.Payload.Name, "Que horas almoçamos?")
	}
	want := pollOptions()
	if len(call.Payload.Options) != len(want) {
		t.Fatalf("Options: got %d opcoes, want %d", len(call.Payload.Options), len(want))
	}
	for i := range want {
		if call.Payload.Options[i] != want[i] {
			t.Errorf("Options[%d]: got %q, want %q (a ORDEM importa: e' o que casa hash com texto)", i, call.Payload.Options[i], want[i])
		}
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-poll-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-poll-123")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendPoll_SendFailureNeverReportsSent é o teste da ORDEM: quando o envio
// falha, o use case não pode ter produzido um resultado de sucesso. É o eixo
// que o defeito original violava — o use case devolvia "validated" sem nunca
// enviar.
func TestSendPoll_SendFailureNeverReportsSent(t *testing.T) {
	sendErr := errors.New("porta: envio de enquete recusado pelo servidor")
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(context.Context, string, domain.JID, domain.PollPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Qual?", Options: pollOptions(),
		})

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: got %#v", err)
	}
	if result != nil {
		t.Fatalf("envio falhou mas o use case devolveu resultado: %+v", result)
	}
}

// TestSendPoll_MessageIDIsTheOneActuallySent: o MessageID publicado é o que a
// porta devolveu, mesmo quando diverge do id de entrada. Importa mais aqui do
// que nas outras capabilities: é por esse ID que o voto procura as opções
// guardadas (pkg/bootstrap/eventhandler_message.go:117).
func TestSendPoll_MessageIDIsTheOneActuallySent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(context.Context, string, domain.JID, domain.PollPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendPollUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendPollRequest{
			Group: "120363313346913103@g.us", Header: "Qual?", Options: pollOptions(), ID: "id-pedido-pelo-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "id-que-o-sdk-usou")
	}
	if len(sm.SendPollCalls) != 1 || sm.SendPollCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", sm.SendPollCalls)
	}
}
