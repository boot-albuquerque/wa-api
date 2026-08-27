package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// errNoSession é o erro que o guarda de sessão devolve nos testes; o contrato
// depois da migração é que ele suba intacto, não embrulhado numa string.
var errNoSession = errors.New("session not found")

// assertNoSessionLog verifica a forma do log padronizado de sessão ausente.
// assertNoSessionLog exige o log em WARN, nao em Error (F72).
//
// "sessao nao conectada" e' estado ESPERADO — de toda sessao que ainda nao
// pareou ou que foi desconectada de proposito —, e logar como Error inutiliza
// alerta por nivel: um painel fazendo poll de status a cada 3s gera 20 linhas
// de error por minuto e por sessao parada.
//
// Warn, e nao Info/Debug, porque a metrica de log-coverage do projeto so
// conta um caminho de saida como coberto com nivel >= Warn (METRIC.md:136).
// Rebaixar mais tornaria 64 caminhos descobertos de uma vez e obrigaria a
// afrouxar a catraca — trocar um problema por outro.
func assertNoSessionLog(t *testing.T, logger *contractsfake.Logger, userID string) {
	t.Helper()
	rec, ok := logger.FindLevel(contractsfake.LevelWarn, "no wanoise session")
	if !ok {
		t.Fatalf("log de sessão ausente não emitido; houve %v", logger.Messages())
	}
	if !rec.IsStructured() {
		t.Errorf("keyvals = %v, queria pares estruturados", rec.Keyvals)
	}
	if v, ok := rec.Keyval("user_id"); !ok || v != userID {
		t.Errorf("keyval user_id = %v, queria %q", v, userID)
	}
	if _, ok := rec.Keyval("error"); !ok {
		t.Error("keyval error ausente")
	}
}

func TestCheckUserUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name      string
		session   error
		checkFunc func(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error)
		wantIs    error
		wantLen   int
	}{
		{
			name:    "sem sessão o erro original sobe",
			session: errNoSession,
			wantIs:  errNoSession,
		},
		{
			name: "falha da consulta é embrulhada",
			checkFunc: func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
				return nil, boom
			},
			wantIs: boom,
		},
		{
			name: "resposta vazia devolve slice nil",
			checkFunc: func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
				return nil, nil
			},
		},
		{
			name: "dois telefones",
			checkFunc: func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
				return []domain.WhatsAppCheck{
					{Query: "5511987654321", IsIn: true, JID: "5511@s.whatsapp.net", VerifiedName: "Alice"},
					{Query: "5522987654321", IsIn: false},
				}, nil
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cd := &contractsfake.ContactDirectory{IsOnWhatsAppFunc: tt.checkFunc}
			if tt.session != nil {
				cd.SessionGuard = contractsfake.FailSession(tt.session)
			}
			logger := &contractsfake.Logger{}
			uc := user.NewCheckUserUseCase(cd, logger)

			got, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511987654321", "5522987654321"}})
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, queria %v", err, tt.wantIs)
				}
				if errors.Is(tt.wantIs, errNoSession) {
					assertNoSessionLog(t, logger, "u1")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, queria %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 {
				if got[0].Query != "5511987654321" || !got[0].IsInWhatsapp || got[0].VerifiedName != "Alice" {
					t.Errorf("primeiro resultado = %+v", got[0])
				}
				if got[1].IsInWhatsapp {
					t.Error("segundo resultado deveria estar fora do WhatsApp")
				}
			}
		})
	}
}

func TestGetContactsUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("sem sessão", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{SessionGuard: contractsfake.FailSession(errNoSession)}
		logger := &contractsfake.Logger{}
		uc := user.NewGetContactsUseCase(cd, logger)

		if _, err := uc.Execute(context.Background(), "u1", domain.GetContactsRequest{}); !errors.Is(err, errNoSession) {
			t.Fatalf("err = %v, queria errNoSession", err)
		}
		assertNoSessionLog(t, logger, "u1")
	})

	t.Run("erro do adapter sobe sem embrulho", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			GetAllContactsFunc: func(context.Context, string) ([]domain.Contact, int, error) { return nil, 0, boom },
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetContactsUseCase(cd, logger)

		_, err := uc.Execute(context.Background(), "u1", domain.GetContactsRequest{})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, queria boom", err)
		}
		if !logger.Logged("Failed to get contacts") {
			t.Error("esperava log de erro")
		}
	})

	t.Run("sucesso registra a contagem", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			GetAllContactsFunc: func(context.Context, string) ([]domain.Contact, int, error) {
				return []domain.Contact{{JID: "a@s.whatsapp.net"}, {JID: "b@lid"}}, 2, nil
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetContactsUseCase(cd, logger)

		got, err := uc.Execute(context.Background(), "u1", domain.GetContactsRequest{})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("resultado = %#v, queria os dois contatos", got)
		}
		rec, ok := logger.Find("Retrieved contacts")
		if !ok {
			t.Fatal("log de sucesso ausente")
		}
		if v, ok := rec.Keyval("count"); !ok || v != 2 {
			t.Errorf("keyval count = %v, queria 2", v)
		}
	})
}

func TestGetAvatarUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	badJID := errors.New("jid inválido")

	tests := []struct {
		name       string
		session    error
		req        domain.GetAvatarRequest
		resolveErr error
		picFunc    func(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error)
		wantErr    bool
		wantIs     error
	}{
		{
			name:    "sem sessão",
			session: errNoSession,
			req:     domain.GetAvatarRequest{Phone: "5511987654321"},
			wantErr: true,
			wantIs:  errNoSession,
		},
		{
			name:    "telefone ausente",
			req:     domain.GetAvatarRequest{},
			wantErr: true,
		},
		{
			name:       "telefone que não parseia",
			req:        domain.GetAvatarRequest{Phone: "??"},
			resolveErr: badJID,
			wantErr:    true,
		},
		{
			name: "falha ao buscar a foto",
			req:  domain.GetAvatarRequest{Phone: "5511987654321"},
			picFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
				return nil, boom
			},
			wantErr: true,
		},
		{
			name: "contato sem foto",
			req:  domain.GetAvatarRequest{Phone: "5511987654321"},
			picFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
				return nil, nil
			},
			wantErr: true,
		},
		{
			name: "foto encontrada",
			req:  domain.GetAvatarRequest{Phone: "5511987654321", Preview: true},
			picFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
				return &domain.AvatarInfo{ID: "pic-1", URL: "https://img/1.jpg"}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cd := &contractsfake.ContactDirectory{GetProfilePictureFunc: tt.picFunc}
			if tt.session != nil {
				cd.SessionGuard = contractsfake.FailSession(tt.session)
			}
			jr := &contractsfake.JIDResolver{}
			if tt.resolveErr != nil {
				jr.ResolveJIDFunc = func(context.Context, string) (domain.JID, error) { return "", tt.resolveErr }
			}
			logger := &contractsfake.Logger{}
			uc := user.NewGetAvatarUseCase(cd, jr, logger)

			got, err := uc.Execute(context.Background(), "u1", tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantIs != nil {
					if !errors.Is(err, tt.wantIs) {
						t.Fatalf("err = %v, queria %v", err, tt.wantIs)
					}
					assertNoSessionLog(t, logger, "u1")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got == nil || got.ID != "pic-1" || got.URL != "https://img/1.jpg" {
				t.Errorf("resultado = %v", got)
			}
			if len(cd.GetProfilePictureCalls) != 1 || !cd.GetProfilePictureCalls[0].Preview {
				t.Errorf("chamada de foto = %+v, queria preview true", cd.GetProfilePictureCalls)
			}
			if !logger.Logged("Got avatar") {
				t.Error("esperava log de sucesso")
			}
		})
	}
}

func TestGetUserUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	t.Run("sem sessão", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{SessionGuard: contractsfake.FailSession(errNoSession)}
		logger := &contractsfake.Logger{}
		uc := user.NewGetUserUseCase(cd, &contractsfake.JIDResolver{}, logger)

		if _, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{}); !errors.Is(err, errNoSession) {
			t.Fatalf("err = %v, queria errNoSession", err)
		}
		assertNoSessionLog(t, logger, "u1")
	})

	t.Run("telefone que não parseia é pulado, não é erro", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			GetUserInfoFunc: func(_ context.Context, _ string, jids []domain.JID) ([]domain.UserInfo, error) {
				if len(jids) != 1 || jids[0] != domain.JID("5511987654321") {
					t.Errorf("jids = %v, queria só o telefone válido", jids)
				}
				return []domain.UserInfo{{JID: "5511987654321", PushName: "Alice"}}, nil
			},
		}
		jr := &contractsfake.JIDResolver{
			ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
				if raw == "quebrado" {
					return "", errors.New("jid inválido")
				}
				return domain.JID(raw), nil
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetUserUseCase(cd, jr, logger)

		got, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511987654321", "quebrado"}})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if len(got) != 1 || got[0].PushName != "Alice" {
			t.Errorf("resultado = %+v", got)
		}
		if !logger.Logged("Failed to parse JID") {
			t.Error("esperava aviso do telefone descartado")
		}
	})

	t.Run("falha do adapter é embrulhada", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			GetUserInfoFunc: func(context.Context, string, []domain.JID) ([]domain.UserInfo, error) { return nil, boom },
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetUserUseCase(cd, &contractsfake.JIDResolver{}, logger)

		_, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511987654321"}})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, queria boom", err)
		}
		if !logger.Logged("Failed to get user info") {
			t.Error("esperava log de erro")
		}
	})

	// O subteste "resposta não serializável vira erro de marshal" saiu com o
	// ramo que ele exercitava: o caso de uso deixou de montar o corpo HTTP
	// (json.Marshal de {"users": …}), que é decisão da fronteira. Sem esse
	// ramo, o teste não media nada.
}

func TestGetUserLIDUseCase_Execute(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		session    error
		resolveErr error
		lidFunc    func(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error)
		wantErr    bool
		wantIs     error
		wantLID    string
	}{
		{
			name:    "sem sessão",
			session: errNoSession,
			wantErr: true,
			wantIs:  errNoSession,
		},
		{
			name:       "jid inválido",
			resolveErr: errors.New("formato ruim"),
			wantErr:    true,
		},
		{
			name:    "falha ao consultar o LID",
			lidFunc: func(context.Context, string, domain.JID) (domain.JID, error) { return "", boom },
			wantErr: true,
			wantIs:  boom,
		},
		{
			name:    "LID vazio é ausência, não sucesso",
			lidFunc: func(context.Context, string, domain.JID) (domain.JID, error) { return "", nil },
			wantErr: true,
		},
		{
			name:    "LID encontrado",
			lidFunc: func(context.Context, string, domain.JID) (domain.JID, error) { return "999@lid", nil },
			wantLID: "999@lid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cd := &contractsfake.ContactDirectory{GetLIDForPNFunc: tt.lidFunc}
			if tt.session != nil {
				cd.SessionGuard = contractsfake.FailSession(tt.session)
			}
			jr := &contractsfake.JIDResolver{}
			if tt.resolveErr != nil {
				jr.ResolveQualifiedJIDFunc = func(context.Context, string) (domain.JID, error) {
					return "", tt.resolveErr
				}
			}
			logger := &contractsfake.Logger{}
			uc := user.NewGetUserLIDUseCase(cd, jr, logger)

			got, err := uc.Execute(context.Background(), "u1", domain.GetUserLIDRequest{JID: "5511@s.whatsapp.net"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("esperava erro")
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, queria %v", err, tt.wantIs)
				}
				if errors.Is(err, errNoSession) {
					assertNoSessionLog(t, logger, "u1")
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got.LID != tt.wantLID || got.JID != "5511@s.whatsapp.net" {
				t.Errorf("resultado = %+v", got)
			}
		})
	}
}

// --- F182: tipo de JID errado é 400, não 500 --------------------------------

// TestGetUserLID_LIDRecusadoCom400 trava a CAUSA da F182: passar um LID a uma
// rota que resolve "o LID DE um telefone" é erro do CLIENTE, determinístico —
// repetir não adianta. Antes disto o pedido chegava ao store, que recusava, e
// o cliente recebia 500 com "internal server error": a mensagem útil ficava no
// log do servidor e ele não tinha como descobrir que passou o tipo errado.
func TestGetUserLID_LIDRecusadoCom400(t *testing.T) {
	t.Parallel()

	chamou := false
	cd := &contractsfake.ContactDirectory{
		GetLIDForPNFunc: func(context.Context, string, domain.JID) (domain.JID, error) {
			chamou = true
			return "", nil
		},
	}
	uc := user.NewGetUserLIDUseCase(cd, &contractsfake.JIDResolver{}, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1",
		domain.GetUserLIDRequest{JID: "182699419517150@lid"})
	if err == nil {
		t.Fatal("LID devia ser recusado por esta rota")
	}

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro não é apperr: %v", err)
	}
	// A CATEGORIA é o que decide o status HTTP (response.go:52). Asserir só a
	// existência do erro deixaria o 500 passar.
	if appErr.Category != apperr.CategoryValidation {
		t.Fatalf("categoria = %q, quero %q — categoria errada devolve 500 para um erro do cliente",
			appErr.Category, apperr.CategoryValidation)
	}
	// E a porta NÃO pode ser tocada: gastar uma consulta ao store por um
	// pedido que nunca poderia ter sucesso é o defeito com mais passos.
	if chamou {
		t.Fatal("o store foi consultado com um JID que a rota não aceita")
	}
}

// TestGetUserLID_FalhaDoStoreContinua500 é o outro lado, e é o que impede a
// correção de virar excesso: o comentário do código defende o 500 para falha
// de infraestrutura, porque o cliente não pode concluir "não existe" de uma
// falha transitória. Essa parte não muda.
func TestGetUserLID_FalhaDoStoreContinua500(t *testing.T) {
	t.Parallel()

	boom := errors.New("store fora do ar")
	cd := &contractsfake.ContactDirectory{
		GetLIDForPNFunc: func(context.Context, string, domain.JID) (domain.JID, error) {
			return "", boom
		},
	}
	uc := user.NewGetUserLIDUseCase(cd, &contractsfake.JIDResolver{}, &contractsfake.Logger{})

	_, err := uc.Execute(context.Background(), "u1",
		domain.GetUserLIDRequest{JID: "5511999999999@s.whatsapp.net"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, quero envolver %v", err, boom)
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr.Category == apperr.CategoryValidation {
		t.Fatal("falha de store virou validação: o cliente concluiria que o número não tem LID")
	}
}
