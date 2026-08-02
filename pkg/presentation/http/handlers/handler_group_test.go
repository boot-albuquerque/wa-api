package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
)

// Cobertura dos handlers de LEITURA de grupo (handler_group.go).
//
// Os sete handlers deste arquivo tem a mesma forma: guarda de sessao, decode
// do corpo, chamada ao use case, envelope. Nenhum retorna error — o
// denominador de caminho de saida vem de S-http (resposta >=400) e, para cada
// um deles, o co-gate D exige que o 400/500 tenha logado a CAUSA no logger
// request-scoped. E' isso que os testes abaixo travam: nao o texto da
// mensagem, mas a existencia de um registro warn/error com `error` e `req_id`.

// grpErrPort e' a falha que a porta de grupo devolve nos casos negativos. Um
// sentinela e' o que permite assertar que a causa ATRAVESSOU o use case ate' o
// log, em vez de ser trocada por um texto generico no caminho.
var grpErrPort = errors.New("group port exploded")

// grpErrNoSession e' a recusa da guarda de sessao.
var grpErrNoSession = errors.New("no whatsmeow session for user")

// grpFakes reune os fakes da F11 que os use cases de grupo consomem. O
// zero-value de cada um e' o caminho feliz; os casos negativos injetam Func.
type grpFakes struct {
	requests  *contractsfake.GroupRequests
	directory *contractsfake.GroupDirectory
	jids      *contractsfake.JIDResolver
	logger    *contractsfake.Logger
}

func newGrpFakes() *grpFakes {
	return &grpFakes{
		requests:  &contractsfake.GroupRequests{},
		directory: &contractsfake.GroupDirectory{},
		jids:      &contractsfake.JIDResolver{},
		logger:    &contractsfake.Logger{},
	}
}

// failSession faz as duas portas de grupo recusarem por falta de sessao.
func (f *grpFakes) failSession(err error) {
	f.requests.EnsureSessionFunc = func(context.Context, string) error { return err }
	f.directory.EnsureSessionFunc = func(context.Context, string) error { return err }
}

// grpReadCase descreve um handler de leitura e os corpos que o exercitam.
type grpReadCase struct {
	name   string
	method string
	path   string
	// body e' o corpo que leva o handler ao 200.
	body string
	// readsBody diz se o handler decodifica JSON. O unico que nao le e'
	// ListGroups; e' tambem o unico sem validacao de campo obrigatorio, e
	// por isso a flag serve aos dois testes.
	readsBody bool
	build     func(*grpFakes) http.Handler
	// failOp injeta falha na operacao (nao na sessao) que ESTE handler
	// consome — o 500 que vem da dependencia real, nao da guarda.
	failOp func(*grpFakes, error)
}

func grpReadCases() []grpReadCase {
	return []grpReadCase{
		{
			name:      "GetGroupRequestParticipants",
			method:    http.MethodPost,
			path:      "/group/requests",
			body:      `{"groupJID":"120363@g.us"}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewGetGroupRequestParticipantsHandler(group.NewGroupRequestUseCase(f.requests, f.jids, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.requests.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) (any, error) {
					return nil, err
				}
			},
		},
		{
			name:      "UpdateGroupRequestParticipants",
			method:    http.MethodPost,
			path:      "/group/requests/update",
			body:      `{"groupJID":"120363@g.us","Phone":["5511999999999"],"Action":"approve"}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewUpdateGroupRequestParticipantsHandler(group.NewGroupRequestUseCase(f.requests, f.jids, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.requests.UpdateRequestParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.RequestAction) error {
					return err
				}
			},
		},
		{
			name:      "SetGroupJoinApprovalMode",
			method:    http.MethodPost,
			path:      "/group/join/approval",
			body:      `{"groupjid":"120363@g.us","mode":true}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewSetGroupJoinApprovalModeHandler(group.NewGroupRequestUseCase(f.requests, f.jids, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.requests.SetJoinApprovalModeFunc = func(context.Context, string, domain.JID, bool) error { return err }
			},
		},
		{
			name:      "ListGroups",
			method:    http.MethodGet,
			path:      "/group/list",
			body:      "",
			readsBody: false,
			build: func(f *grpFakes) http.Handler {
				return NewListGroupsHandler(group.NewListGroupsUseCase(f.directory, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.directory.ListJoinedGroupsFunc = func(context.Context, string) (any, int, error) { return nil, 0, err }
			},
		},
		{
			name:      "GetGroupInfo",
			method:    http.MethodPost,
			path:      "/group/info",
			body:      `{"groupJID":"120363@g.us"}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewGetGroupInfoHandler(group.NewGetGroupInfoUseCase(f.directory, f.jids, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (any, error) { return nil, err }
			},
		},
		{
			name:      "GetGroupInviteLink",
			method:    http.MethodPost,
			path:      "/group/invitelink",
			body:      `{"groupJID":"120363@g.us"}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewGetGroupInviteLinkHandler(group.NewGetGroupInviteLinkUseCase(f.directory, f.jids, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.directory.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) { return "", err }
			},
		},
		{
			name:      "GetGroupInviteInfo",
			method:    http.MethodPost,
			path:      "/group/inviteinfo",
			body:      `{"Code":"AbCdEf"}`,
			readsBody: true,
			build: func(f *grpFakes) http.Handler {
				return NewGetGroupInviteInfoHandler(group.NewGetGroupInviteInfoUseCase(f.directory, f.logger))
			},
			failOp: func(f *grpFakes, err error) {
				f.directory.GetGroupInfoFromLinkFunc = func(context.Context, string, string) (any, error) { return nil, err }
			},
		},
	}
}

// grpServe roda o handler dentro da MESMA cadeia request-scoped de producao
// (co-gate D) e com o usuario que o middleware de autenticacao injetaria.
func grpServe(tc grpReadCase, f *grpFakes, body string) (*httptest.ResponseRecorder, *logCapture) {
	h, capture := logassert.Wrap(tc.build(f))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
	h.ServeHTTP(rec, withUser(r, "user-1"))
	return rec, capture
}

// TestGroupReadHandlers_Success: com sessao valida e corpo bem formado, cada
// handler devolve 200 e um envelope de sucesso.
func TestGroupReadHandlers_Success(t *testing.T) {
	for _, tc := range grpReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			rec, _ := grpServe(tc, f, tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false num caminho feliz: %s", rec.Body.String())
			}
		})
	}
}

// TestGroupReadHandlers_MalformedBody: corpo ilegivel e' 400 do cliente, e o
// caminho de saida tem de logar a causa do decode.
func TestGroupReadHandlers_MalformedBody(t *testing.T) {
	for _, tc := range grpReadCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			rec, capture := grpServe(tc, f, `{"groupJID": "120363`)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			got := logassert.OutcomeLogged(t, capture.Records(t))
			if got.str("level") != "warn" {
				t.Fatalf("rejeicao causada pelo cliente logada em %q, quero warn", got.str("level"))
			}
		})
	}
}

// TestGroupReadHandlers_MissingRequiredField: campo obrigatorio ausente e' um
// caminho de saida — o handler nao pode devolver 200 nem sair calado.
func TestGroupReadHandlers_MissingRequiredField(t *testing.T) {
	for _, tc := range grpReadCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			rec, capture := grpServe(tc, f, `{}`)

			if rec.Code < 400 {
				t.Fatalf("campo obrigatorio ausente produziu status %d", rec.Code)
			}
			logassert.OutcomeLogged(t, capture.Records(t), "missing")
		})
	}
}

// TestGroupReadHandlers_SessionFailure: sem sessao o handler responde 500 e
// loga a causa em nivel error — e' falha de dependencia, nao do cliente.
func TestGroupReadHandlers_SessionFailure(t *testing.T) {
	for _, tc := range grpReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			f.failSession(grpErrNoSession)

			rec, capture := grpServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), grpErrNoSession.Error())
			if got.str("level") != "error" {
				t.Fatalf("falha de dependencia logada em %q, quero error", got.str("level"))
			}
		})
	}
}

// TestGroupReadHandlers_UseCaseFailure: a operacao em si falha depois da
// guarda de sessao. E' o caminho que so' existe quando a porta responde.
func TestGroupReadHandlers_UseCaseFailure(t *testing.T) {
	for _, tc := range grpReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			tc.failOp(f, grpErrPort)

			rec, capture := grpServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), grpErrPort.Error())
			if got.str("level") != "error" {
				t.Fatalf("falha de porta logada em %q, quero error", got.str("level"))
			}
		})
	}
}

// TestGroupReadHandlers_NeverLogSecrets e' a clausula (d) do co-gate D
// exercitada contra codigo real: os tres segredos da F9.4 viajam na
// requisicao (header e corpo) e a porta ainda falha, forcando o caminho que
// mais loga. Nenhum registro pode carrega-los.
func TestGroupReadHandlers_NeverLogSecrets(t *testing.T) {
	for _, tc := range grpReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			tc.failOp(f, grpErrPort)

			h, capture := logassert.Wrap(tc.build(f))
			rec := httptest.NewRecorder()
			body := tc.body
			if tc.readsBody {
				body = `{"groupJID":"120363@g.us","Code":"AbCdEf","secret":"` +
					logassertGlobalEncryptionKey + `","hmac":"` + logassertGlobalHMACKey + `"}`
			}
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			r.Header.Set("Authorization", logassertAdminToken)
			h.ServeHTTP(rec, withUser(r, "user-1"))

			if rec.Code < 400 {
				t.Fatalf("o caso de vazamento precisa de um caminho de saida; status %d", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}

// TestGroupReadHandlers_RejectWithoutSession trava a guarda de fronteira dos
// sete handlers de leitura: sem o valor que o middleware injeta e' 401, com
// ele porem sem Id e' 400 — e em nenhum dos dois a porta pode ser tocada.
func TestGroupReadHandlers_RejectWithoutSession(t *testing.T) {
	for _, tc := range grpReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrpFakes()
			h, _ := logassert.Wrap(tc.build(f))

			semUser := httptest.NewRecorder()
			h.ServeHTTP(semUser, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			assertErrorEnvelope(t, semUser, http.StatusUnauthorized)

			semID := httptest.NewRecorder()
			h.ServeHTTP(semID, withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), ""))
			assertErrorEnvelope(t, semID, http.StatusBadRequest)

			if n := len(f.requests.EnsureSessionCalls) + len(f.directory.EnsureSessionCalls); n != 0 {
				t.Fatalf("requisicao sem sessao alcancou a porta %d vez(es)", n)
			}
		})
	}
}
