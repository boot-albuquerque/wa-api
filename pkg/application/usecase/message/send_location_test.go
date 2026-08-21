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

// TestSendLocation_MissingRequiredField: Phone, Latitude ou Longitude
// ausente é recusado antes de qualquer porta ser tocada. Latitude/Longitude
// "ausente" é indistinguível de "zero" na desserialização de float64 sem
// ponteiro — ver TestSendLocation_ZeroLatitudeRejected/
// TestSendLocation_ZeroLongitudeRejected, que documentam a consequência.
func TestSendLocation_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendLocationRequest
	}{
		{"Phone", domain.SendLocationRequest{Latitude: -23.5505, Longitude: -46.6333}},
		{"Latitude", domain.SendLocationRequest{Phone: "5511987654321", Longitude: -46.6333}},
		{"Longitude", domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5505}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(sm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sm.SendLocationCalls); n != 0 {
				t.Errorf("validacao falhou mas SendLocation foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendLocation_ZeroLatitudeRejected documenta o defeito HISTÓRICO
// (`git show 41bc8e2^:handlers.go`, em torno da linha 1913): a validação
// `Latitude == 0` confunde "campo ausente" com "valor zero", então
// qualquer ponto sobre o equador é rejeitado como se Latitude não tivesse
// sido enviada. Isto NÃO é o comportamento desejado — é o comportamento
// preservado por decisão explícita (CAP-08A não corrige contrato público
// sem autorização, ver HOUSEKEEP.md F121). A causa raiz é
// domain.SendLocationRequest.Latitude ser float64 sem ponteiro: corrigir
// exigiria *float64, que é mudança de contrato.
func TestSendLocation_ZeroLatitudeRejected(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: 0, Longitude: -46.6333})

	if err == nil {
		t.Fatal("Latitude=0 (equador) foi aceita — defeito historico deixou de ser reproduzido")
	}
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Errorf("Latitude=0 rejeitada, mas SendLocation foi chamado %d vez(es)", n)
	}
}

// TestSendLocation_ZeroLongitudeRejected é o par de
// TestSendLocation_ZeroLatitudeRejected para o meridiano de Greenwich.
func TestSendLocation_ZeroLongitudeRejected(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5505, Longitude: 0})

	if err == nil {
		t.Fatal("Longitude=0 (meridiano de Greenwich) foi aceita — defeito historico deixou de ser reproduzido")
	}
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Errorf("Longitude=0 rejeitada, mas SendLocation foi chamado %d vez(es)", n)
	}
}

// TestSendLocation_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendLocation nunca é tocado.
func TestSendLocation_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5505, Longitude: -46.6333})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Errorf("sem sessao, mas SendLocation foi chamado %d vez(es)", n)
	}
}

// TestSendLocation_InvalidPhoneNeverReachesSendLocation: JID que não
// resolve é recusado como validation error e SendLocation jamais é
// chamado.
func TestSendLocation_InvalidPhoneNeverReachesSendLocation(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendLocationUseCase(sm, jr, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "lixo", Latitude: -23.5505, Longitude: -46.6333})

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendLocation foi chamado %d vez(es)", n)
	}
}

// TestSendLocation_CausalSuccess é o teste da causa (política anti-
// regressão do HOUSEKEEP, ARMADILHA 2): valores negativos e fracionários
// reais (-23.5505, -46.6333 — Praça da Sé, São Paulo) atravessam float64
// sem perda até o payload entregue à porta, e Status só vale StatusSent
// depois que SendLocation devolve sucesso.
func TestSendLocation_CausalSuccess(t *testing.T) {
	sentAt := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(_ context.Context, _ string, target domain.JID, payload domain.LocationPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-id-location-123"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{
			Phone: "5511987654321", Name: "Praça da Sé", Latitude: -23.5505, Longitude: -46.6333,
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(sm.SendLocationCalls); n != 1 {
		t.Fatalf("SendLocation chamado %d vez(es), quero exatamente 1", n)
	}
	call := sm.SendLocationCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if call.Payload.Latitude != -23.5505 {
		t.Errorf("Latitude: got %v, want %v (perda de precisao em float64)", call.Payload.Latitude, -23.5505)
	}
	if call.Payload.Longitude != -46.6333 {
		t.Errorf("Longitude: got %v, want %v (perda de precisao em float64)", call.Payload.Longitude, -46.6333)
	}
	if call.Payload.Name != "Praça da Sé" {
		t.Errorf("Name: got %q, want %q", call.Payload.Name, "Praça da Sé")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-location-123" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-location-123")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt.Unix())
	}
}

// TestSendLocation_NameOptional_EmptyStringForwarded: Name é OPCIONAL —
// sem validação — e vai como LocationMessage.Name mesmo vazio.
func TestSendLocation_NameOptional_EmptyStringForwarded(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5505, Longitude: -46.6333})

	if err != nil {
		t.Fatalf("caminho feliz falhou (Name ausente deveria ser aceito): %v", err)
	}
	if n := len(sm.SendLocationCalls); n != 1 {
		t.Fatalf("SendLocation chamado %d vez(es), quero exatamente 1", n)
	}
	if got := sm.SendLocationCalls[0].Payload.Name; got != "" {
		t.Errorf("Name: got %q, want vazio", got)
	}
}

// TestSendLocation_MessageIDIsTheOneActuallySent: o MessageID publicado é o
// que a porta devolveu, mesmo quando diverge do id de entrada.
func TestSendLocation_MessageIDIsTheOneActuallySent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.LocationPayload, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{
			Phone: "5511987654321", Latitude: -23.5505, Longitude: -46.6333, ID: "id-pedido-pelo-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "id-que-o-sdk-usou")
	}
	if len(sm.SendLocationCalls) != 1 || sm.SendLocationCalls[0].ID != "id-pedido-pelo-cliente" {
		t.Errorf("id do cliente nao foi repassado a porta: %+v", sm.SendLocationCalls)
	}
}

// TestSendLocation_DownstreamFailureNeverProducesSent: SendLocation
// falhando não pode virar Status=StatusSent nem resultado não-nil.
func TestSendLocation_DownstreamFailureNeverProducesSent(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(context.Context, string, domain.JID, domain.LocationPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: -23.5505, Longitude: -46.6333})

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
