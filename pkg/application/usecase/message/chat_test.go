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

// --- MarkRead ----------------------------------------------------------

func TestMarkRead_SessionFailurePropagates(t *testing.T) {
	cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	err := message.NewMarkReadUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.MarkReadRequest{ChatPhone: "5511987654321", Id: []string{"A"}})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(cm.MarkReadCalls); n != 0 {
		t.Errorf("sem sessao, mas houve MarkRead %d vez(es)", n)
	}
	assertSessionLog(t, logger, "user_id", userID)
}

func TestMarkRead_RejectsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		req  domain.MarkReadRequest
		jr   *contractsfake.JIDResolver
	}{
		{
			"nem ChatPhone nem Chat",
			domain.MarkReadRequest{Id: []string{"A"}},
			&contractsfake.JIDResolver{},
		},
		{
			"ChatPhone nao resolve",
			domain.MarkReadRequest{ChatPhone: "nao-e-telefone", Id: []string{"A"}},
			failJID(),
		},
		{
			// failJID() nao serve aqui: recusaria ja o ChatPhone, e o teste
			// sairia pelo caminho anterior sem tocar o do SenderPhone.
			"SenderPhone nao resolve",
			domain.MarkReadRequest{ChatPhone: "5511987654321", SenderPhone: badPhone, Id: []string{"A"}},
			rejectOnly(),
		},
		{
			"Id vazio",
			domain.MarkReadRequest{ChatPhone: "5511987654321"},
			&contractsfake.JIDResolver{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			logger := &contractsfake.Logger{}

			err := message.NewMarkReadUseCase(cm, tt.jr, logger).Execute(context.Background(), userID, tt.req)

			if err == nil {
				t.Fatal("request invalido foi aceito")
			}
			if n := len(cm.MarkReadCalls); n != 0 {
				t.Errorf("request invalido alcancou a porta %d vez(es)", n)
			}
		})
	}
}

// TestMarkRead_LegacyFieldsResolveToEmptyJID trava o comportamento dos
// campos legados Chat/Sender: eles satisfazem a exigência de "algum chat foi
// informado" mas produzem JID vazio, porque o parsing legado nunca foi
// portado. É um comportamento preservado do upstream, não um acidente — e
// enquanto for assim, precisa estar escrito.
func TestMarkRead_LegacyFieldsResolveToEmptyJID(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}

	err := message.NewMarkReadUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.MarkReadRequest{
			Chat:   "legado@s.whatsapp.net",
			Sender: "legado@s.whatsapp.net",
			Id:     []string{"A"},
		})

	if err != nil {
		t.Fatalf("campos legados foram recusados: %v", err)
	}
	if n := len(cm.MarkReadCalls); n != 1 {
		t.Fatalf("MarkRead chamado %d vez(es), esperava 1", n)
	}
	call := cm.MarkReadCalls[0]
	if call.Chat != "" || call.Sender != "" {
		t.Errorf("campo legado passou a resolver: chat=%q sender=%q", call.Chat, call.Sender)
	}
}

func TestMarkRead_DownstreamFailureIsLoggedWithCause(t *testing.T) {
	cm := &contractsfake.ChatMessenger{
		MarkReadFunc: func(context.Context, string, []string, time.Time, domain.JID, domain.JID) error {
			return errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	err := message.NewMarkReadUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.MarkReadRequest{ChatPhone: "5511987654321", Id: []string{"A"}})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	rec := requireLog(t, logger, contractsfake.LevelError, "Failed to mark messages as read")
	if got, _ := rec.Keyval("error"); got != error(errDownstream) {
		t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
	}
}

func TestMarkRead_Success(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	logger := &contractsfake.Logger{}

	err := message.NewMarkReadUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.MarkReadRequest{
			ChatPhone:   "5511987654321",
			SenderPhone: "5511900000000",
			Id:          []string{"A", "B"},
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(cm.MarkReadCalls); n != 1 {
		t.Fatalf("MarkRead chamado %d vez(es), esperava 1", n)
	}
	call := cm.MarkReadCalls[0]
	if call.Chat != domain.JID("5511987654321") || call.Sender != domain.JID("5511900000000") {
		t.Errorf("JIDs repassados errados: %+v", call)
	}
	if len(call.IDs) != 2 {
		t.Errorf("IDs repassados: got %v", call.IDs)
	}
	rec := requireLog(t, logger, contractsfake.LevelInfo, "Messages marked as read")
	if got, ok := rec.Keyval("count"); !ok || got != 2 {
		t.Errorf("log de sucesso nao carrega count=2: %v", rec.Keyvals)
	}
}

// --- React -------------------------------------------------------------

func TestReact_SessionFailurePropagates(t *testing.T) {
	cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewReactUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(cm.SendReactionCalls); n != 0 {
		t.Errorf("sem sessao, mas houve SendReaction %d vez(es)", n)
	}
	assertSessionLog(t, logger, "user_id", userID)
}

func TestReact_RejectsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		req  domain.ReactRequest
		jr   *contractsfake.JIDResolver
	}{
		{"Phone ausente", domain.ReactRequest{Body: "👍", Id: "A"}, &contractsfake.JIDResolver{}},
		{"Body ausente", domain.ReactRequest{Phone: "5511987654321", Id: "A"}, &contractsfake.JIDResolver{}},
		{"Id ausente", domain.ReactRequest{Phone: "5511987654321", Body: "👍"}, &contractsfake.JIDResolver{}},
		{"Phone nao resolve", domain.ReactRequest{Phone: "nao-e-telefone", Body: "👍", Id: "A"}, failJID()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewReactUseCase(cm, tt.jr, logger).Execute(context.Background(), userID, tt.req)

			if err == nil {
				t.Fatal("request invalido foi aceito")
			}
			if n := len(cm.SendReactionCalls); n != 0 {
				t.Errorf("request invalido alcancou a porta %d vez(es)", n)
			}
		})
	}
}

// TestReact_ReactionShape cobre as três traduções que o use case faz sobre o
// request antes de chamar a porta: o prefixo "me:", a palavra "remove" e a
// resolução do Participant.
func TestReact_ReactionShape(t *testing.T) {
	tests := []struct {
		name            string
		req             domain.ReactRequest
		jr              *contractsfake.JIDResolver
		wantTargetID    string
		wantFromMe      bool
		wantText        string
		wantParticipant domain.JID
	}{
		{
			name:         "reacao simples",
			req:          domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A"},
			jr:           &contractsfake.JIDResolver{},
			wantTargetID: "A",
			wantText:     "👍",
		},
		{
			name:         "prefixo me: marca fromMe e sai do id",
			req:          domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "me:A"},
			jr:           &contractsfake.JIDResolver{},
			wantTargetID: "A",
			wantFromMe:   true,
			wantText:     "👍",
		},
		{
			name:         "remove vira texto vazio",
			req:          domain.ReactRequest{Phone: "5511987654321", Body: "remove", Id: "A"},
			jr:           &contractsfake.JIDResolver{},
			wantTargetID: "A",
			wantText:     "",
		},
		{
			name:            "participant resolvido e' repassado",
			req:             domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A", Participant: "5511900000000"},
			jr:              &contractsfake.JIDResolver{},
			wantTargetID:    "A",
			wantText:        "👍",
			wantParticipant: domain.JID("5511900000000"),
		},
		{
			// Comportamento preservado do upstream: um Participant que não
			// resolve é ignorado em silêncio, não vira erro.
			name:         "participant que nao resolve e' ignorado",
			req:          domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A", Participant: badPhone},
			jr:           rejectOnly(),
			wantTargetID: "A",
			wantText:     "👍",
		},
		{
			// Com fromMe o Participant sequer é consultado.
			name:         "fromMe ignora o participant",
			req:          domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "me:A", Participant: "5511900000000"},
			jr:           &contractsfake.JIDResolver{},
			wantTargetID: "A",
			wantFromMe:   true,
			wantText:     "👍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			logger := &contractsfake.Logger{}

			out, err := message.NewReactUseCase(cm, tt.jr, logger).Execute(context.Background(), userID, tt.req)

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if n := len(cm.SendReactionCalls); n != 1 {
				t.Fatalf("SendReaction chamado %d vez(es), esperava 1", n)
			}
			got := cm.SendReactionCalls[0].Reaction
			if got.TargetMessageID != tt.wantTargetID {
				t.Errorf("TargetMessageID: got %q, want %q", got.TargetMessageID, tt.wantTargetID)
			}
			if got.FromMe != tt.wantFromMe {
				t.Errorf("FromMe: got %v, want %v", got.FromMe, tt.wantFromMe)
			}
			if got.Text != tt.wantText {
				t.Errorf("Text: got %q, want %q", got.Text, tt.wantText)
			}
			if got.Participant != tt.wantParticipant {
				t.Errorf("Participant: got %q, want %q", got.Participant, tt.wantParticipant)
			}
			if out["Id"] != tt.wantTargetID {
				t.Errorf("resposta.Id: got %v, want %q", out["Id"], tt.wantTargetID)
			}
			if out["Details"] != "Sent" {
				t.Errorf("resposta.Details: got %v", out["Details"])
			}
			requireLog(t, logger, contractsfake.LevelInfo, "Reaction sent")
		})
	}
}

// badPhone é a entrada que rejectOnly recusa. Existe como constante porque
// os use cases deste arquivo chamam ResolveJID mais de uma vez por Execute, e
// o teste precisa dizer QUAL das chamadas deve falhar.
const badPhone = "lixo"

// rejectOnly resolve tudo, menos badPhone. É o que distingue "o primeiro
// telefone não resolve" de "o segundo não resolve" — um resolvedor que
// recusa tudo sai sempre pelo primeiro caminho e deixa o segundo sem teste.
func rejectOnly() *contractsfake.JIDResolver {
	return &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == badPhone {
				return "", errJID
			}
			return domain.JID(raw), nil
		},
	}
}

func TestReact_TimestampComesFromThePort(t *testing.T) {
	at := time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC)
	cm := &contractsfake.ChatMessenger{
		SendReactionFunc: func(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{Timestamp: at}, nil
		},
	}
	logger := &contractsfake.Logger{}

	out, err := message.NewReactUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if out["Timestamp"] != at.Unix() {
		t.Errorf("Timestamp: got %v, want %v", out["Timestamp"], at.Unix())
	}
}

func TestReact_DownstreamFailureIsLoggedWithCause(t *testing.T) {
	cm := &contractsfake.ChatMessenger{
		SendReactionFunc: func(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	out, err := message.NewReactUseCase(cm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.ReactRequest{Phone: "5511987654321", Body: "👍", Id: "A"})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	if out != nil {
		t.Errorf("resultado devolvido junto com erro: %v", out)
	}
	rec := requireLog(t, logger, contractsfake.LevelError, "Error sending reaction")
	if got, _ := rec.Keyval("error"); got != error(errDownstream) {
		t.Errorf("log nao carrega a causa: %v", rec.Keyvals)
	}
}
