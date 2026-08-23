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

// TestSendContact_MissingRequiredField: Phone, Name ou Vcard ausente é
// recusado antes de qualquer porta ser tocada.
func TestSendContact_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendContactRequest
	}{
		{"Phone", domain.SendContactRequest{Name: "Ana", Vcard: "BEGIN:VCARD"}},
		{"Name", domain.SendContactRequest{Phone: "5511987654321", Vcard: "BEGIN:VCARD"}},
		{"Vcard", domain.SendContactRequest{Phone: "5511987654321", Name: "Ana"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(sm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sm.SendContactCalls); n != 0 {
				t.Errorf("validacao falhou mas SendContact foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendContact_DefaultFakeSuccess_UsesDefaultSentID: sem SendContactFunc
// configurada, o fake devolve contractsfake.DefaultSentContactMessageID —
// cobre o caminho zero-value de contractsfake.SimpleMessenger.SendContact
// (o par de TestSendLocation_NameOptional_EmptyStringForwarded, que já
// cobre o mesmo caminho para SendLocation via o campo Name opcional).
func TestSendContact_DefaultFakeSuccess_UsesDefaultSentID(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{Phone: "5511987654321", Name: "Ana", Vcard: "BEGIN:VCARD"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != contractsfake.DefaultSentContactMessageID {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, contractsfake.DefaultSentContactMessageID)
	}
}

// TestSendContact_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendContact nunca é tocado.
func TestSendContact_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{Phone: "5511987654321", Name: "Ana", Vcard: "BEGIN:VCARD"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(sm.SendContactCalls); n != 0 {
		t.Errorf("sem sessao, mas SendContact foi chamado %d vez(es)", n)
	}
}

// TestSendContact_InvalidPhoneNeverReachesSendContact: JID que não resolve
// é recusado como validation error e SendContact jamais é chamado.
func TestSendContact_InvalidPhoneNeverReachesSendContact(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendContactUseCase(sm, jr, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{Phone: "lixo", Name: "Ana", Vcard: "BEGIN:VCARD"})

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(sm.SendContactCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendContact foi chamado %d vez(es)", n)
	}
}

// TestSendContact_CausalSuccess é o teste da causa (política anti-regressão
// do HOUSEKEEP, ARMADILHA 2): um envio bem-sucedido OBRIGATORIAMENTE chama
// SendContact, com o destinatário, DisplayName e Vcard corretos, exatamente
// uma vez, e Status só vale StatusSent DEPOIS que a porta devolveu sucesso.
func TestSendContact_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	sm := &contractsfake.SimpleMessenger{
		SendContactFunc: func(_ context.Context, _ string, target domain.JID, payload domain.ContactPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-contact-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	vcard := "BEGIN:VCARD\nVERSION:3.0\nFN:Ana\nTEL:5511987654321\nEND:VCARD"
	result, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{Phone: "5511987654321", Name: "Ana", Vcard: vcard})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(sm.SendContactCalls); n != 1 {
		t.Fatalf("SendContact chamado %d vez(es), quero exatamente 1", n)
	}
	call := sm.SendContactCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if call.Payload.Name != "Ana" {
		t.Errorf("Name/DisplayName: got %q, want %q", call.Payload.Name, "Ana")
	}
	if call.Payload.Vcard != vcard {
		t.Errorf("Vcard: got %q, want %q (repassado como string crua, sem parse)", call.Payload.Vcard, vcard)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-contact-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-contact-123")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendContact_MessageIDIsTheOneActuallySent: o MessageID publicado é o
// que a porta devolveu, mesmo quando diverge do id de entrada.
func TestSendContact_MessageIDIsTheOneActuallySent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendContactFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ContactPayload, _ *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{
			Phone: "5511987654321", Name: "Ana", Vcard: "BEGIN:VCARD", ID: "id-pedido-pelo-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "id-que-o-sdk-usou")
	}
	if len(sm.SendContactCalls) != 1 || sm.SendContactCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", sm.SendContactCalls)
	}
}

// TestSendContact_DownstreamFailureNeverProducesSent: SendContact falhando
// não pode virar Status=StatusSent nem resultado não-nil.
func TestSendContact_DownstreamFailureNeverProducesSent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendContactFunc: func(context.Context, string, domain.JID, domain.ContactPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendContactUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendContactRequest{Phone: "5511987654321", Name: "Ana", Vcard: "BEGIN:VCARD"})

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
