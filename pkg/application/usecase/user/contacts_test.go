package user_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// errNoSession é o erro que o guarda de sessão devolve nos testes; o contrato
// depois da migração é que ele suba intacto, não embrulhado numa string.
var errNoSession = errors.New("session not found")

// assertNoSessionLog verifica a forma do log padronizado de sessão ausente.
func assertNoSessionLog(t *testing.T, logger *contractsfake.Logger, userID string) {
	t.Helper()
	rec, ok := logger.FindLevel(contractsfake.LevelError, "no whatsmeow session")
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
					{Query: "5511", IsIn: true, JID: "5511@s.whatsapp.net", VerifiedName: "Alice"},
					{Query: "5522", IsIn: false},
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

			got, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511", "5522"}})
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
				if got[0].Query != "5511" || !got[0].IsInWhatsapp || got[0].VerifiedName != "Alice" {
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
			GetAllContactsFunc: func(context.Context, string) (any, int, error) { return nil, 0, boom },
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
			GetAllContactsFunc: func(context.Context, string) (any, int, error) {
				return []string{"a", "b"}, 2, nil
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetContactsUseCase(cd, logger)

		got, err := uc.Execute(context.Background(), "u1", domain.GetContactsRequest{})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		list, ok := got.([]string)
		if !ok || len(list) != 2 {
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
			req:     domain.GetAvatarRequest{Phone: "5511"},
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
			req:  domain.GetAvatarRequest{Phone: "5511"},
			picFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
				return nil, boom
			},
			wantErr: true,
		},
		{
			name: "contato sem foto",
			req:  domain.GetAvatarRequest{Phone: "5511"},
			picFunc: func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
				return nil, nil
			},
			wantErr: true,
		},
		{
			name: "foto encontrada",
			req:  domain.GetAvatarRequest{Phone: "5511", Preview: true},
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
			if got["id"] != "pic-1" || got["url"] != "https://img/1.jpg" {
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
			GetUserInfoFunc: func(_ context.Context, _ string, jids []domain.JID) (any, error) {
				if len(jids) != 1 || jids[0] != domain.JID("5511") {
					t.Errorf("jids = %v, queria só o telefone válido", jids)
				}
				return map[string]string{"5511": "Alice"}, nil
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

		data, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511", "quebrado"}})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		var payload map[string]map[string]string
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("resposta não é JSON: %v", err)
		}
		if payload["users"]["5511"] != "Alice" {
			t.Errorf("payload = %v", payload)
		}
		if !logger.Logged("Failed to parse JID") {
			t.Error("esperava aviso do telefone descartado")
		}
	})

	t.Run("falha do adapter é embrulhada", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			GetUserInfoFunc: func(context.Context, string, []domain.JID) (any, error) { return nil, boom },
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetUserUseCase(cd, &contractsfake.JIDResolver{}, logger)

		_, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511"}})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, queria boom", err)
		}
		if !logger.Logged("Failed to get user info") {
			t.Error("esperava log de erro")
		}
	})

	t.Run("resposta não serializável vira erro de marshal", func(t *testing.T) {
		t.Parallel()
		cd := &contractsfake.ContactDirectory{
			// Um canal não tem representação JSON: é o gatilho do ramo de
			// erro de serialização sem precisar de tipo artificial.
			GetUserInfoFunc: func(context.Context, string, []domain.JID) (any, error) {
				return make(chan int), nil
			},
		}
		logger := &contractsfake.Logger{}
		uc := user.NewGetUserUseCase(cd, &contractsfake.JIDResolver{}, logger)

		var unsupported *json.UnsupportedTypeError
		_, err := uc.Execute(context.Background(), "u1", domain.CheckUserRequest{Phone: []string{"5511"}})
		if !errors.As(err, &unsupported) {
			t.Fatalf("err = %v, queria json.UnsupportedTypeError", err)
		}
		if !logger.Logged("Failed to marshal response") {
			t.Error("esperava log do erro de serialização")
		}
	})
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
