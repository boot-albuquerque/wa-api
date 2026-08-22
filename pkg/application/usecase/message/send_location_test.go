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

// coord devolve ponteiro para um literal. A F121 trocou Latitude/Longitude
// para *float64 para distinguir "nao informado" de zero.
func coord(v float64) *float64 { return &v }

// TestSendLocation_MissingRequiredField: Phone, Latitude ou Longitude
// ausente é recusado antes de qualquer porta ser tocada. Latitude/Longitude
// Desde a F121 os campos são *float64, então "ausente" (nil) e "zero" são
// distinguíveis — ver TestSendLocation_ZeroEhCoordenadaValida.
func TestSendLocation_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendLocationRequest
	}{
		{"Phone", domain.SendLocationRequest{Latitude: coord(-23.5505), Longitude: coord(-46.6333)}},
		{"Latitude", domain.SendLocationRequest{Phone: "5511987654321", Longitude: coord(-46.6333)}},
		{"Longitude", domain.SendLocationRequest{Phone: "5511987654321", Latitude: coord(-23.5505)}},
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

// TestSendLocation_ZeroEhCoordenadaValida
//
// Este teste dizia o CONTRÁRIO até 2026-08-22. Chamava-se
// `ZeroLatitudeRejected`/`ZeroLongitudeRejected` e tratava "zero foi aceite"
// como FALHA — travava o defeito histórico em vez do comportamento correto.
//
// Zero é coordenada válida: latitude 0 é o equador, longitude 0 é o meridiano
// de Greenwich, e os dois a zero são um ponto real no golfo da Guiné. A F121
// trocou os campos para ponteiro para separar "não informado" de "zero".
func TestSendLocation_ZeroEhCoordenadaValida(t *testing.T) {
	for _, c := range []struct {
		nome     string
		lat, lon float64
	}{
		{"equador", 0, -46.6333},
		{"meridiano_de_Greenwich", -23.5505, 0},
		{"golfo_da_Guine", 0, 0},
	} {
		t.Run(c.nome, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, &contractsfake.Logger{}).
				Execute(context.Background(), userID, domain.SendLocationRequest{
					Phone: "5511987654321", Latitude: coord(c.lat), Longitude: coord(c.lon)})
			if err != nil {
				t.Fatalf("coordenada valida recusada: %v", err)
			}
			if n := len(sm.SendLocationCalls); n != 1 {
				t.Fatalf("SendLocation chamado %d vez(es), quero 1", n)
			}
			got := sm.SendLocationCalls[0].Payload
			if got.Latitude != c.lat || got.Longitude != c.lon {
				t.Errorf("payload = (%v, %v), quero (%v, %v)", got.Latitude, got.Longitude, c.lat, c.lon)
			}
		})
	}
}

// TestSendLocation_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e SendLocation nunca é tocado.
func TestSendLocation_SessionFailurePropagates(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendLocationUseCase(sm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: coord(-23.5505), Longitude: coord(-46.6333)})

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
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "lixo", Latitude: coord(-23.5505), Longitude: coord(-46.6333)})

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
			Phone: "5511987654321", Name: "Praça da Sé", Latitude: coord(-23.5505), Longitude: coord(-46.6333),
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
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: coord(-23.5505), Longitude: coord(-46.6333)})

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
			Phone: "5511987654321", Latitude: coord(-23.5505), Longitude: coord(-46.6333), ID: "id-pedido-pelo-cliente",
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
		Execute(context.Background(), userID, domain.SendLocationRequest{Phone: "5511987654321", Latitude: coord(-23.5505), Longitude: coord(-46.6333)})

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
