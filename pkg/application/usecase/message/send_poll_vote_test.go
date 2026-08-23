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

func validPollVoteRequest() domain.SendPollVoteRequest {
	return domain.SendPollVoteRequest{
		Phone:                "120363313346913103@g.us",
		Sender:               "5511999999999@s.whatsapp.net",
		PollMessageID:        "3EB0POLL1",
		PollMessageTimestamp: 1755500100,
		Options:              []string{"12h"},
	}
}

func TestSendPollVote_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendPollVoteRequest
	}{
		{"Phone", domain.SendPollVoteRequest{Sender: "5511999999999@s.whatsapp.net", PollMessageID: "3EB0POLL1", PollMessageTimestamp: 1755500100, Options: []string{"12h"}}},
		{"PollMessageId", domain.SendPollVoteRequest{Phone: "120363313346913103@g.us", Sender: "5511999999999@s.whatsapp.net", PollMessageTimestamp: 1755500100, Options: []string{"12h"}}},
		{"PollMessageTimestamp", domain.SendPollVoteRequest{Phone: "120363313346913103@g.us", Sender: "5511999999999@s.whatsapp.net", PollMessageID: "3EB0POLL1", Options: []string{"12h"}}},
		{"Sender", domain.SendPollVoteRequest{Phone: "120363313346913103@g.us", PollMessageID: "3EB0POLL1", PollMessageTimestamp: 1755500100, Options: []string{"12h"}}},
		{"Options_nenhuma", domain.SendPollVoteRequest{Phone: "120363313346913103@g.us", Sender: "5511999999999@s.whatsapp.net", PollMessageID: "3EB0POLL1", PollMessageTimestamp: 1755500100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request invalido (%s) foi aceito", tc.name)
			}
			if n := len(cm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(cm.SendPollVoteCalls); n != 0 {
				t.Errorf("validacao falhou mas SendPollVote foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendPollVote_OneOptionIsTheBoundary(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validPollVoteRequest())

	if err != nil {
		t.Fatalf("uma opcao e' o minimo ACEITO, mas foi recusado: %v", err)
	}
	if n := len(cm.SendPollVoteCalls); n != 1 {
		t.Fatalf("SendPollVote chamado %d vez(es), quero exatamente 1", n)
	}
}

func TestSendPollVote_SessionFailurePropagates(t *testing.T) {
	cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validPollVoteRequest())

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Errorf("sem sessao, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_InvalidPhoneNeverReachesSendPollVote(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "lixo" {
				return "", errJID
			}
			return domain.JID(raw), nil
		},
	}

	req := validPollVoteRequest()
	req.Phone = "lixo"

	_, err := message.NewSendPollVoteUseCase(cm, jr, logger).
		Execute(context.Background(), userID, req)

	if err == nil {
		t.Fatal("phone invalido foi aceito")
	}
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_InvalidSenderNeverReachesSendPollVote(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}

	callCount := 0
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			callCount++
			if callCount == 2 {
				return "", errJID
			}
			return domain.JID(raw), nil
		},
	}

	req := validPollVoteRequest()
	req.Sender = "sender-lixo"

	_, err := message.NewSendPollVoteUseCase(cm, jr, logger).
		Execute(context.Background(), userID, req)

	if err == nil {
		t.Fatal("sender invalido foi aceito")
	}
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("Sender invalido, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_PhoneAndSenderResolvedWithResolveJID(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{}

	_, err := message.NewSendPollVoteUseCase(cm, jr, logger).
		Execute(context.Background(), userID, validPollVoteRequest())

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(jr.ResolveJIDCalls); n != 2 {
		t.Fatalf("ResolveJID chamado %d vez(es), quero 2 (Phone + Sender)", n)
	}
	if got := jr.ResolveJIDCalls[0].Raw; got != "120363313346913103@g.us" {
		t.Errorf("ResolveJID[0] recebeu %q, quero o Phone cru", got)
	}
	if got := jr.ResolveJIDCalls[1].Raw; got != "5511999999999@s.whatsapp.net" {
		t.Errorf("ResolveJID[1] recebeu %q, quero o Sender cru", got)
	}
	if n := len(jr.ResolveQualifiedJIDCalls); n != 0 {
		t.Errorf("ResolveQualifiedJID chamado %d vez(es); a regra e' ResolveJID", n)
	}
}

func TestSendPollVote_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 23, 10, 30, 0, 0, time.UTC)
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(context.Context, string, domain.JID, domain.PollVotePayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-pollvote-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validPollVoteRequest())

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(cm.SendPollVoteCalls); n != 1 {
		t.Fatalf("SendPollVote chamado %d vez(es), quero exatamente 1", n)
	}
	call := cm.SendPollVoteCalls[0]
	if call.Target != domain.JID("120363313346913103@g.us") {
		t.Errorf("target: got %q, want %q", call.Target, "120363313346913103@g.us")
	}
	if call.Payload.PollChat != domain.JID("120363313346913103@g.us") {
		t.Errorf("PollChat: got %q, want %q", call.Payload.PollChat, "120363313346913103@g.us")
	}
	if call.Payload.PollSender != domain.JID("5511999999999@s.whatsapp.net") {
		t.Errorf("PollSender: got %q, want %q", call.Payload.PollSender, "5511999999999@s.whatsapp.net")
	}
	if call.Payload.PollMessageID != "3EB0POLL1" {
		t.Errorf("PollMessageID: got %q, want %q", call.Payload.PollMessageID, "3EB0POLL1")
	}
	if call.Payload.PollTimestamp != 1755500100 {
		t.Errorf("PollTimestamp: got %d, want %d", call.Payload.PollTimestamp, 1755500100)
	}
	if len(call.Payload.OptionNames) != 1 || call.Payload.OptionNames[0] != "12h" {
		t.Errorf("OptionNames: got %q, want [12h]", call.Payload.OptionNames)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-pollvote-123" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-id-pollvote-123")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

func TestSendPollVote_SendFailureNeverReportsSent(t *testing.T) {
	sendErr := errors.New("porta: envio de voto recusado pelo servidor")
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(context.Context, string, domain.JID, domain.PollVotePayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, validPollVoteRequest())

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: got %#v", err)
	}
	if result != nil {
		t.Fatalf("envio falhou mas o use case devolveu resultado: %+v", result)
	}
}

func TestSendPollVote_MessageIDIsTheOneActuallySent(t *testing.T) {
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(context.Context, string, domain.JID, domain.PollVotePayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	req := validPollVoteRequest()
	req.ID = "id-pedido-pelo-cliente"

	result, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "id-que-o-sdk-usou")
	}
	if len(cm.SendPollVoteCalls) != 1 || cm.SendPollVoteCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", cm.SendPollVoteCalls)
	}
}

func TestSendPollVote_MultipleOptionsAreAccepted(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}

	req := validPollVoteRequest()
	req.Options = []string{"12h", "13h", "14h"}

	_, err := message.NewSendPollVoteUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, req)

	if err != nil {
		t.Fatalf("multiplas opcoes devem ser aceitas (protocolo nao limita no voto): %v", err)
	}
	if n := len(cm.SendPollVoteCalls); n != 1 {
		t.Fatalf("SendPollVote chamado %d vez(es), quero exatamente 1", n)
	}
	if len(cm.SendPollVoteCalls[0].Payload.OptionNames) != 3 {
		t.Errorf("OptionNames: got %d opcoes, want 3", len(cm.SendPollVoteCalls[0].Payload.OptionNames))
	}
}
