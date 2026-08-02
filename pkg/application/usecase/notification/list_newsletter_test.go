package notification_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
)

func TestListNewsletterExecute(t *testing.T) {
	errSession := errors.New("sessao nao conectada")
	errList := errors.New("timeout no SDK")
	payload := []map[string]string{{"id": "123@newsletter", "name": "Canal"}}

	tests := []struct {
		name       string
		sessionErr error
		listValue  any
		listErr    error

		wantErr error
		// wantNewsletter é o valor esperado em NewsletterCollection.Newsletter
		// no caminho feliz.
		wantNewsletter any
		wantErrorLog   string
		// wantListCalls é quantas vezes ListSubscribed deve ter sido chamado.
		wantListCalls int
	}{
		{
			name:       "sem sessao aborta antes de consultar o SDK",
			sessionErr: errSession,
			// A migração do fmt.Errorf("no session") tem de propagar a causa:
			// o handler acima distingue "sem sessão" de "SDK quebrado" por
			// ela, e a string opaca destruía essa distinção.
			wantErr:       errSession,
			wantErrorLog:  "no whatsmeow session",
			wantListCalls: 0,
		},
		{
			name:          "falha ao listar propaga e loga",
			listErr:       errList,
			wantErr:       errList,
			wantErrorLog:  "failed to get newsletter list",
			wantListCalls: 1,
		},
		{
			name:           "lista vazia devolve colecao com newsletter nil",
			listValue:      nil,
			wantNewsletter: nil,
			wantListCalls:  1,
		},
		{
			name:           "lista populada e devolvida sem transformacao",
			listValue:      payload,
			wantNewsletter: payload,
			wantListCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &contractsfake.NewsletterReader{
				ListSubscribedFunc: func(context.Context, string) (any, error) {
					return tt.listValue, tt.listErr
				},
			}
			reader.SessionGuard = contractsfake.FailSession(tt.sessionErr)
			logger := &contractsfake.Logger{}
			uc := notification.NewListNewsletterUseCase(reader, logger)

			got, err := uc.Execute(context.Background(), "user-42")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("erro = %v, queria envolver %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("colecao = %+v, queria nil no caminho de erro", got)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if got == nil {
					t.Fatal("colecao nil sem erro")
				}
				if !equalAny(got.Newsletter, tt.wantNewsletter) {
					t.Errorf("newsletter = %#v, queria %#v", got.Newsletter, tt.wantNewsletter)
				}
			}

			if len(reader.EnsureSessionCalls) != 1 {
				t.Errorf("EnsureSession chamado %d vezes, queria 1", len(reader.EnsureSessionCalls))
			} else if reader.EnsureSessionCalls[0].TxtID != "user-42" {
				t.Errorf("EnsureSession recebeu txtID %q, queria \"user-42\"", reader.EnsureSessionCalls[0].TxtID)
			}
			if len(reader.ListSubscribedCalls) != tt.wantListCalls {
				t.Errorf("ListSubscribed chamado %d vezes, queria %d", len(reader.ListSubscribedCalls), tt.wantListCalls)
			}

			if tt.wantErrorLog == "" {
				if len(logger.ByLevel("error")) != 0 {
					t.Errorf("logs de erro inesperados: %v", logger.Messages())
				}
				return
			}
			rec, ok := logger.FindLevel("error", tt.wantErrorLog)
			if !ok {
				t.Fatalf("faltou log de erro %q; registros: %v", tt.wantErrorLog, logger.Messages())
			}
			if !rec.HasKey("error") {
				t.Errorf("log %q sem a keyval \"error\" — a causa se perde", tt.wantErrorLog)
			}
			if gotID, ok := rec.Keyval("user_id"); !ok || gotID != "user-42" {
				t.Errorf("log %q com user_id = %v (presente=%v), queria \"user-42\"", tt.wantErrorLog, gotID, ok)
			}
			if !rec.IsStructured() {
				t.Errorf("log %q com keyvals desbalanceadas", tt.wantErrorLog)
			}
		})
	}
}

// equalAny compara os dois formatos que ListSubscribed devolve nos testes:
// nil e a fatia de mapas usada como payload. Evita reflect.DeepEqual para
// manter a comparação explícita sobre o que o use case realmente repassa.
func equalAny(got, want any) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	gotSlice, gok := got.([]map[string]string)
	wantSlice, wok := want.([]map[string]string)
	if !gok || !wok || len(gotSlice) != len(wantSlice) {
		return false
	}
	for i := range gotSlice {
		if len(gotSlice[i]) != len(wantSlice[i]) {
			return false
		}
		for k, v := range wantSlice[i] {
			if gotSlice[i][k] != v {
				return false
			}
		}
	}
	return true
}
