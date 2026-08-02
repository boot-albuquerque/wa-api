package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Este arquivo cobre handler_session.go inteiro: os 8 handlers das rotas
// /session e o helper sessionUser que todos compartilham.
//
// O eixo que ele mede, e que a fronteira em handler_boundary_test.go nao
// mede, e' o CAMINHO DE SAIDA: para cada resposta >=400, existe um registro
// de log que carrega a CAUSA, no nivel certo, correlacionavel por req_id.
// O log de fronteira do router (router.go:208-214) registra toda requisicao
// com status — e nada mais. Um 400 de `no_session` e um 500 de banco fora do
// ar sao indistinguiveis nele. E' este registro que os separa.
//
// A distincao de NIVEL nao e' cosmetica: isClientCausedSessionError escolhe
// warn ou error lendo apperr.Category, e um alerta calibrado sobre `error`
// dispara em falso a cada cliente que pede QR sem sessao se essa escolha
// estiver errada.

// sessionSecretUser planta os tres segredos-sentinela do co-gate D no valor
// que o middleware guarda no contexto. Get devolve o segredo para QUALQUER
// chave que nao seja "Id" — se algum handler resolver logar o userinfo
// inteiro, ou um campo dele que nao o Id, a clausula (d) morde.
type sessionSecretUser struct{ id string }

func (u sessionSecretUser) Get(key string) string {
	switch key {
	case "Id":
		return u.id
	case "Token":
		return logassertAdminToken
	case "EncryptionKey":
		return logassertGlobalEncryptionKey
	default:
		return logassertGlobalHMACKey
	}
}

// serveSession roda o handler com a cadeia hlog request-scoped instalada e
// devolve resposta e registros. `id` vazio produz userinfo com Id vazio;
// authed=false nao injeta userinfo nenhum.
func serveSession(t *testing.T, h http.Handler, method, path, body, id string, authed bool) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authed {
		req = req.WithContext(context.WithValue(req.Context(), appport.UserInfoKey, sessionSecretUser{id: id}))
	}
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	return rec, capture.Records(t)
}

func sessionEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo %s)", err, rec.Body.String())
	}
	return env
}

// noSessionErr e' o erro tipado que a porta whatsmeow produz quando nao ha
// sessao: categoria validation, logo 400 na fronteira.
func noSessionErr() error {
	return apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
}

// sessionCase descreve um handler de /session e o corpo minimo que ele aceita.
type sessionCase struct {
	name string
	// build liga o handler a um SessionGuard que falha com err.
	build     func(err error) http.Handler
	method    string
	path      string
	body      string
	readsBody bool
}

func sessionCases() []sessionCase {
	log := &contractsfake.Logger{}
	guard := func(err error) *contractsfake.SessionGuard {
		g := contractsfake.FailSession(err)
		return &g
	}
	users := func() *contractsfake.UserRepository {
		return &contractsfake.UserRepository{
			ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
				return []domain.UserListEntry{{ID: "user-1", QRCode: "qr-data"}}, nil
			},
		}
	}

	return []sessionCase{
		{
			name:   "Disconnect",
			build:  func(e error) http.Handler { return NewDisconnectHandler(session.NewDisconnectUseCase(guard(e), log)) },
			method: http.MethodPost,
			path:   "/session/disconnect",
		},
		{
			name:   "Logout",
			build:  func(e error) http.Handler { return NewLogoutHandler(session.NewLogoutUseCase(guard(e), log)) },
			method: http.MethodPost,
			path:   "/session/logout",
		},
		{
			name: "GetQR",
			build: func(e error) http.Handler {
				return NewGetQRHandler(session.NewGetQRUseCase(guard(e), users(), log))
			},
			method: http.MethodGet,
			path:   "/session/qr",
		},
		{
			name: "GetStatus",
			build: func(e error) http.Handler {
				return NewGetStatusHandler(session.NewGetStatusUseCase(guard(e), &contractsfake.SessionStatusReader{}, users(), log))
			},
			method: http.MethodGet,
			path:   "/session/status",
		},
		{
			name: "RequestHistorySync",
			build: func(e error) http.Handler {
				return NewRequestHistorySyncHandler(session.NewRequestHistorySyncUseCase(guard(e), log))
			},
			method: http.MethodPost,
			path:   "/session/historysync",
		},
		{
			name: "PairPhone",
			build: func(e error) http.Handler {
				return NewPairPhoneHandler(session.NewPairPhoneUseCase(guard(e), log))
			},
			method:    http.MethodPost,
			path:      "/session/pairphone",
			body:      `{"Phone":"5511999999999"}`,
			readsBody: true,
		},
		{
			name: "SetStatusMessage",
			build: func(e error) http.Handler {
				return NewSetStatusMessageHandler(session.NewSetStatusMessageUseCase(guard(e), log))
			},
			method:    http.MethodPost,
			path:      "/session/statusmessage",
			body:      `{"Body":"disponivel"}`,
			readsBody: true,
		},
		{
			name:   "Connect",
			build:  func(error) http.Handler { return NewConnectHandler(session.NewConnectUseCase(log)) },
			method: http.MethodPost,
			path:   "/session/connect",
		},
	}
}

// TestSessionHandlers_Unauthenticated_LogsCause: sem userinfo e' 401 E o
// registro que diz por que. sessionUser e' o unico ponto que sabe a causa.
func TestSessionHandlers_Unauthenticated_LogsCause(t *testing.T) {
	for _, tc := range sessionCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(nil), tc.method, tc.path, tc.body, "", false)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, quero 401 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs, "unauthorized")
			if got.str("level") != "warn" {
				t.Fatalf("recusa de cliente no nivel %q, quero warn", got.str("level"))
			}
		})
	}
}

// TestSessionHandlers_EmptySessionID_LogsCause: autenticado porem sem Id e'
// 400, com causa propria — a distincao entre "nao sei quem voce e'" e "sei,
// mas nao ha sessao a que isto se refira" tem de sobreviver ate' o log.
func TestSessionHandlers_EmptySessionID_LogsCause(t *testing.T) {
	for _, tc := range sessionCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(nil), tc.method, tc.path, tc.body, "", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			logassert.OutcomeLogged(t, recs, "missing session id")
		})
	}
}

// TestSessionHandlers_NoSessionAppErrReachesClient e' o teste central deste
// arquivo. O erro tipado da porta (`no_session`, categoria validation)
// atravessa o use case — que na F11 deixou de traduzi-lo com fmt.Errorf sem
// %w — e chega a' fronteira ainda tipado. Duas consequencias, ambas
// assertadas: o cliente recebe 400 com error.code estavel, e o log sai em
// WARN, nao em ERROR, porque a falha e' do cliente.
func TestSessionHandlers_NoSessionAppErrReachesClient(t *testing.T) {
	for _, tc := range sessionCases() {
		if tc.name == "Connect" {
			continue // Connect nao consulta SessionGuard
		}
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(noSessionErr()), tc.method, tc.path, tc.body, "user-1", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 — a categoria validation do apperr nao chegou "+
					"a' fronteira (corpo %s)", rec.Code, rec.Body.String())
			}
			errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
			if !ok {
				t.Fatalf("envelope.error nao e' o objeto tipado do ADR-002: %s", rec.Body.String())
			}
			if errObj["code"] != "no_session" {
				t.Fatalf("error.code = %v, quero no_session", errObj["code"])
			}

			got := logassert.OutcomeLogged(t, recs, "no session")
			if got.str("level") != "warn" {
				t.Fatalf("nivel %q para uma falha de CLIENTE (400) — a escolha de nivel "+
					"deixou de ler apperr.Category e todo pedido de QR sem sessao vira alerta",
					got.str("level"))
			}
			if got.str("user_id") != "user-1" {
				t.Fatalf("registro sem user_id correlacionavel: %s", got.Raw)
			}
		})
	}
}

// TestSessionHandlers_InternalFailure_500_LogsError: erro sem taxonomia e'
// 500 e sai em ERROR — o outro lado da escolha de nivel.
func TestSessionHandlers_InternalFailure_500_LogsError(t *testing.T) {
	const boom = "connection refused by whatsmeow store"

	for _, tc := range sessionCases() {
		if tc.name == "Connect" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(errors.New(boom)), tc.method, tc.path, tc.body, "user-1", true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs, boom)
			if got.str("level") != "error" {
				t.Fatalf("falha interna no nivel %q, quero error", got.str("level"))
			}
			if strings.Contains(rec.Body.String(), boom) {
				t.Fatalf("detalhe interno vazou no corpo: %s", rec.Body.String())
			}
		})
	}
}

// TestSessionHandlers_MalformedBody_400_LogsCause: so' PairPhone e
// SetStatusMessage leem corpo, e nos dois o decoder e' a causa.
func TestSessionHandlers_MalformedBody_400_LogsCause(t *testing.T) {
	for _, tc := range sessionCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(nil), tc.method, tc.path, `{"Phone":`, "user-1", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs)
			if got.str("level") != "warn" {
				t.Fatalf("corpo malformado no nivel %q, quero warn", got.str("level"))
			}
		})
	}
}

// TestSessionHandlers_MissingRequiredField_500_LogsError: campo obrigatorio
// ausente hoje vira fmt.Errorf sem taxonomia, e por isso sai 500. O teste
// trava o comportamento OBSERVADO — se um dia esses sitios migrarem para
// apperr de validacao, este teste e' o que avisa que o status mudou.
func TestSessionHandlers_MissingRequiredField_500_LogsError(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		path    string
		want    string
	}{
		{
			name:    "PairPhone sem Phone",
			handler: NewPairPhoneHandler(session.NewPairPhoneUseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{})),
			path:    "/session/pairphone",
			want:    "missing Phone",
		},
		{
			name:    "SetStatusMessage sem Body",
			handler: NewSetStatusMessageHandler(session.NewSetStatusMessageUseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{})),
			path:    "/session/statusmessage",
			want:    "missing Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, recs := serveSession(t, tt.handler, http.MethodPost, tt.path, `{}`, "user-1", true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
			}
			logassert.OutcomeLogged(t, recs, tt.want)
		})
	}
}

// TestSessionHandlers_Success_200 percorre o caminho feliz de cada handler:
// sessao valida, corpo bem formado, 200 e envelope de sucesso. Nenhum log de
// caminho de saida e' esperado — o que se assere e' a ausencia de segredo.
func TestSessionHandlers_Success_200(t *testing.T) {
	for _, tc := range sessionCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, recs := serveSession(t, tc.build(nil), tc.method, tc.path, tc.body, "user-1", true)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
			}
			if env := sessionEnvelope(t, rec); env["success"] != true {
				t.Fatalf("envelope.success = %v numa resposta 200", env["success"])
			}
			logassert.NoSecrets(t, recs)
		})
	}
}

// TestGetQR_ReadsPersistedCode: o QR devolvido e' o que o repositorio tem, e
// nao um GetQRResult vazio — a regressao que a F11 consertou em get_qr.go.
func TestGetQR_ReadsPersistedCode(t *testing.T) {
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "user-1", QRCode: "2@codigo-de-pareamento"}}, nil
		},
	}
	h := NewGetQRHandler(session.NewGetQRUseCase(&contractsfake.SessionGuard{}, users, &contractsfake.Logger{}))

	rec, recs := serveSession(t, h, http.MethodGet, "/session/qr", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	data := sessionEnvelope(t, rec)["data"].(map[string]any)
	if data["QRCode"] != "2@codigo-de-pareamento" && data["qrcode"] != "2@codigo-de-pareamento" {
		t.Fatalf("o QR persistido nao chegou ao cliente: %s", rec.Body.String())
	}
	logassert.NoSecrets(t, recs)
}

// TestGetStatus_ReportsLiveSessionState: connected/loggedIn vem do leitor ao
// vivo, nao de um zero-value. E' a outra metade da correcao da F11.
func TestGetStatus_ReportsLiveSessionState(t *testing.T) {
	status := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "user-1", JID: "5511@s.whatsapp.net"}}, nil
		},
	}
	h := NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, status, users, &contractsfake.Logger{}))

	rec, recs := serveSession(t, h, http.MethodGet, "/session/status", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "5511@s.whatsapp.net") {
		t.Fatalf("o registro persistido nao chegou ao cliente: %s", rec.Body.String())
	}
	logassert.NoSecrets(t, recs)
}

// TestGetQRAndStatus_NoUserRecord_400_NoSession: repositorio sem registro
// para a sessao produz o apperr `no_session`, que a fronteira traduz em 400
// — nao o 500 que um erro sem taxonomia daria.
func TestGetQRAndStatus_NoUserRecord_400_NoSession(t *testing.T) {
	empty := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) { return nil, nil },
	}
	log := &contractsfake.Logger{}

	tests := []struct {
		name    string
		handler http.Handler
		path    string
	}{
		{
			name:    "GetQR",
			handler: NewGetQRHandler(session.NewGetQRUseCase(&contractsfake.SessionGuard{}, empty, log)),
			path:    "/session/qr",
		},
		{
			name:    "GetStatus",
			handler: NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, &contractsfake.SessionStatusReader{}, empty, log)),
			path:    "/session/status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, recs := serveSession(t, tt.handler, http.MethodGet, tt.path, "", "user-1", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			errObj := sessionEnvelope(t, rec)["error"].(map[string]any)
			if errObj["code"] != "no_session" {
				t.Fatalf("error.code = %v, quero no_session", errObj["code"])
			}
			got := logassert.OutcomeLogged(t, recs, "no session")
			if got.str("level") != "warn" {
				t.Fatalf("nivel %q para 400 de cliente, quero warn", got.str("level"))
			}
		})
	}
}

// TestGetQRAndStatus_RepositoryFailure_500_LogsError: falha do repositorio
// e' embrulhada em "database error" SEM taxonomia — 500 e nivel error.
func TestGetQRAndStatus_RepositoryFailure_500_LogsError(t *testing.T) {
	const boom = "pq: too many connections"
	broken := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, errors.New(boom)
		},
	}
	log := &contractsfake.Logger{}

	tests := []struct {
		name    string
		handler http.Handler
		path    string
	}{
		{
			name:    "GetQR",
			handler: NewGetQRHandler(session.NewGetQRUseCase(&contractsfake.SessionGuard{}, broken, log)),
			path:    "/session/qr",
		},
		{
			name:    "GetStatus",
			handler: NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, &contractsfake.SessionStatusReader{}, broken, log)),
			path:    "/session/status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, recs := serveSession(t, tt.handler, http.MethodGet, tt.path, "", "user-1", true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs, boom)
			if got.str("level") != "error" {
				t.Fatalf("falha de repositorio no nivel %q, quero error", got.str("level"))
			}
			if strings.Contains(rec.Body.String(), boom) {
				t.Fatalf("detalhe do banco vazou no corpo: %s", rec.Body.String())
			}
		})
	}
}

// TestSessionUser_WrongTypeInContext_401: o contexto carrega `any`. Um valor
// que nao satisfaz userInfo tem de virar 401 com causa — nao panico, e nao
// seguir com ID vazio.
func TestSessionUser_WrongTypeInContext_401(t *testing.T) {
	h := NewLogoutHandler(session.NewLogoutUseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{}))
	wrapped, capture := logassert.Wrap(h)

	req := httptest.NewRequest(http.MethodPost, "/session/logout", nil)
	req = req.WithContext(context.WithValue(req.Context(), appport.UserInfoKey, 42))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, quero 401", rec.Code)
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unauthorized")
}

// TestConnectHandler_StartsClientOnce: o unico handler com efeito colateral
// proprio. WithStartClient injeta o lancador; ConnectHandler responde 200 e
// dispara o cliente em background exatamente uma vez.
func TestConnectHandler_StartsClientOnce(t *testing.T) {
	started := make(chan string, 2)
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{})).
		WithStartClient(func(userID, jid, token string, kill chan bool) { started <- userID })

	rec, recs := serveSession(t, h, http.MethodPost, "/session/connect", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if got := <-started; got != "user-1" {
		t.Fatalf("StartClient recebeu %q, quero user-1", got)
	}
	select {
	case extra := <-started:
		t.Fatalf("StartClient chamado mais de uma vez (segunda: %q)", extra)
	default:
	}
	logassert.NoSecrets(t, recs)
}

// TestConnectHandler_WithoutStartClient_StillResponds: sem lancador injetado
// (o zero-value do handler, que e' o que bootstrap monta antes do wiring) a
// rota nao pode entrar em panico com um nil call.
func TestConnectHandler_WithoutStartClient_StillResponds(t *testing.T) {
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}))

	rec, recs := serveSession(t, h, http.MethodPost, "/session/connect", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	logassert.NoSecrets(t, recs)
}
