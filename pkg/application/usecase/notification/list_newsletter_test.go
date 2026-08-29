package notification_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"
)

func TestListNewsletterExecute(t *testing.T) {
	errSession := errors.New("sessao nao conectada")
	errList := errors.New("timeout no SDK")
	payload := []domain.NewsletterMetadata{{
		JID:   "123@newsletter",
		State: "active",
		Name:  domain.NewsletterText{Text: "Canal"},
	}}

	tests := []struct {
		name       string
		sessionErr error
		listValue  []domain.NewsletterMetadata
		listErr    error

		wantErr error
		// wantNewsletters é o valor esperado em
		// NewsletterCollection.Newsletters no caminho feliz.
		wantNewsletters []domain.NewsletterMetadata
		wantErrorLog    string
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
			wantErrorLog:  "no wanoise session",
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
			name:            "lista vazia devolve colecao vazia",
			listValue:       []domain.NewsletterMetadata{},
			wantNewsletters: []domain.NewsletterMetadata{},
			wantListCalls:   1,
		},
		{
			name:            "lista populada e devolvida sem transformacao",
			listValue:       payload,
			wantNewsletters: payload,
			wantListCalls:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &contractsfake.NewsletterReader{
				ListSubscribedFunc: func(context.Context, string) ([]domain.NewsletterMetadata, error) {
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
				if !equalNewsletters(got.Newsletters, tt.wantNewsletters) {
					t.Errorf("newsletters = %#v, queria %#v", got.Newsletters, tt.wantNewsletters)
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
			// "no wanoise session" e' o unico caso que sai em warn: sessao
			// nao conectada e' estado esperado, nao erro de servidor (F72).
			nivel := "error"
			if tt.wantErrorLog == "no wanoise session" {
				nivel = "warn"
			}
			rec, ok := logger.FindLevel(nivel, tt.wantErrorLog)
			if !ok {
				t.Fatalf("faltou log %s %q; registros: %v", nivel, tt.wantErrorLog, logger.Messages())
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

// equalNewsletters compara duas listagens campo a campo.
//
// Explícito em vez de reflect.DeepEqual para que o que o use case promete
// repassar seja legível aqui: identificador, estado e nome, e não "os structs
// são iguais em toda a profundidade" — que passaria a valer para campos que o
// use case não toca.
func equalNewsletters(got, want []domain.NewsletterMetadata) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].JID != want[i].JID ||
			got[i].State != want[i].State ||
			got[i].Name.Text != want[i].Name.Text {
			return false
		}
	}
	return true
}
