package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// Cobertura de UserHandlers (handler_user.go) — as 9 rotas de /admin/users e
// /user/*.
//
// Duas coisas sao medidas aqui, e a segunda e' a razao da F12:
//
//  1. o STATUS de cada caminho de saida (sucesso e cada recusa);
//  2. que cada resposta >=400 deixou um registro de log com a CAUSA — o que o
//     registro de fronteira do hlog, que anota toda requisicao, nao tem.
//
// (2) e' asserido pelo co-gate D (logassert.OutcomeLogged), sempre sobre a
// substring do erro que o TESTE plantou — nunca sobre o texto da mensagem de
// log (Principio 5).

// uhFakes reune os fakes da F11 que as 9 rotas consomem.
type uhFakes struct {
	users    *contractsfake.UserRepository
	contacts *contractsfake.ContactDirectory
	block    *contractsfake.BlocklistManager
	jids     *contractsfake.JIDResolver
	sessions *contractsfake.SessionStatusReader
	logger   *contractsfake.Logger
}

func uhNewFakes() *uhFakes {
	f := &uhFakes{
		users:    &contractsfake.UserRepository{},
		contacts: &contractsfake.ContactDirectory{},
		block:    &contractsfake.BlocklistManager{},
		jids:     &contractsfake.JIDResolver{},
		sessions: &contractsfake.SessionStatusReader{},
		logger:   &contractsfake.Logger{},
	}
	// Sem LID o use case de /user/lid recusa com "LID not found for this
	// number"; o caminho feliz precisa de um valor.
	f.contacts.GetLIDForPNFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
		return "5511999@lid", nil
	}
	return f
}

// failSession faz TODA porta com guarda de sessao recusar com err — e' a
// forma mais direta de exercitar o 500 das cinco rotas de /user/*.
func (f *uhFakes) failSession(err error) {
	f.contacts.EnsureSessionFunc = func(context.Context, string) error { return err }
	f.block.EnsureSessionFunc = func(context.Context, string) error { return err }
}

func (f *uhFakes) handlers() *UserHandlers {
	return NewUserHandlers(
		user.NewListUsersUseCase(f.users, f.logger, f.sessions),
		user.NewAddUserUseCase(f.users, f.logger),
		user.NewEditUserUseCase(f.users, f.logger),
		user.NewDeleteUserUseCase(f.users, f.logger),
		user.NewCheckUserUseCase(f.contacts, f.logger),
		user.NewGetUserUseCase(f.contacts, f.jids, f.logger),
		user.NewGetUserLIDUseCase(f.contacts, f.jids, f.logger),
		user.NewBlockUserUseCase(f.block, f.jids, f.logger),
		user.NewUnblockUserUseCase(f.block, f.jids, f.logger),
	)
}

// uhServe roda o handler ja' embrulhado na cadeia hlog do co-gate D e devolve
// a resposta e a captura de log.
func uhServe(h http.Handler, r *http.Request) (*httptest.ResponseRecorder, *logCapture) {
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	return rec, capture
}

func uhRequest(method, path, body string, vars map[string]string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if vars != nil {
		r = mux.SetURLVars(r, vars)
	}
	return r
}

// --- rotas de /admin/users ---------------------------------------------

func TestUserHandlers_AdminRoutes(t *testing.T) {
	boom := errors.New("uh-admin-boom")

	cases := []struct {
		name string
		// arrange configura os fakes para o caminho desejado.
		arrange func(*uhFakes)
		build   func(*UserHandlers) http.Handler
		method  string
		path    string
		body    string
		vars    map[string]string
		want    int
		// wantErrSubstring vazio significa "nao ha caminho de saida a asserir".
		wantErrSubstring string
	}{
		{
			name:   "ListUsers sucesso",
			build:  func(h *UserHandlers) http.Handler { return h.ListUsers() },
			method: http.MethodGet, path: "/admin/users", want: http.StatusOK,
		},
		{
			name: "ListUsers erro do repositorio",
			arrange: func(f *uhFakes) {
				f.users.ListUsersFunc = func(context.Context, string) ([]domain.UserListEntry, error) {
					return nil, boom
				}
			},
			build:  func(h *UserHandlers) http.Handler { return h.ListUsers() },
			method: http.MethodGet, path: "/admin/users", want: http.StatusInternalServerError,
			wantErrSubstring: boom.Error(),
		},
		{
			name:   "AddUser sucesso",
			build:  func(h *UserHandlers) http.Handler { return h.AddUser() },
			method: http.MethodPost, path: "/admin/users",
			body: `{"name":"alice","token":"tok-1"}`, want: http.StatusOK,
		},
		{
			name:   "AddUser corpo malformado",
			build:  func(h *UserHandlers) http.Handler { return h.AddUser() },
			method: http.MethodPost, path: "/admin/users",
			body: `{"name":"alice"`, want: http.StatusBadRequest,
			wantErrSubstring: "unexpected EOF",
		},
		{
			name:   "AddUser sem name nem token",
			build:  func(h *UserHandlers) http.Handler { return h.AddUser() },
			method: http.MethodPost, path: "/admin/users",
			body: `{}`, want: http.StatusInternalServerError,
			wantErrSubstring: "name and token are required",
		},
		{
			name: "AddUser token duplicado e' 409, nao 500",
			arrange: func(f *uhFakes) {
				f.users.CreateUserFunc = func(context.Context, domain.UserRecord) (bool, error) {
					return false, user.ErrDuplicateToken
				}
			},
			build:  func(h *UserHandlers) http.Handler { return h.AddUser() },
			method: http.MethodPost, path: "/admin/users",
			body: `{"name":"alice","token":"tok-1"}`, want: http.StatusConflict,
			wantErrSubstring: user.ErrDuplicateToken.Error(),
		},
		{
			name: "AddUser erro do repositorio",
			arrange: func(f *uhFakes) {
				f.users.CreateUserFunc = func(context.Context, domain.UserRecord) (bool, error) {
					return false, boom
				}
			},
			build:  func(h *UserHandlers) http.Handler { return h.AddUser() },
			method: http.MethodPost, path: "/admin/users",
			body: `{"name":"alice","token":"tok-1"}`, want: http.StatusInternalServerError,
			wantErrSubstring: boom.Error(),
		},
		{
			name: "EditUser sucesso",
			arrange: func(f *uhFakes) {
				f.users.UserExistsFunc = func(context.Context, string) (bool, error) { return true, nil }
			},
			build:  func(h *UserHandlers) http.Handler { return h.EditUser() },
			method: http.MethodPut, path: "/admin/users/u-1",
			body: `{"name":"bob"}`, vars: map[string]string{"id": "u-1"}, want: http.StatusOK,
		},
		{
			name:   "EditUser corpo malformado",
			build:  func(h *UserHandlers) http.Handler { return h.EditUser() },
			method: http.MethodPut, path: "/admin/users/u-1",
			body: `{"name":`, vars: map[string]string{"id": "u-1"}, want: http.StatusBadRequest,
			wantErrSubstring: "unexpected EOF",
		},
		{
			name: "EditUser usuario inexistente",
			arrange: func(f *uhFakes) {
				f.users.UserExistsFunc = func(context.Context, string) (bool, error) { return false, nil }
			},
			build:  func(h *UserHandlers) http.Handler { return h.EditUser() },
			method: http.MethodPut, path: "/admin/users/u-1",
			body: `{"name":"bob"}`, vars: map[string]string{"id": "u-1"}, want: http.StatusInternalServerError,
			wantErrSubstring: "user not found",
		},
		{
			name: "EditUser erro de escrita",
			arrange: func(f *uhFakes) {
				f.users.UserExistsFunc = func(context.Context, string) (bool, error) { return true, nil }
				f.users.UpdateUserFunc = func(context.Context, string, domain.UserUpdate) error { return boom }
			},
			build:  func(h *UserHandlers) http.Handler { return h.EditUser() },
			method: http.MethodPut, path: "/admin/users/u-1",
			body: `{"name":"bob"}`, vars: map[string]string{"id": "u-1"}, want: http.StatusInternalServerError,
			wantErrSubstring: boom.Error(),
		},
		{
			name: "EditUser sem id na URL",
			build: func(h *UserHandlers) http.Handler {
				return h.EditUser()
			},
			method: http.MethodPut, path: "/admin/users/",
			body: `{"name":"bob"}`, want: http.StatusInternalServerError,
			wantErrSubstring: "user ID is required",
		},
		{
			name:   "DeleteUser sucesso",
			build:  func(h *UserHandlers) http.Handler { return h.DeleteUser() },
			method: http.MethodDelete, path: "/admin/users/u-1",
			vars: map[string]string{"id": "u-1"}, want: http.StatusOK,
		},
		{
			name: "DeleteUser usuario inexistente",
			arrange: func(f *uhFakes) {
				f.users.DeleteUserFunc = func(context.Context, string) (bool, error) { return false, nil }
			},
			build:  func(h *UserHandlers) http.Handler { return h.DeleteUser() },
			method: http.MethodDelete, path: "/admin/users/u-1",
			vars: map[string]string{"id": "u-1"}, want: http.StatusInternalServerError,
			wantErrSubstring: "user not found",
		},
		{
			name: "DeleteUser erro do repositorio",
			arrange: func(f *uhFakes) {
				f.users.DeleteUserFunc = func(context.Context, string) (bool, error) { return false, boom }
			},
			build:  func(h *UserHandlers) http.Handler { return h.DeleteUser() },
			method: http.MethodDelete, path: "/admin/users/u-1",
			vars: map[string]string{"id": "u-1"}, want: http.StatusInternalServerError,
			wantErrSubstring: boom.Error(),
		},
		{
			name:   "DeleteUser sem id na URL",
			build:  func(h *UserHandlers) http.Handler { return h.DeleteUser() },
			method: http.MethodDelete, path: "/admin/users/", want: http.StatusInternalServerError,
			wantErrSubstring: "user ID is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()
			if tc.arrange != nil {
				tc.arrange(f)
			}

			rec, capture := uhServe(tc.build(f.handlers()), uhRequest(tc.method, tc.path, tc.body, tc.vars))

			if rec.Code != tc.want {
				t.Fatalf("status: got %d, want %d (corpo: %s)", rec.Code, tc.want, rec.Body.String())
			}
			recs := capture.Records(t)
			if tc.wantErrSubstring == "" {
				logassert.NoSecrets(t, recs)
				return
			}
			assertErrorEnvelope(t, rec, tc.want)
			logassert.OutcomeLogged(t, recs, tc.wantErrSubstring)
		})
	}
}

// --- rotas de /user/* com guarda de sessao ------------------------------

// uhSessionRoute descreve uma das cinco rotas que exigem userinfo no contexto.
type uhSessionRoute struct {
	name   string
	build  func(*UserHandlers) http.Handler
	method string
	path   string
	body   string
}

func uhSessionRoutes() []uhSessionRoute {
	return []uhSessionRoute{
		{"CheckUser", func(h *UserHandlers) http.Handler { return h.CheckUser() },
			http.MethodPost, "/user/check", `{"phone":["5511999"]}`},
		{"GetUser", func(h *UserHandlers) http.Handler { return h.GetUser() },
			http.MethodPost, "/user/info", `{"phone":["5511999"]}`},
		{"GetUserLID", func(h *UserHandlers) http.Handler { return h.GetUserLID() },
			http.MethodPost, "/user/lid", `{"JID":"5511999"}`},
		{"BlockUser", func(h *UserHandlers) http.Handler { return h.BlockUser() },
			http.MethodPost, "/user/block", `{"Phone":"5511999"}`},
		{"UnblockUser", func(h *UserHandlers) http.Handler { return h.UnblockUser() },
			http.MethodPost, "/user/unblock", `{"Phone":"5511999"}`},
	}
}

func TestUserHandlers_SessionRoutes_Sucesso(t *testing.T) {
	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()

			rec, capture := uhServe(tc.build(f.handlers()),
				withUser(uhRequest(tc.method, tc.path, tc.body, nil), "u-1"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// TestUserHandlers_SessionRoutes_SemUserInfo: sem o valor que o middleware
// injeta e' 401, e o 401 tem de deixar rastro com a causa.
func TestUserHandlers_SessionRoutes_SemUserInfo(t *testing.T) {
	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()

			rec, capture := uhServe(tc.build(f.handlers()), uhRequest(tc.method, tc.path, tc.body, nil))

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, capture.Records(t), errUnauthorized.Error())
			if len(f.contacts.EnsureSessionCalls)+len(f.block.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao nao autenticada alcancou a porta")
			}
		})
	}
}

// TestUserHandlers_SessionRoutes_TipoErradoNoContexto: o contexto carrega
// `any`; um valor que nao satisfaz userInfo e' 401, nao panico.
func TestUserHandlers_SessionRoutes_TipoErradoNoContexto(t *testing.T) {
	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()
			r := uhRequest(tc.method, tc.path, tc.body, nil)
			r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, "isto-nao-e-userinfo"))

			rec, capture := uhServe(tc.build(f.handlers()), r)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, capture.Records(t), errUnauthorized.Error())
		})
	}
}

// TestUserHandlers_SessionRoutes_SessionIDVazio: autenticado porem sem Id e'
// 400 — "sei quem voce e', mas nao ha sessao a que essa chamada se refira".
func TestUserHandlers_SessionRoutes_SessionIDVazio(t *testing.T) {
	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()

			rec, capture := uhServe(tc.build(f.handlers()),
				withUser(uhRequest(tc.method, tc.path, tc.body, nil), ""))

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, capture.Records(t), errMissingSessionID.Error())
			if len(f.contacts.EnsureSessionCalls)+len(f.block.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao sem session id alcancou a porta")
			}
		})
	}
}

func TestUserHandlers_SessionRoutes_CorpoMalformado(t *testing.T) {
	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()

			rec, capture := uhServe(tc.build(f.handlers()),
				withUser(uhRequest(tc.method, tc.path, `{"Phone": "5511`, nil), "u-1"))

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
			if len(f.contacts.EnsureSessionCalls)+len(f.block.EnsureSessionCalls) != 0 {
				t.Fatal("corpo malformado alcancou a porta")
			}
		})
	}
}

// TestUserHandlers_SessionRoutes_SessaoRecusada: a porta recusa a sessao e o
// 500 leva a causa ao log.
func TestUserHandlers_SessionRoutes_SessaoRecusada(t *testing.T) {
	boom := errors.New("uh-no-whatsmeow-session")

	for _, tc := range uhSessionRoutes() {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()
			f.failSession(boom)

			rec, capture := uhServe(tc.build(f.handlers()),
				withUser(uhRequest(tc.method, tc.path, tc.body, nil), "u-1"))

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, capture.Records(t), boom.Error())
		})
	}
}

// TestUserHandlers_SessionRoutes_PortaFalha exercita a falha da OPERACAO (nao
// da sessao) de cada rota: e' o segundo `if err != nil` de cada handler.
func TestUserHandlers_SessionRoutes_PortaFalha(t *testing.T) {
	boom := errors.New("uh-port-boom")

	cases := []struct {
		route   uhSessionRoute
		arrange func(*uhFakes)
	}{
		{uhSessionRoutes()[0], func(f *uhFakes) {
			f.contacts.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
				return nil, boom
			}
		}},
		{uhSessionRoutes()[1], func(f *uhFakes) {
			f.contacts.GetUserInfoFunc = func(context.Context, string, []domain.JID) (any, error) {
				return nil, boom
			}
		}},
		{uhSessionRoutes()[2], func(f *uhFakes) {
			f.contacts.GetLIDForPNFunc = func(context.Context, string, domain.JID) (domain.JID, error) {
				return "", boom
			}
		}},
		{uhSessionRoutes()[3], func(f *uhFakes) {
			f.block.UpdateBlocklistFunc = func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{}, boom
			}
		}},
		{uhSessionRoutes()[4], func(f *uhFakes) {
			f.block.UpdateBlocklistFunc = func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
				return domain.BlocklistUpdate{}, boom
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.route.name, func(t *testing.T) {
			f := uhNewFakes()
			tc.arrange(f)

			rec, capture := uhServe(tc.route.build(f.handlers()),
				withUser(uhRequest(tc.route.method, tc.route.path, tc.route.body, nil), "u-1"))

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, capture.Records(t), boom.Error())
		})
	}
}

// TestUserHandlers_BlockSemAlvo: bloquear sem Phone nem JID e' recusa de
// validacao do use case, e chega a' fronteira como 500 com causa logada.
func TestUserHandlers_BlockSemAlvo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*UserHandlers) http.Handler
		path  string
	}{
		{"BlockUser", func(h *UserHandlers) http.Handler { return h.BlockUser() }, "/user/block"},
		{"UnblockUser", func(h *UserHandlers) http.Handler { return h.UnblockUser() }, "/user/unblock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := uhNewFakes()

			rec, capture := uhServe(tc.build(f.handlers()),
				withUser(uhRequest(http.MethodPost, tc.path, `{}`, nil), "u-1"))

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, capture.Records(t), "missing Phone or JID")
		})
	}
}

// TestUserHandlers_NaoLogamSegredoDoPayload e' a regressao permanente da F9.4
// medida onde ela importa: AddUser recebe um token no corpo, e o caminho de
// saida loga a causa — sem carregar o token junto.
func TestUserHandlers_NaoLogamSegredoDoPayload(t *testing.T) {
	f := uhNewFakes()
	f.users.CreateUserFunc = func(context.Context, domain.UserRecord) (bool, error) {
		return false, user.ErrDuplicateToken
	}

	body := `{"name":"alice","token":"` + logassertAdminToken +
		`","hmacKey":"` + logassertGlobalHMACKey + `"}`
	rec, capture := uhServe(f.handlers().AddUser(),
		uhRequest(http.MethodPost, "/admin/users", body, nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409 (corpo: %s)", rec.Code, rec.Body.String())
	}
	recs := capture.Records(t)
	logassert.OutcomeLogged(t, recs, user.ErrDuplicateToken.Error())
	logassert.NoSecrets(t, recs, logassertGlobalEncryptionKey)
}
