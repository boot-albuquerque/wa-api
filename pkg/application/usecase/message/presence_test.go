package message_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// errJID é a recusa do resolvedor de JID. Distinta de errSession porque os
// dois caminhos convivem no mesmo Execute e o teste precisa saber qual
// disparou.
var errJID = errors.New("porta: telefone nao vira JID")

// errDownstream é a falha do envio propriamente dito, depois de tudo
// validado.
var errDownstream = errors.New("porta: servidor do whatsapp recusou")

const userID = "user-1"

// failJID devolve um JIDResolver que recusa qualquer entrada.
func failJID() *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}
}

// --- ChatPresence ------------------------------------------------------

func TestChatPresence_SessionFailurePropagates(t *testing.T) {
	pc := &contractsfake.PresenceController{SessionGuard: contractsfake.FailSession(errSession)}
	jr := &contractsfake.JIDResolver{}
	logger := &contractsfake.Logger{}

	err := message.NewChatPresenceUseCase(pc, jr, logger).Execute(context.Background(), userID,
		domain.ChatPresenceRequest{Phone: "5511987654321", State: "composing"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(pc.SendChatPresenceCalls); n != 0 {
		t.Errorf("sem sessao, mas a presenca foi enviada %d vez(es)", n)
	}
	assertSessionLog(t, logger, "user_id", userID)
}

func TestChatPresence_RejectsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		req  domain.ChatPresenceRequest
		jr   *contractsfake.JIDResolver
	}{
		{"Phone ausente", domain.ChatPresenceRequest{State: "composing"}, &contractsfake.JIDResolver{}},
		{"State ausente", domain.ChatPresenceRequest{Phone: "5511987654321"}, &contractsfake.JIDResolver{}},
		{"Phone nao resolve", domain.ChatPresenceRequest{Phone: "nao-e-telefone", State: "composing"}, failJID()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &contractsfake.PresenceController{}
			logger := &contractsfake.Logger{}

			err := message.NewChatPresenceUseCase(pc, tt.jr, logger).Execute(context.Background(), userID, tt.req)

			if err == nil {
				t.Fatal("request invalido foi aceito")
			}
			if n := len(pc.SendChatPresenceCalls); n != 0 {
				t.Errorf("request invalido alcancou a porta %d vez(es)", n)
			}
		})
	}
}

func TestChatPresence_DownstreamFailureIsLoggedWithCause(t *testing.T) {
	pc := &contractsfake.PresenceController{
		SendChatPresenceFunc: func(context.Context, string, domain.JID, string, string) error { return errDownstream },
	}
	logger := &contractsfake.Logger{}

	err := message.NewChatPresenceUseCase(pc, &contractsfake.JIDResolver{}, logger).Execute(context.Background(), userID,
		domain.ChatPresenceRequest{Phone: "5511987654321", State: "composing"})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	rec := requireLog(t, logger, contractsfake.LevelError, "Failed to send chat presence")
	if got, _ := rec.Keyval("error"); got != error(errDownstream) {
		t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
	}
}

func TestChatPresence_Success(t *testing.T) {
	pc := &contractsfake.PresenceController{}
	logger := &contractsfake.Logger{}

	err := message.NewChatPresenceUseCase(pc, &contractsfake.JIDResolver{}, logger).Execute(context.Background(), userID,
		domain.ChatPresenceRequest{Phone: "5511987654321", State: "composing", Media: "audio"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(pc.SendChatPresenceCalls); n != 1 {
		t.Fatalf("SendChatPresence chamado %d vez(es), esperava 1", n)
	}
	call := pc.SendChatPresenceCalls[0]
	if call.TxtID != userID || call.Chat != domain.JID("5511987654321") ||
		call.State != "composing" || call.Media != "audio" {
		t.Errorf("argumentos repassados errados: %+v", call)
	}
}

// --- SendPresence ------------------------------------------------------

func TestSendPresence_SessionFailurePropagates(t *testing.T) {
	pc := &contractsfake.PresenceController{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	err := message.NewSendPresenceUseCase(pc, logger).Execute(context.Background(), userID,
		domain.SendPresenceRequest{Type: "available"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(pc.SendPresenceCalls); n != 0 {
		t.Errorf("sem sessao, mas a presenca foi enviada %d vez(es)", n)
	}
	assertSessionLog(t, logger, "user_id", userID)
}

func TestSendPresence_TypeMapping(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		want    domain.PresenceType
		wantErr bool
	}{
		{"available", "available", domain.PresenceAvailable, false},
		{"unavailable", "unavailable", domain.PresenceUnavailable, false},
		{"vazio e' invalido", "", "", true},
		{"desconhecido e' invalido", "online", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &contractsfake.PresenceController{}
			logger := &contractsfake.Logger{}

			err := message.NewSendPresenceUseCase(pc, logger).Execute(context.Background(), userID,
				domain.SendPresenceRequest{Type: tt.typ})

			if tt.wantErr {
				if err == nil {
					t.Fatalf("tipo %q foi aceito", tt.typ)
				}
				if n := len(pc.SendPresenceCalls); n != 0 {
					t.Errorf("tipo invalido alcancou a porta %d vez(es)", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("tipo %q recusado: %v", tt.typ, err)
			}
			if n := len(pc.SendPresenceCalls); n != 1 {
				t.Fatalf("SendPresence chamado %d vez(es), esperava 1", n)
			}
			if got := pc.SendPresenceCalls[0].Presence; got != tt.want {
				t.Errorf("presenca traduzida: got %q, want %q", got, tt.want)
			}
			requireLog(t, logger, contractsfake.LevelInfo, "Setting presence")
		})
	}
}

func TestSendPresence_DownstreamFailureIsLoggedWithCause(t *testing.T) {
	pc := &contractsfake.PresenceController{
		SendPresenceFunc: func(context.Context, string, domain.PresenceType) error { return errDownstream },
	}
	logger := &contractsfake.Logger{}

	err := message.NewSendPresenceUseCase(pc, logger).Execute(context.Background(), userID,
		domain.SendPresenceRequest{Type: "available"})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	rec := requireLog(t, logger, contractsfake.LevelError, "Failed to send presence")
	if got, _ := rec.Keyval("error"); got != error(errDownstream) {
		t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
	}
}

// --- SubscribePresence -------------------------------------------------

func TestSubscribePresence_SessionFailurePropagates(t *testing.T) {
	pc := &contractsfake.PresenceController{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	err := message.NewSubscribePresenceUseCase(pc, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SubscribePresenceRequest{Phone: "5511987654321"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(pc.SubscribePresenceCalls); n != 0 {
		t.Errorf("sem sessao, mas houve subscribe %d vez(es)", n)
	}
	assertSessionLog(t, logger, "user_id", userID)
}

func TestSubscribePresence_RejectsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		req  domain.SubscribePresenceRequest
		jr   *contractsfake.JIDResolver
	}{
		{"Phone ausente", domain.SubscribePresenceRequest{}, &contractsfake.JIDResolver{}},
		{"Phone nao resolve", domain.SubscribePresenceRequest{Phone: "nao-e-telefone"}, failJID()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &contractsfake.PresenceController{}
			logger := &contractsfake.Logger{}

			err := message.NewSubscribePresenceUseCase(pc, tt.jr, logger).
				Execute(context.Background(), userID, tt.req)

			if err == nil {
				t.Fatal("request invalido foi aceito")
			}
			if n := len(pc.SubscribePresenceCalls); n != 0 {
				t.Errorf("request invalido alcancou a porta %d vez(es)", n)
			}
		})
	}
}

func TestSubscribePresence_DownstreamFailureIsLoggedWithCause(t *testing.T) {
	pc := &contractsfake.PresenceController{
		SubscribePresenceFunc: func(context.Context, string, domain.JID) error { return errDownstream },
	}
	logger := &contractsfake.Logger{}

	err := message.NewSubscribePresenceUseCase(pc, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SubscribePresenceRequest{Phone: "5511987654321"})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	rec := requireLog(t, logger, contractsfake.LevelError, "Failed to subscribe to presence")
	if got, _ := rec.Keyval("error"); got != error(errDownstream) {
		t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
	}
}

func TestSubscribePresence_Success(t *testing.T) {
	pc := &contractsfake.PresenceController{}
	logger := &contractsfake.Logger{}

	err := message.NewSubscribePresenceUseCase(pc, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SubscribePresenceRequest{Phone: "5511987654321"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(pc.SubscribePresenceCalls); n != 1 {
		t.Fatalf("SubscribePresence chamado %d vez(es), esperava 1", n)
	}
	if got := pc.SubscribePresenceCalls[0].Target; got != domain.JID("5511987654321") {
		t.Errorf("alvo do subscribe: got %q", got)
	}
	rec := requireLog(t, logger, contractsfake.LevelInfo, "Subscribed to presence")
	if got, ok := rec.Keyval("jid"); !ok || got != "5511987654321" {
		t.Errorf("log de sucesso nao carrega o jid: %v", rec.Keyvals)
	}
}
