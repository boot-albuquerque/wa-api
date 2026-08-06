package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// /user/avatar, /user/contacts e /user/info (handler_contact.go).
//
// Nota sobre o co-gate D nestes tres handlers: as recusas 401/"sem userinfo" e
// 400/"session id vazio" NAO sao emitidas aqui — vem do helper compartilhado
// sessionUser (handler_session.go), que responde e devolve false. O teste
// dessas duas cobre o STATUS; o log delas e' responsabilidade do arquivo que
// hospeda o helper. Todo caminho de saida que ESTE arquivo produz — corpo
// ilegivel e falha do use case — assere o co-gate D.

type chFakes struct {
	contacts *contractsfake.ContactDirectory
	jids     *contractsfake.JIDResolver
	logger   *contractsfake.Logger
}

func chNewFakes() *chFakes {
	f := &chFakes{
		contacts: &contractsfake.ContactDirectory{},
		jids:     &contractsfake.JIDResolver{},
		logger:   &contractsfake.Logger{},
	}
	// Sem foto o use case de avatar recusa com "no avatar found"; o caminho
	// feliz precisa de uma.
	f.contacts.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
		return &domain.AvatarInfo{ID: "pic-1", URL: "https://example.com/pic-1"}, nil
	}
	return f
}

func (f *chFakes) avatar() http.Handler {
	return NewGetAvatarHandler(user.NewGetAvatarUseCase(f.contacts, f.jids, f.logger))
}

func (f *chFakes) contactsHandler() http.Handler {
	return NewGetContactsHandler(user.NewGetContactsUseCase(f.contacts, f.logger))
}

func (f *chFakes) userInfoHandler() http.Handler {
	return NewGetUserInfoHandler(user.NewGetUserUseCase(f.contacts, f.jids, f.logger))
}

// --- caminho feliz -----------------------------------------------------

func TestContactHandlers_Sucesso(t *testing.T) {
	cases := []struct {
		name   string
		build  func(*chFakes) http.Handler
		method string
		path   string
		body   string
	}{
		{"GetAvatar", func(f *chFakes) http.Handler { return f.avatar() },
			http.MethodPost, "/user/avatar", `{"Phone":"5511999"}`},
		{"GetContacts", func(f *chFakes) http.Handler { return f.contactsHandler() },
			http.MethodGet, "/user/contacts", ""},
		{"GetUserInfo", func(f *chFakes) http.Handler { return f.userInfoHandler() },
			http.MethodPost, "/user/info", `{"phone":["5511999"]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := chNewFakes()

			rec, capture := uhServe(tc.build(f),
				withUser(uhRequest(tc.method, tc.path, tc.body, nil), "u-1"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// --- fronteira: sem userinfo / sem session id --------------------------

func TestContactHandlers_FronteiraDeSessao(t *testing.T) {
	builds := map[string]struct {
		build  func(*chFakes) http.Handler
		method string
		path   string
	}{
		"GetAvatar":   {func(f *chFakes) http.Handler { return f.avatar() }, http.MethodPost, "/user/avatar"},
		"GetContacts": {func(f *chFakes) http.Handler { return f.contactsHandler() }, http.MethodGet, "/user/contacts"},
		"GetUserInfo": {func(f *chFakes) http.Handler { return f.userInfoHandler() }, http.MethodPost, "/user/info"},
	}

	inputs := []struct {
		name string
		mut  func(*http.Request) *http.Request
		want int
	}{
		{"sem userinfo", func(r *http.Request) *http.Request { return r }, http.StatusUnauthorized},
		{"tipo errado no contexto", func(r *http.Request) *http.Request {
			return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
		}, http.StatusUnauthorized},
		{"session id vazio", func(r *http.Request) *http.Request { return withUser(r, "") }, http.StatusBadRequest},
	}

	for name, b := range builds {
		for _, in := range inputs {
			t.Run(name+"/"+in.name, func(t *testing.T) {
				f := chNewFakes()

				rec, _ := uhServe(b.build(f), in.mut(uhRequest(b.method, b.path, `{}`, nil)))

				assertErrorEnvelope(t, rec, in.want)
				if len(f.contacts.EnsureSessionCalls) != 0 {
					t.Fatal("recusa de fronteira alcancou a porta")
				}
			})
		}
	}
}

// --- corpo ilegivel ----------------------------------------------------

func TestContactHandlers_CorpoMalformado(t *testing.T) {
	cases := []struct {
		name  string
		build func(*chFakes) http.Handler
		path  string
	}{
		{"GetAvatar", func(f *chFakes) http.Handler { return f.avatar() }, "/user/avatar"},
		{"GetUserInfo", func(f *chFakes) http.Handler { return f.userInfoHandler() }, "/user/info"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := chNewFakes()

			rec, capture := uhServe(tc.build(f),
				withUser(uhRequest(http.MethodPost, tc.path, `{"Phone": "5511`, nil), "u-1"))

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
			if len(f.contacts.EnsureSessionCalls) != 0 {
				t.Fatal("corpo malformado alcancou a porta")
			}
		})
	}
}

// --- falhas do use case ------------------------------------------------

func TestContactHandlers_UseCaseFalha(t *testing.T) {
	sessionBoom := errors.New("ch-no-session")
	portBoom := errors.New("ch-port-boom")

	cases := []struct {
		name             string
		arrange          func(*chFakes)
		build            func(*chFakes) http.Handler
		method           string
		path             string
		body             string
		wantErrSubstring string
		wantStatus       int // 0 → default http.StatusInternalServerError
	}{
		{
			name: "GetAvatar sessao recusada",
			arrange: func(f *chFakes) {
				f.contacts.EnsureSessionFunc = func(context.Context, string) error { return sessionBoom }
			},
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{"Phone":"5511999"}`,
			wantErrSubstring: sessionBoom.Error(),
		},
		{
			name:   "GetAvatar sem Phone no payload",
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{}`,
			wantErrSubstring: "missing Phone in Payload",
		},
		{
			name: "GetAvatar JID nao parseia",
			arrange: func(f *chFakes) {
				f.jids.ResolveJIDFunc = func(context.Context, string) (domain.JID, error) {
					return "", errors.New("bad jid")
				}
			},
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{"Phone":"nao-e-um-numero"}`,
			wantErrSubstring: "could not parse Phone",
		},
		{
			name: "GetAvatar porta falha",
			arrange: func(f *chFakes) {
				f.contacts.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
					return nil, portBoom
				}
			},
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{"Phone":"5511999"}`,
			wantErrSubstring: portBoom.Error(),
		},
		{
			name: "GetAvatar sem foto",
			arrange: func(f *chFakes) {
				f.contacts.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
					return nil, nil
				}
			},
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{"Phone":"5511999"}`,
			wantErrSubstring: "no avatar found",
			// Contato sem foto pública é o caso comum, não uma falha de servidor —
			// distinto dos demais casos desta tabela (sessão/porta/JID reais falhando).
			wantStatus: http.StatusNotFound,
		},
		{
			name: "GetAvatar escondido por privacidade",
			arrange: func(f *chFakes) {
				f.contacts.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
					return nil, domain.ErrAvatarUnauthorized
				}
			},
			build:  func(f *chFakes) http.Handler { return f.avatar() },
			method: http.MethodPost, path: "/user/avatar", body: `{"Phone":"5511999"}`,
			wantErrSubstring: domain.ErrAvatarUnauthorized.Error(),
			// Distinto de "sem foto" (404): o contato TEM foto, só está oculta
			// pela privacidade dele — 403, não 404.
			wantStatus: http.StatusForbidden,
		},
		{
			name: "GetContacts sessao recusada",
			arrange: func(f *chFakes) {
				f.contacts.EnsureSessionFunc = func(context.Context, string) error { return sessionBoom }
			},
			build:  func(f *chFakes) http.Handler { return f.contactsHandler() },
			method: http.MethodGet, path: "/user/contacts",
			wantErrSubstring: sessionBoom.Error(),
		},
		{
			name: "GetContacts porta falha",
			arrange: func(f *chFakes) {
				f.contacts.GetAllContactsFunc = func(context.Context, string) (any, int, error) {
					return nil, 0, portBoom
				}
			},
			build:  func(f *chFakes) http.Handler { return f.contactsHandler() },
			method: http.MethodGet, path: "/user/contacts",
			wantErrSubstring: portBoom.Error(),
		},
		{
			name: "GetUserInfo sessao recusada",
			arrange: func(f *chFakes) {
				f.contacts.EnsureSessionFunc = func(context.Context, string) error { return sessionBoom }
			},
			build:  func(f *chFakes) http.Handler { return f.userInfoHandler() },
			method: http.MethodPost, path: "/user/info", body: `{"phone":["5511999"]}`,
			wantErrSubstring: sessionBoom.Error(),
		},
		{
			name: "GetUserInfo porta falha",
			arrange: func(f *chFakes) {
				f.contacts.GetUserInfoFunc = func(context.Context, string, []domain.JID) (any, error) {
					return nil, portBoom
				}
			},
			build:  func(f *chFakes) http.Handler { return f.userInfoHandler() },
			method: http.MethodPost, path: "/user/info", body: `{"phone":["5511999"]}`,
			wantErrSubstring: portBoom.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := chNewFakes()
			if tc.arrange != nil {
				tc.arrange(f)
			}

			rec, capture := uhServe(tc.build(f),
				withUser(uhRequest(tc.method, tc.path, tc.body, nil), "u-1"))

			wantStatus := tc.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusInternalServerError
			}
			assertErrorEnvelope(t, rec, wantStatus)
			logassert.OutcomeLogged(t, capture.Records(t), tc.wantErrSubstring)
		})
	}
}

// TestGetUserInfo_TelefoneInvalidoNaoDerrubaAChamada: um telefone que nao
// parseia e' PULADO, nao vira erro — comportamento preservado do upstream. A
// chamada segue e responde 200.
func TestGetUserInfo_TelefoneInvalidoNaoDerrubaAChamada(t *testing.T) {
	f := chNewFakes()
	f.jids.ResolveQualifiedJIDFunc = func(_ context.Context, raw string) (domain.JID, error) {
		if raw == "invalido" {
			return "", errors.New("bad jid")
		}
		return domain.JID(raw), nil
	}

	rec, capture := uhServe(f.userInfoHandler(),
		withUser(uhRequest(http.MethodPost, "/user/info", `{"phone":["invalido","5511999"]}`, nil), "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(f.contacts.GetUserInfoCalls); n != 1 {
		t.Fatalf("GetUserInfo chamado %d vez(es), quero 1", n)
	}
	if jids := f.contacts.GetUserInfoCalls[0].JIDs; len(jids) != 1 || jids[0] != domain.JID("5511999") {
		t.Fatalf("o telefone invalido nao foi pulado: %v", jids)
	}
	logassert.NoSecrets(t, capture.Records(t))
}
