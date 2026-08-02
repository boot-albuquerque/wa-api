// chat_test.go — os 3 use cases avulsos sobre uma conversa.
//
// O que se assere: o erro da porta chega intacto ao chamador (errors.Is
// contra a sentinela injetada, nunca comparação de texto), quais chamadas de
// porta aconteceram — e, tão importante, quais NÃO aconteceram quando a
// validação recusa antes —, e o log do caminho de saída.
//
// Os três abrem com EnsureSession e, desde a F11, propagam a causa em vez de
// traduzi-la para um fmt.Errorf de texto fixo. errors.Is é a trava disso.
package chat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
)

var (
	errNoSession = errors.New("porta: sem sessao whatsmeow")
	errPorta     = errors.New("porta: falha do downstream")
	errJID       = errors.New("resolver: JID invalido")
)

const userID = "user-1"

// --- Guarda de sessão, comum aos três -----------------------------------

func TestUseCases_SemSessao_PropagamACausa(t *testing.T) {
	cases := []struct {
		name string
		run  func(co *contractsfake.ChatOperations, jr *contractsfake.JIDResolver, log *contractsfake.Logger) (any, error)
	}{
		{"ArchiveChat", func(co *contractsfake.ChatOperations, jr *contractsfake.JIDResolver, log *contractsfake.Logger) (any, error) {
			return chat.NewArchiveChatUseCase(co, jr, log).
				Execute(context.Background(), userID, domain.ArchiveChatRequest{Jid: "5511@s.whatsapp.net"})
		}},
		{"RejectCall", func(co *contractsfake.ChatOperations, jr *contractsfake.JIDResolver, log *contractsfake.Logger) (any, error) {
			return chat.NewRejectCallUseCase(co, jr, log).
				Execute(context.Background(), userID, domain.RejectCallRequest{CallFrom: "5511@s.whatsapp.net", CallID: "c1"})
		}},
		{"RequestUnavailableMessage", func(co *contractsfake.ChatOperations, jr *contractsfake.JIDResolver, log *contractsfake.Logger) (any, error) {
			return chat.NewRequestUnavailableMessageUseCase(co, jr, log).
				Execute(context.Background(), userID, domain.RequestUnavailableMessageRequest{Chat: "c@g.us", Sender: "s@s.whatsapp.net", ID: "m1"})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			co := &contractsfake.ChatOperations{SessionGuard: contractsfake.FailSession(errNoSession)}
			jr := &contractsfake.JIDResolver{}
			log := &contractsfake.Logger{}

			_, err := tc.run(co, jr, log)

			if !errors.Is(err, errNoSession) {
				t.Fatalf("a causa da porta se perdeu: %v — a traducao fmt.Errorf(\"no session\") voltou?", err)
			}
			if len(jr.ResolveQualifiedJIDCalls) != 0 {
				t.Error("sessao recusada nao devia chegar ate' o resolver de JID")
			}

			rec, found := log.FindLevel(contractsfake.LevelError, "no whatsmeow session")
			if !found {
				t.Fatalf("recusa de sessao nao foi logada em nivel error: %v", log.Messages())
			}
			if !rec.IsStructured() {
				t.Errorf("registro nao e' estruturado: %v", rec.Keyvals)
			}
			if v, ok := rec.Keyval("user_id"); !ok || v != userID {
				t.Errorf(`Keyval("user_id") = %v, %v; quero %q`, v, ok, userID)
			}
			if v, ok := rec.Keyval("error"); !ok || !errors.Is(v.(error), errNoSession) {
				t.Errorf(`Keyval("error") = %v, %v; quero a causa da porta`, v, ok)
			}
		})
	}
}

// --- ArchiveChat --------------------------------------------------------

func TestArchiveChat(t *testing.T) {
	t.Run("jid ausente recusa antes do resolver", func(t *testing.T) {
		co, jr, log := &contractsfake.ChatOperations{}, &contractsfake.JIDResolver{}, &contractsfake.Logger{}

		r, err := chat.NewArchiveChatUseCase(co, jr, log).
			Execute(context.Background(), userID, domain.ArchiveChatRequest{})

		if err == nil {
			t.Fatal("jid vazio devia ser recusado")
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if len(jr.ResolveQualifiedJIDCalls) != 0 || len(co.ArchiveChatCalls) != 0 {
			t.Error("recusa por payload nao devia tocar resolver nem porta")
		}
	})

	t.Run("jid irresolvivel recusa antes de arquivar", func(t *testing.T) {
		co := &contractsfake.ChatOperations{}
		jr := &contractsfake.JIDResolver{ResolveQualifiedJIDFunc: func(context.Context, string) (domain.JID, error) {
			return "", errJID
		}}

		r, err := chat.NewArchiveChatUseCase(co, jr, &contractsfake.Logger{}).
			Execute(context.Background(), userID, domain.ArchiveChatRequest{Jid: "lixo"})

		if err == nil {
			t.Fatal("JID irresolvivel devia ser recusado")
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if len(co.ArchiveChatCalls) != 0 {
			t.Error("ArchiveChat nao devia ser chamada com JID invalido")
		}
	})

	t.Run("falha da porta e' logada e embrulhada com a causa", func(t *testing.T) {
		co := &contractsfake.ChatOperations{ArchiveChatFunc: func(context.Context, string, domain.JID, bool) error {
			return errPorta
		}}
		log := &contractsfake.Logger{}

		r, err := chat.NewArchiveChatUseCase(co, &contractsfake.JIDResolver{}, log).
			Execute(context.Background(), userID, domain.ArchiveChatRequest{Jid: "c@g.us", Archive: true})

		if !errors.Is(err, errPorta) {
			t.Fatalf("a causa da porta se perdeu: %v", err)
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("failed to archive chat") {
			t.Errorf("falha nao foi logada: %v", log.Messages())
		}
	})

	t.Run("arquivar e desarquivar produzem mensagens distintas", func(t *testing.T) {
		cases := []struct {
			archive bool
			want    string
		}{
			{true, "Chat archived"},
			{false, "Chat unarchived"},
		}
		for _, tc := range cases {
			co := &contractsfake.ChatOperations{}
			r, err := chat.NewArchiveChatUseCase(co, &contractsfake.JIDResolver{}, &contractsfake.Logger{}).
				Execute(context.Background(), userID, domain.ArchiveChatRequest{Jid: "c@g.us", Archive: tc.archive})

			if err != nil {
				t.Fatalf("archive=%v: erro inesperado: %v", tc.archive, err)
			}
			if !r.Success {
				t.Errorf("archive=%v: Success = false", tc.archive)
			}
			if r.Message != tc.want {
				t.Errorf("archive=%v: Message = %q, quero %q", tc.archive, r.Message, tc.want)
			}
			if len(co.ArchiveChatCalls) != 1 {
				t.Fatalf("archive=%v: ArchiveChat chamada %d vez(es)", tc.archive, len(co.ArchiveChatCalls))
			}
			call := co.ArchiveChatCalls[0]
			if call.TxtID != userID || call.Chat != domain.JID("c@g.us") || call.Archive != tc.archive {
				t.Errorf("archive=%v: ArchiveChat recebeu %+v", tc.archive, call)
			}
		}
	})
}

// --- RejectCall ---------------------------------------------------------

func TestRejectCall(t *testing.T) {
	t.Run("campos obrigatorios recusam antes do resolver", func(t *testing.T) {
		cases := []struct {
			name string
			req  domain.RejectCallRequest
		}{
			{"sem call_from", domain.RejectCallRequest{CallID: "c1"}},
			{"sem call_id", domain.RejectCallRequest{CallFrom: "5511@s.whatsapp.net"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				co, jr := &contractsfake.ChatOperations{}, &contractsfake.JIDResolver{}

				r, err := chat.NewRejectCallUseCase(co, jr, &contractsfake.Logger{}).
					Execute(context.Background(), userID, tc.req)

				if err == nil {
					t.Fatal("payload incompleto devia ser recusado")
				}
				if r != nil {
					t.Error("resultado devia ser nil")
				}
				if len(jr.ResolveQualifiedJIDCalls) != 0 || len(co.RejectCallCalls) != 0 {
					t.Error("recusa por payload nao devia tocar resolver nem porta")
				}
			})
		}
	})

	t.Run("call_from irresolvivel recusa antes de rejeitar", func(t *testing.T) {
		co := &contractsfake.ChatOperations{}
		jr := &contractsfake.JIDResolver{ResolveQualifiedJIDFunc: func(context.Context, string) (domain.JID, error) {
			return "", errJID
		}}

		r, err := chat.NewRejectCallUseCase(co, jr, &contractsfake.Logger{}).
			Execute(context.Background(), userID, domain.RejectCallRequest{CallFrom: "lixo", CallID: "c1"})

		if err == nil {
			t.Fatal("call_from irresolvivel devia ser recusado")
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if len(co.RejectCallCalls) != 0 {
			t.Error("RejectCall nao devia ser chamada")
		}
	})

	t.Run("falha da porta e' logada e embrulhada com a causa", func(t *testing.T) {
		co := &contractsfake.ChatOperations{RejectCallFunc: func(context.Context, string, domain.JID, string) error {
			return errPorta
		}}
		log := &contractsfake.Logger{}

		r, err := chat.NewRejectCallUseCase(co, &contractsfake.JIDResolver{}, log).
			Execute(context.Background(), userID, domain.RejectCallRequest{CallFrom: "5511@s.whatsapp.net", CallID: "c1"})

		if !errors.Is(err, errPorta) {
			t.Fatalf("a causa da porta se perdeu: %v", err)
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("failed to reject call") {
			t.Errorf("falha nao foi logada: %v", log.Messages())
		}
	})

	t.Run("caminho feliz repassa call_from resolvido e call_id", func(t *testing.T) {
		co := &contractsfake.ChatOperations{}
		jr := &contractsfake.JIDResolver{ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			return domain.JID(raw + "@s.whatsapp.net"), nil
		}}
		log := &contractsfake.Logger{}

		r, err := chat.NewRejectCallUseCase(co, jr, log).
			Execute(context.Background(), userID, domain.RejectCallRequest{CallFrom: "5511", CallID: "c1"})

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.CallID != "c1" || r.Details == "" {
			t.Errorf("resultado = %+v", r)
		}
		if len(co.RejectCallCalls) != 1 {
			t.Fatalf("RejectCall chamada %d vez(es), quero 1", len(co.RejectCallCalls))
		}
		call := co.RejectCallCalls[0]
		if call.From != domain.JID("5511@s.whatsapp.net") || call.CallID != "c1" || call.TxtID != userID {
			t.Errorf("RejectCall recebeu %+v", call)
		}
		if _, ok := log.FindLevel(contractsfake.LevelInfo, "Call rejected"); !ok {
			t.Errorf("sucesso nao foi logado: %v", log.Messages())
		}
	})
}

// --- RequestUnavailableMessage ------------------------------------------

func TestRequestUnavailableMessage(t *testing.T) {
	completo := domain.RequestUnavailableMessageRequest{Chat: "c@g.us", Sender: "s@s.whatsapp.net", ID: "m1"}

	t.Run("campos obrigatorios recusam antes do resolver", func(t *testing.T) {
		cases := []struct {
			name  string
			mutar func(r *domain.RequestUnavailableMessageRequest)
		}{
			{"sem Chat", func(r *domain.RequestUnavailableMessageRequest) { r.Chat = "" }},
			{"sem Sender", func(r *domain.RequestUnavailableMessageRequest) { r.Sender = "" }},
			{"sem ID", func(r *domain.RequestUnavailableMessageRequest) { r.ID = "" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := completo
				tc.mutar(&req)
				co, jr := &contractsfake.ChatOperations{}, &contractsfake.JIDResolver{}

				r, err := chat.NewRequestUnavailableMessageUseCase(co, jr, &contractsfake.Logger{}).
					Execute(context.Background(), userID, req)

				if err == nil {
					t.Fatal("payload incompleto devia ser recusado")
				}
				if r != nil {
					t.Error("resultado devia ser nil")
				}
				if len(jr.ResolveQualifiedJIDCalls) != 0 || len(co.RequestUnavailableMessageCalls) != 0 {
					t.Error("recusa por payload nao devia tocar resolver nem porta")
				}
			})
		}
	})

	// Chat e Sender são resolvidos em sequência; o teste separa os dois para
	// que a falha do segundo não seja confundida com a do primeiro.
	t.Run("JID irresolvivel recusa, e o teste distingue Chat de Sender", func(t *testing.T) {
		cases := []struct {
			name      string
			falharEm  string
			wantCalls int
		}{
			{"Chat invalido", "c@g.us", 1},
			{"Sender invalido", "s@s.whatsapp.net", 2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				co := &contractsfake.ChatOperations{}
				jr := &contractsfake.JIDResolver{ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
					if raw == tc.falharEm {
						return "", errJID
					}
					return domain.JID(raw), nil
				}}

				r, err := chat.NewRequestUnavailableMessageUseCase(co, jr, &contractsfake.Logger{}).
					Execute(context.Background(), userID, completo)

				if err == nil {
					t.Fatal("JID irresolvivel devia ser recusado")
				}
				if r != nil {
					t.Error("resultado devia ser nil")
				}
				if len(jr.ResolveQualifiedJIDCalls) != tc.wantCalls {
					t.Errorf("resolver chamado %d vez(es), quero %d — a ordem Chat->Sender mudou?",
						len(jr.ResolveQualifiedJIDCalls), tc.wantCalls)
				}
				if len(co.RequestUnavailableMessageCalls) != 0 {
					t.Error("porta nao devia ser chamada com JID invalido")
				}
			})
		}
	})

	t.Run("falha da porta e' logada e embrulhada com a causa", func(t *testing.T) {
		co := &contractsfake.ChatOperations{
			RequestUnavailableMessageFunc: func(context.Context, string, domain.JID, domain.JID, string) (domain.UnavailableMessageAck, error) {
				return domain.UnavailableMessageAck{}, errPorta
			},
		}
		log := &contractsfake.Logger{}

		r, err := chat.NewRequestUnavailableMessageUseCase(co, &contractsfake.JIDResolver{}, log).
			Execute(context.Background(), userID, completo)

		if !errors.Is(err, errPorta) {
			t.Fatalf("a causa da porta se perdeu: %v", err)
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("failed to send unavailable message request") {
			t.Errorf("falha nao foi logada: %v", log.Messages())
		}
	})

	t.Run("caminho feliz devolve o ack da porta e ecoa o pedido", func(t *testing.T) {
		at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
		co := &contractsfake.ChatOperations{
			RequestUnavailableMessageFunc: func(context.Context, string, domain.JID, domain.JID, string) (domain.UnavailableMessageAck, error) {
				return domain.UnavailableMessageAck{RequestID: "req-9", Timestamp: at}, nil
			},
		}

		r, err := chat.NewRequestUnavailableMessageUseCase(co, &contractsfake.JIDResolver{}, &contractsfake.Logger{}).
			Execute(context.Background(), userID, completo)

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !r.Success || r.RequestID != "req-9" || r.Timestamp != at.Unix() {
			t.Errorf("ack nao chegou ao resultado: %+v", r)
		}
		if r.Chat != completo.Chat || r.Sender != completo.Sender || r.MessageID != completo.ID {
			t.Errorf("resultado nao ecoa o pedido: %+v", r)
		}
		if len(co.RequestUnavailableMessageCalls) != 1 {
			t.Fatalf("porta chamada %d vez(es), quero 1", len(co.RequestUnavailableMessageCalls))
		}
		call := co.RequestUnavailableMessageCalls[0]
		if call.Chat != domain.JID(completo.Chat) || call.Sender != domain.JID(completo.Sender) || call.MessageID != completo.ID {
			t.Errorf("porta recebeu %+v", call)
		}
	})
}
