package profile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain"
)

// panicDataAccess entra em pânico dentro de PushName para exercitar o recover
// de buildProfile — antes da Fase 4b esse pânico era engolido sem log algum
// (erros/F7). O fake compartilhado não tem um PushNameFunc porque PushName é
// leitura pura; o pânico é o único motivo para envolvê-lo.
type panicDataAccess struct {
	contractsfake.ProfileDataAccess
}

func (m *panicDataAccess) PushName() string { panic("boom") }

// fullAccess é o ProfileDataAccess do caminho feliz.
func fullAccess() *contractsfake.ProfileDataAccess {
	return &contractsfake.ProfileDataAccess{
		PushNameValue: "John",
		OwnJIDValue:   domain.JID("5511987654321@s.whatsapp.net"),
		OwnJIDOK:      true,
		ProfilePictureURLFunc: func(context.Context, domain.JID) (string, string, error) {
			return "https://img.example.com/1.jpg", "img-1", nil
		},
		ContactInfoFunc: func(context.Context, domain.JID) (string, string, error) {
			return "John Full", "Biz", nil
		},
	}
}

func TestGetProfileExecute(t *testing.T) {
	errNoSession := errors.New("connection refused")

	tests := []struct {
		name       string
		access     port.ProfileDataAccess
		accessErr  error
		wantErr    error
		wantFields map[string]string
		// wantPicCalls/wantContactCalls são as chamadas esperadas ao
		// ProfileDataAccess: sem JID próprio, nenhuma delas deve acontecer.
		wantPicCalls     int
		wantContactCalls int
		wantWarnLog      string
	}{
		{
			name:      "sem sessao devolve ErrNoSession e loga warn",
			accessErr: errNoSession,
			// O use case NÃO propaga a causa do provider: traduz para o erro
			// tipado do pacote, que é o que o handler mapeia para 4xx. A
			// causa fica no log, não no valor de retorno.
			wantErr:     profile.ErrNoSession,
			wantWarnLog: "no session for txtID",
		},
		{
			name:   "perfil completo",
			access: fullAccess(),
			wantFields: map[string]string{
				"pushname":      "John",
				"jid":           "5511987654321@s.whatsapp.net",
				"avatar_url":    "https://img.example.com/1.jpg",
				"avatar_id":     "img-1",
				"full_name":     "John Full",
				"business_name": "Biz",
			},
			wantPicCalls:     1,
			wantContactCalls: 1,
		},
		{
			name: "sem JID proprio nao consulta avatar nem contato",
			access: &contractsfake.ProfileDataAccess{
				PushNameValue: "John",
				OwnJIDOK:      false,
			},
			wantFields: map[string]string{
				"pushname":      "John",
				"jid":           "",
				"avatar_url":    "",
				"avatar_id":     "",
				"full_name":     "",
				"business_name": "",
			},
			wantPicCalls:     0,
			wantContactCalls: 0,
		},
		{
			name: "avatar indisponivel nao derruba o resto do perfil",
			access: &contractsfake.ProfileDataAccess{
				PushNameValue: "John",
				OwnJIDValue:   domain.JID("5511987654321@s.whatsapp.net"),
				OwnJIDOK:      true,
				ProfilePictureURLFunc: func(context.Context, domain.JID) (string, string, error) {
					return "", "", errors.New("sem foto")
				},
				ContactInfoFunc: func(context.Context, domain.JID) (string, string, error) {
					return "John Full", "Biz", nil
				},
			},
			wantFields: map[string]string{
				"pushname":      "John",
				"jid":           "5511987654321@s.whatsapp.net",
				"avatar_url":    "",
				"avatar_id":     "",
				"full_name":     "John Full",
				"business_name": "Biz",
			},
			wantPicCalls:     1,
			wantContactCalls: 1,
		},
		{
			name: "contato indisponivel nao derruba o avatar",
			access: &contractsfake.ProfileDataAccess{
				PushNameValue: "John",
				OwnJIDValue:   domain.JID("5511987654321@s.whatsapp.net"),
				OwnJIDOK:      true,
				ProfilePictureURLFunc: func(context.Context, domain.JID) (string, string, error) {
					return "https://img.example.com/1.jpg", "img-1", nil
				},
				ContactInfoFunc: func(context.Context, domain.JID) (string, string, error) {
					return "", "", errors.New("contato desconhecido")
				},
			},
			wantFields: map[string]string{
				"pushname":      "John",
				"jid":           "5511987654321@s.whatsapp.net",
				"avatar_url":    "https://img.example.com/1.jpg",
				"avatar_id":     "img-1",
				"full_name":     "",
				"business_name": "",
			},
			wantPicCalls:     1,
			wantContactCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &contractsfake.ProfileAccessProvider{Access: tt.access}
			if tt.accessErr != nil {
				provider.ProfileAccessFunc = func(context.Context, string) (port.ProfileDataAccess, error) {
					return nil, tt.accessErr
				}
			}
			logger := &contractsfake.Logger{}
			uc := profile.NewGetProfileUseCase(provider, logger)

			result, err := uc.Execute(context.Background(), "test-user")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("erro = %v, queria %v", err, tt.wantErr)
				}
				if result != "" {
					t.Errorf("resultado = %q, queria vazio no caminho de erro", result)
				}
				rec, ok := logger.FindLevel("warn", tt.wantWarnLog)
				if !ok {
					t.Fatalf("faltou log warn %q; registros: %v", tt.wantWarnLog, logger.Messages())
				}
				if !rec.HasKey("error") {
					t.Error("log de sessao ausente sem a keyval \"error\" — a causa se perde")
				}
				return
			}

			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}

			// O corpo de GET /session/profile é esta string, escrita direto no
			// ResponseWriter por ProfileHandler. Então as chaves abaixo SÃO o
			// contrato público da rota, e não um detalhe de serialização.
			//
			// Estas asserções vieram de pkg/domain/profile_test.go, deletado
			// nesta fase: lá elas testavam as tags de domain.Profile, um tipo
			// que NENHUM código de produção constrói. Apontadas para
			// ProfileResult, que é o que de fato vai para o cliente, elas
			// passam a valer alguma coisa. A asserção que estava aqui antes
			// (len(result) >= 10) não distinguia o perfil correto de
			// "{\"a\":1234}".
			var got map[string]string
			if err := json.Unmarshal([]byte(result), &got); err != nil {
				t.Fatalf("resultado nao e' um objeto JSON de strings: %v (%q)", err, result)
			}
			for key, wantVal := range tt.wantFields {
				gotVal, present := got[key]
				if !present {
					t.Errorf("chave %q sumiu do corpo da rota (contrato publico): %s", key, result)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("%s: got %q, want %q", key, gotVal, wantVal)
				}
			}
			for key := range got {
				if _, esperada := tt.wantFields[key]; !esperada {
					t.Errorf("chave inesperada %q no corpo da rota: %s", key, result)
				}
			}

			if len(logger.ByLevel("error")) != 0 {
				t.Errorf("logs de erro inesperados no caminho feliz: %v", logger.Messages())
			}

			da, ok := tt.access.(*contractsfake.ProfileDataAccess)
			if !ok {
				return
			}
			if len(da.ProfilePictureURLCalls) != tt.wantPicCalls {
				t.Errorf("ProfilePictureURL chamado %d vezes, queria %d", len(da.ProfilePictureURLCalls), tt.wantPicCalls)
			}
			if len(da.ContactInfoCalls) != tt.wantContactCalls {
				t.Errorf("ContactInfo chamado %d vezes, queria %d", len(da.ContactInfoCalls), tt.wantContactCalls)
			}
		})
	}
}

func TestGetProfileExecute_RecoversFromPanicAndLogs(t *testing.T) {
	provider := &contractsfake.ProfileAccessProvider{Access: &panicDataAccess{}}
	logger := &contractsfake.Logger{}
	uc := profile.NewGetProfileUseCase(provider, logger)

	result, err := uc.Execute(context.Background(), "test-user")
	if err != nil {
		t.Fatalf("panico dentro de buildProfile devia ser recuperado, nao virar erro: %v", err)
	}
	if result == "" {
		t.Fatalf("esperava JSON mesmo com perfil parcialmente construido, got %q", result)
	}
	errLogs := logger.ByLevel("error")
	if len(errLogs) != 1 {
		t.Fatalf("esperava exatamente 1 log de erro para o panico recuperado, got %d: %v", len(errLogs), logger.Messages())
	}
	if !strings.Contains(errLogs[0].Msg, "panic") {
		t.Errorf("log de erro %q devia mencionar o panico", errLogs[0].Msg)
	}
	if !errLogs[0].HasKey("panic") {
		t.Errorf("log do panico sem a keyval \"panic\": %+v", errLogs[0])
	}
}

func TestGetProfileNoPIIInLogs(t *testing.T) {
	provider := &contractsfake.ProfileAccessProvider{
		Access: &contractsfake.ProfileDataAccess{PushNameValue: "John"},
	}
	logger := &contractsfake.Logger{}
	uc := profile.NewGetProfileUseCase(provider, logger)
	_, _ = uc.Execute(context.Background(), "test-user")
	for _, msg := range logger.Messages() {
		if strings.Contains(msg, "5511") || strings.Contains(msg, "John") {
			t.Errorf("logger contem PII: %s", msg)
		}
	}
}

func TestProfileError(t *testing.T) {
	tests := []struct {
		name string
		err  *profile.ProfileError
		want string
	}{
		{name: "ErrNoSession", err: profile.ErrNoSession, want: "no session"},
		{name: "mensagem propria", err: profile.NewProfileError("perfil indisponivel"), want: "perfil indisponivel"},
		{name: "mensagem vazia", err: profile.NewProfileError(""), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, queria %q", got, tt.want)
			}
		})
	}

	// Dois ProfileError com a mesma mensagem são valores distintos: o
	// handler compara com errors.Is contra ErrNoSession, não por texto.
	if errors.Is(profile.NewProfileError("no session"), profile.ErrNoSession) {
		t.Error("ProfileError nao deve casar com ErrNoSession so por ter a mesma mensagem")
	}
}
