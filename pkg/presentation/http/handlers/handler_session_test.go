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
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/pairing"
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

// noSessionErr e' o erro tipado que a porta wa-noise produz quando nao ha
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

// sessionCaseRegistry monta o registry de pareamento para esta tabela: a
// sessao autenticada esta' gravada em wa_noise, o provider passado serve esse
// engine, e o wa_headless fica registado e vazio. A matriz e' a de PRODUCAO —
// um duble permissivo abencoaria caminhos que nao existem (ARMADILHAS.md #1).
func sessionCaseRegistry(waNoise *pairing.Provider) *pairing.Registry {
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(_ context.Context, id string) ([]domain.UserListEntry, error) {
			if id == "" {
				return nil, nil
			}
			return []domain.UserListEntry{{ID: id, Engine: domain.EngineWaNoise, QRCode: "qr-data"}}, nil
		},
	}
	return pairing.NewRegistry(users, capabilityregistry.NewCapabilityRegistry(),
		waNoise, &pairing.Provider{Engine: domain.EngineWaHeadless})
}

func sessionCases() []sessionCase {
	log := &contractsfake.Logger{}
	guard := func(err error) *contractsfake.SessionGuard {
		g := contractsfake.FailSession(err)
		return &g
	}
	// Disconnect e Logout consomem SessionController desde a F79: precisam
	// AGIR sobre a sessão, não só verificá-la. Aqui só importa a recusa da
	// guarda propagar, então o controller nasce com a mesma FailSession.
	ctl := func(err error) *contractsfake.SessionController {
		return &contractsfake.SessionController{SessionGuard: contractsfake.FailSession(err)}
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
			build:  func(e error) http.Handler { return NewDisconnectHandler(session.NewDisconnectUseCase(ctl(e), log)) },
			method: http.MethodPost,
			path:   "/session/disconnect",
		},
		{
			name: "Logout",
			build: func(e error) http.Handler {
				return NewLogoutHandler(session.NewLogoutUseCase(ctl(e), &contractsfake.SessionDetacher{}, log))
			},
			method: http.MethodPost,
			path:   "/session/logout",
		},
		{
			// GetQR passou a resolver a porta por engine (F281). O provider
			// wa_noise e' um PairingQRReader cuja guarda falha com `e`, que e'
			// o eixo que esta tabela mede; a matriz consultada e' a REAL.
			name: "GetQR",
			build: func(e error) http.Handler {
				return NewGetQRHandler(log, sessionCaseRegistry(&pairing.Provider{
					Engine: domain.EngineWaNoise,
					QRReader: &contractsfake.PairingQRReader{
						SessionGuard:  *guard(e),
						PairingQRFunc: func(context.Context, string) (string, error) { return "qr-data", nil },
					},
				}))
			},
			method: http.MethodGet,
			path:   "/session/qr?engine=wa_noise",
		},
		{
			name: "GetStatus",
			build: func(e error) http.Handler {
				// F196: GetStatus não consulta o SessionGuard. O `e` deste
				// caso passa a ser injetado onde ele AINDA pode falhar — a
				// leitura do registo — para que os testes de falha interna
				// continuem a exercitar um caminho real em vez de nenhum.
				u := users()
				if e != nil {
					u.ListUsersFunc = func(context.Context, string) ([]domain.UserListEntry, error) {
						return nil, e
					}
				}
				return NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, u, log))
			},
			method: http.MethodGet,
			path:   "/session/status",
		},
		{
			name: "RequestHistorySync",
			build: func(e error) http.Handler {
				return NewRequestHistorySyncHandler(session.NewRequestHistorySyncUseCase(&contractsfake.HistorySyncRequester{SessionGuard: *guard(e)}, log))
			},
			method: http.MethodPost,
			path:   "/session/historysync",
			// F198: a rota deixou de aceitar corpo vazio. Antes respondia 200
			// sem pedir nada; agora a âncora é obrigatória, porque o protocolo
			// pede "as N mensagens ANTES desta" e sem ela não há pedido.
			body: `{"chat_jid":"5511999999999@s.whatsapp.net","oldest_msg_id":"MSG1","count":10}`,
		},
		{
			name: "SyncContactRoster",
			build: func(e error) http.Handler {
				as := &contractsfake.AppStateSyncer{SessionGuard: contractsfake.FailSession(e)}
				return NewSyncContactRosterHandler(session.NewSyncContactRosterUseCase(as, log))
			},
			method:    http.MethodPost,
			path:      "/user/contacts/sync",
			body:      `{"mode":"incremental"}`,
			readsBody: true,
		},
		{
			name: "PairPhone",
			build: func(e error) http.Handler {
				// PairPhone consome PhonePairer desde o CAP-26: precisa PEDIR
				// o código, não só verificar a sessão. Aqui só importa a
				// recusa da guarda propagar, então o pairer nasce com a mesma
				// FailSession.
				return NewPairPhoneHandler(log, sessionCaseRegistry(&pairing.Provider{
					Engine:      domain.EngineWaNoise,
					PhonePairer: &contractsfake.PhonePairer{SessionGuard: contractsfake.FailSession(e)},
				}))
			},
			method:    http.MethodPost,
			path:      "/session/pairphone",
			body:      `{"engine":"wa_noise","Phone":"5511999999999"}`,
			readsBody: true,
		},
		{
			name: "SetStatusMessage",
			build: func(e error) http.Handler {
				return NewSetStatusMessageHandler(session.NewSetStatusMessageUseCase(&contractsfake.StatusMessageSetter{SessionGuard: *guard(e)}, log))
			},
			method:    http.MethodPost,
			path:      "/session/statusmessage",
			body:      `{"Body":"disponivel"}`,
			readsBody: true,
		},
		{
			name: "Connect",
			build: func(error) http.Handler {
				return NewConnectHandler(session.NewConnectUseCase(log), sessionCaseRegistry(&pairing.Provider{
					Engine:  domain.EngineWaNoise,
					Starter: &contractsfake.SessionStarter{},
				}))
			},
			method: http.MethodPost,
			path:   "/session/connect?engine=wa_noise",
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
		if tc.name == "GetStatus" {
			// F196: GetStatus deixou de consultar o SessionGuard de propósito.
			// Perguntar o estado de uma sessão desconectada devolvia 400 "no
			// session" — a resposta mais inútil possível no único momento em
			// que alguém pergunta. Agora devolve 200 com connected:false, e
			// isso é asserido em TestGetStatus_DesconectadaDevolve200.
			continue
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
	const boom = "connection refused by wanoise store"

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
// 400 desde a F66: campo obrigatorio ausente ou valor invalido no payload e
// erro do CLIENTE. Estes testes exigiam 500 — dois com "_500" no proprio
// nome —, fixando o defeito que a F66 corrige.
func TestSessionHandlers_MissingRequiredField_400_LogsError(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		path    string
		want    string
		// body substitui o `{}` partilhado quando o handler exige um campo
		// ANTES do que este teste mede. PairPhone e' o caso: desde a F281 um
		// `{}` seco morre em invalid_engine e nunca chega ao missing_phone.
		body string
	}{
		{
			name: "PairPhone sem Phone",
			body: `{"engine":"wa_noise"}`,
			handler: NewPairPhoneHandler(&contractsfake.Logger{}, sessionCaseRegistry(&pairing.Provider{
				Engine: domain.EngineWaNoise, PhonePairer: &contractsfake.PhonePairer{},
			})),
			path: "/session/pairphone",
			want: "missing Phone",
		},
		{
			name:    "RequestHistorySync sem âncora",
			handler: NewRequestHistorySyncHandler(session.NewRequestHistorySyncUseCase(&contractsfake.HistorySyncRequester{}, &contractsfake.Logger{})),
			path:    "/session/historysync",
			want:    "chat_jid é obrigatório",
		},
		{
			name:    "SetStatusMessage sem Body",
			handler: NewSetStatusMessageHandler(session.NewSetStatusMessageUseCase(&contractsfake.StatusMessageSetter{}, &contractsfake.Logger{})),
			path:    "/session/statusmessage",
			want:    "missing Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if body == "" {
				body = `{}`
			}
			rec, recs := serveSession(t, tt.handler, http.MethodPost, tt.path, body, "user-1", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			logassert.OutcomeLogged(t, recs, tt.want)
		})
	}
}

// TestSyncContactRoster_InvalidMode_400: modo fora dos tres aceitos e'
// recusado com um *apperr.AppError de categoria validation, e por isso 400 —
// diferente de PairPhone/SetStatusMessage (TestSessionHandlers_
// MissingRequiredField_500_LogsError), cuja validacao de payload nao usa a
// taxonomia apperr e por isso ainda vira 500.
func TestSyncContactRoster_InvalidMode_400(t *testing.T) {
	log := &contractsfake.Logger{}
	h := NewSyncContactRosterHandler(session.NewSyncContactRosterUseCase(&contractsfake.AppStateSyncer{}, log))

	rec, recs := serveSession(t, h, http.MethodPost, "/user/contacts/sync", `{"mode":"bogus"}`, "user-1", true)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
	}
	errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.error nao e' o objeto tipado do ADR-002: %s", rec.Body.String())
	}
	if errObj["code"] != "invalid_sync_mode" {
		t.Fatalf("error.code = %v, quero invalid_sync_mode", errObj["code"])
	}
	got := logassert.OutcomeLogged(t, recs, "mode must be one of")
	if got.str("level") != "warn" {
		t.Fatalf("nivel %q para uma falha de CLIENTE (400), quero warn", got.str("level"))
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
			// A coluna guarda a IMAGEM, nao o codigo cru: e' o que o listener
			// de QR do wa_noise escreve (pkg/application/session/
			// orchestrator.go, onPairingQR — "A coluna guarda a IMAGEM").
			// Semear "2@..." aqui divergia da producao na exata regra em
			// causa, e foi a F373 que o mediu contra servidor vivo.
			return []domain.UserListEntry{{ID: "user-1", QRCode: qrImageOf(t, qrCodePersistido)}}, nil
		},
	}
	h := NewGetQRHandler(&contractsfake.Logger{}, sessionCaseRegistry(&pairing.Provider{
		Engine: domain.EngineWaNoise,
		QRReader: &contractsfake.PairingQRReader{
			PairingQRFunc: func(ctx context.Context, txtID string) (string, error) {
				// Imita a REGRA do adaptador de producao
				// (pkg/infra/wa-noise/adapters/pairing/qr.go): le' o registo
				// persistido e devolve entries[0].QRCode.
				entries, err := users.ListUsers(ctx, txtID)
				if err != nil {
					return "", err
				}
				if len(entries) == 0 {
					return "", noSessionErr()
				}
				return entries[0].QRCode, nil
			},
		},
	}))

	rec, recs := serveSession(t, h, http.MethodGet, "/session/qr?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	data := sessionEnvelope(t, rec)["data"].(map[string]any)
	// A chave e' `qr_code` desde a migracao para DTO
	// (docs/HTTP-DTO-CONVENTIONS.md). Era `QRCode` — o nome do campo Go — e o
	// teste tolerava as duas grafias; hoje afirma UMA, que e' o que faz a
	// grafia antiga voltar a falhar aqui.
	if data["qr_code"] != qrImageOf(t, qrCodePersistido) {
		t.Fatalf("o QR persistido nao chegou ao cliente INTACTO (recodifica-lo faz o cliente "+
			"desenhar o proprio data URI — F373): %s", rec.Body.String())
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
	h := NewGetStatusHandler(session.NewGetStatusUseCase(status, users, &contractsfake.Logger{}))

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
			// O registo ausente e' agora detectado ANTES do use case: o
			// registry le' a sessao alvo para resolver o engine e recusa com
			// o MESMO codigo `no_session` (pkg/pairing/errors.go), por isso o
			// contrato observado nao muda.
			name:    "GetQR",
			handler: NewGetQRHandler(log, pairing.NewRegistry(empty, capabilityregistry.NewCapabilityRegistry(), &pairing.Provider{Engine: domain.EngineWaNoise, QRReader: &contractsfake.PairingQRReader{}}, &pairing.Provider{Engine: domain.EngineWaHeadless})),
			path:    "/session/qr?engine=wa_noise",
		},
		{
			name:    "GetStatus",
			handler: NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, empty, log)),
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
			// A leitura do registo passou para o registry, que a faz para
			// resolver o engine do ALVO. A falha do banco continua a sair 500
			// em nivel error — respondPairingRefusal usa a mesma taxonomia.
			name:    "GetQR",
			handler: NewGetQRHandler(log, pairing.NewRegistry(broken, capabilityregistry.NewCapabilityRegistry(), &pairing.Provider{Engine: domain.EngineWaNoise, QRReader: &contractsfake.PairingQRReader{}}, &pairing.Provider{Engine: domain.EngineWaHeadless})),
			path:    "/session/qr?engine=wa_noise",
		},
		{
			name:    "GetStatus",
			handler: NewGetStatusHandler(session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, broken, log)),
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
	h := NewLogoutHandler(session.NewLogoutUseCase(&contractsfake.SessionController{}, &contractsfake.SessionDetacher{}, &contractsfake.Logger{}))
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

// connectStarterHandler monta o ConnectHandler sobre um SessionStarter fake
// registado como provider do wa_noise. A sessao "user-1" esta' gravada nesse
// engine, e a matriz consultada e' a de PRODUCAO.
func connectStarterHandler(starter *contractsfake.SessionStarter) http.Handler {
	return NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise, Starter: starter}))
}

// TestConnectHandler_StartsClientOnce: o unico handler com efeito colateral
// proprio. ConnectHandler responde 200 e dispara a sessao exatamente uma vez,
// propagando o token do userinfo autenticado (HOUSEKEEP.md: antes ia sempre
// vazio).
func TestConnectHandler_StartsClientOnce(t *testing.T) {
	starter := &contractsfake.SessionStarter{}
	rec, recs := serveSession(t, connectStarterHandler(starter), http.MethodPost,
		"/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if len(starter.StartSessionCalls) != 1 {
		t.Fatalf("StartSession chamado %d vez(es), quero 1: %+v", len(starter.StartSessionCalls), starter.StartSessionCalls)
	}
	got := starter.StartSessionCalls[0]
	if got.TxtID != "user-1" {
		t.Fatalf("StartSession recebeu txtID %q, quero user-1", got.TxtID)
	}
	if got.Token != logassertAdminToken {
		t.Fatalf("StartSession recebeu token %q, quero o token do userinfo autenticado", got.Token)
	}
	logassert.NoSecrets(t, recs)
}

// TestConnectHandler_MissingEngine_400: o sucessor de
// TestConnectHandler_WithoutStartSession_StillResponds.
//
// Aquele teste protegia um zero-value do handler em que StartSession era nil e
// a rota tinha de responder 200 mesmo assim. Esse estado deixou de existir: o
// lancador vem do provider que o registry resolve, e nao ha resolucao sem
// engine. O que sobra a proteger e' que a AUSENCIA do engine e' recusa
// explicita e nao panico com um nil.
func TestConnectHandler_MissingEngine_400(t *testing.T) {
	starter := &contractsfake.SessionStarter{}
	rec, _ := serveSession(t, connectStarterHandler(starter), http.MethodPost,
		"/session/connect", "", "user-1", true)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
	}
	if len(starter.StartSessionCalls) != 0 || len(starter.CheckOwnershipCalls) != 0 {
		t.Fatalf("pedido sem engine alcancou o provider: %+v / %+v", starter.CheckOwnershipCalls, starter.StartSessionCalls)
	}
}

// TestConnectHandler_NilStarter_EngineUnavailable: um provider registado cujo
// Starter e' nil nao pode virar panico nem 200 silencioso. E' a metade
// `engine_unavailable` de pkg/pairing, e e' o unico caminho que a distingue de
// `capability_not_supported`.
func TestConnectHandler_NilStarter_EngineUnavailable(t *testing.T) {
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise}))

	rec, _ := serveSession(t, h, http.MethodGet, "/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, quero 409 (corpo %s)", rec.Code, rec.Body.String())
	}
	errObj := sessionEnvelope(t, rec)["error"].(map[string]any)
	if errObj["code"] != pairing.CodeEngineUnavailable {
		t.Fatalf("error.code = %v, quero %q", errObj["code"], pairing.CodeEngineUnavailable)
	}
}

// F108: ownership denied must reach the client as 409, not as 200.
//
// Before this fix, the handler fired startSession in a goroutine and
// responded 200 "connecting" without knowing whether ownership was denied.
// The client received success for a request that would never connect.

// TestConnectHandler_OwnershipDenied_409: when CheckOwnership rejects, the
// handler responds 409 with the classified error — not 200 — and StartSession
// is never reached. A ORDEM e' o que se trava: inverter as duas chamadas
// devolve 409 na mesma, e so' a contagem de StartSession o revela.
func TestConnectHandler_OwnershipDenied_409(t *testing.T) {
	ownershipErr := apperr.New(
		"session_owned_by_another_replica",
		apperr.CategoryConflict,
		"this session is owned by another replica; route the request to its owner",
		false,
		nil,
	)
	starter := &contractsfake.SessionStarter{
		CheckOwnershipFunc: func(context.Context, string) error { return ownershipErr },
	}

	rec, recs := serveSession(t, connectStarterHandler(starter), http.MethodGet,
		"/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409 — CategoryConflict must reach the HTTP boundary (body: %s)",
			rec.Code, rec.Body.String())
	}
	if len(starter.StartSessionCalls) != 0 {
		t.Fatalf("StartSession chamado %d vez(es) com a posse negada", len(starter.StartSessionCalls))
	}
	errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.error is not the typed object from ADR-002: %s", rec.Body.String())
	}
	if errObj["code"] != "session_owned_by_another_replica" {
		t.Fatalf("error.code = %v, want session_owned_by_another_replica", errObj["code"])
	}
	got := logassert.OutcomeLogged(t, recs, "owned by another replica")
	if got.str("level") != "warn" {
		t.Fatalf("level %q for a 409 (client-side), want warn", got.str("level"))
	}
	if got.str("user_id") != "user-1" {
		t.Fatalf("log record without correlatable user_id: %s", got.Raw)
	}
}

// TestConnectHandler_OwnershipGranted_200: when CheckOwnership passes, the
// handler proceeds normally — the check must not block the happy path.
func TestConnectHandler_OwnershipGranted_200(t *testing.T) {
	starter := &contractsfake.SessionStarter{}

	rec, _ := serveSession(t, connectStarterHandler(starter), http.MethodGet,
		"/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(starter.CheckOwnershipCalls) != 1 {
		t.Fatalf("CheckOwnership calls = %d, want 1", len(starter.CheckOwnershipCalls))
	}
	if len(starter.StartSessionCalls) != 1 || starter.StartSessionCalls[0].TxtID != "user-1" {
		t.Fatalf("StartSession: %+v", starter.StartSessionCalls)
	}
}

// F274: a start already in flight for this user must reach the client as
// 409, not as 200.
//
// Before this fix, the in-flight guard lived entirely inside Start, which
// ran in a goroutine fired AFTER the handler already responded 200. A client
// hitting GET /session/connect while a previous pairing flow was still
// active — including right after /session/disconnect, which cut the
// transport but not the flow — got 200 {"status":"connecting"} for an
// attempt that never started.

// TestConnectHandler_StartInFlight_409: when CheckStartInFlight rejects, the
// handler responds 409 with the classified error and never fires
// StartSession.
func TestConnectHandler_StartInFlight_409(t *testing.T) {
	flightErr := apperr.New(
		"session_start_already_in_flight",
		apperr.CategoryConflict,
		"a session start is already in flight for this user; read the current QR from GET /session/qr",
		false,
		nil,
	)
	starter := &contractsfake.SessionStarter{
		StartSessionFunc: func(context.Context, string, string) {
			t.Fatal("StartSession must not be called when a start is already in flight")
		},
	}
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise, Starter: starter})).
		WithCheckStartInFlight(func(string) error { return flightErr })

	rec, recs := serveSession(t, h, http.MethodGet, "/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409 — CategoryConflict must reach the HTTP boundary (body: %s)",
			rec.Code, rec.Body.String())
	}
	errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.error is not the typed object from ADR-002: %s", rec.Body.String())
	}
	if errObj["code"] != "session_start_already_in_flight" {
		t.Fatalf("error.code = %v, want session_start_already_in_flight", errObj["code"])
	}
	got := logassert.OutcomeLogged(t, recs, "already in flight")
	if got.str("level") != "warn" {
		t.Fatalf("level %q for a 409 (client-side), want warn", got.str("level"))
	}
	if got.str("user_id") != "user-1" {
		t.Fatalf("log record without correlatable user_id: %s", got.Raw)
	}
}

// TestConnectHandler_StartInFlight_CheckedBeforeOwnership: the in-flight
// check runs BEFORE the ownership check, mirroring the order Start itself
// uses internally (inFlight.acquire before claimOwnership). If ownership ran
// first, a start-in-flight user owned by another replica would be
// misclassified as an ownership problem instead of the actual cause.
func TestConnectHandler_StartInFlight_CheckedBeforeOwnership(t *testing.T) {
	flightErr := apperr.New(
		"session_start_already_in_flight",
		apperr.CategoryConflict,
		"a session start is already in flight for this user; read the current QR from GET /session/qr",
		false,
		nil,
	)
	starter := &contractsfake.SessionStarter{
		CheckOwnershipFunc: func(context.Context, string) error {
			t.Fatal("CheckOwnership must not run when the in-flight check already rejected")
			return nil
		},
		StartSessionFunc: func(context.Context, string, string) { t.Fatal("StartSession must not be called") },
	}
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise, Starter: starter})).
		WithCheckStartInFlight(func(string) error { return flightErr })

	rec, _ := serveSession(t, h, http.MethodGet, "/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
	errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
	if !ok || errObj["code"] != "session_start_already_in_flight" {
		t.Fatalf("error = %v, want session_start_already_in_flight", sessionEnvelope(t, rec)["error"])
	}
}

// TestConnectHandler_StartAvailable_200: when CheckStartInFlight passes, the
// handler proceeds normally — the check must not block the happy path.
func TestConnectHandler_StartAvailable_200(t *testing.T) {
	started := make(chan string, 1)
	starter := &contractsfake.SessionStarter{
		StartSessionFunc: func(_ context.Context, userID, _ string) { started <- userID },
	}
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise, Starter: starter})).
		WithCheckStartInFlight(func(string) error { return nil })

	rec, _ := serveSession(t, h, http.MethodGet, "/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	uid := <-started
	if uid != "user-1" {
		t.Fatalf("StartSession received userID %q, want user-1", uid)
	}
}

// TestConnectHandler_WithoutCheckStartInFlight_200: without the check
// installed (zero-value handler, mirrors bootstrap wiring before F274), the
// handler behaves exactly as before — it must not panic on a nil func.
func TestConnectHandler_WithoutCheckStartInFlight_200(t *testing.T) {
	started := make(chan string, 1)
	starter := &contractsfake.SessionStarter{
		StartSessionFunc: func(_ context.Context, userID, _ string) { started <- userID },
	}
	h := NewConnectHandler(session.NewConnectUseCase(&contractsfake.Logger{}),
		sessionCaseRegistry(&pairing.Provider{Engine: domain.EngineWaNoise, Starter: starter}))

	rec, _ := serveSession(t, h, http.MethodGet, "/session/connect?engine=wa_noise", "", "user-1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	uid := <-started
	if uid != "user-1" {
		t.Fatalf("StartSession received userID %q, want user-1", uid)
	}
}

// F275 — POST /session/logout on a connected-but-never-paired session
// responded 500 with a plain-text envelope instead of the canonical
// {code,error:{code,message}} shape at 409. The fix lives at the SDK
// boundary (pkg/infra/wa-noise/runtime/session/guard.go's Logout, tested in
// that package) and flows through the use case unchanged; this test proves
// the HTTP boundary — status AND envelope shape — through the REGISTERED
// handler, not just the use case.

// TestLogoutHandler_ConectadaSemPareamento_409ComEnvelopeCanonico is the
// defect test: before the fix, the raw wanoise.ErrNotLoggedIn was not a
// *apperr.AppError, so RespondJSON fell into the untyped branch — genericError
// — which HOUSEKEEP F275 measured (against an older build) as a 500 with
// `error` as a bare string. Even against the CURRENT RespondJSON (which
// always emits an object), the untyped fallback still means the WRONG
// status (500) and the WRONG code ("internal_error" instead of
// "session_not_paired") — a client cannot branch on it.
func TestLogoutHandler_ConectadaSemPareamento_409ComEnvelopeCanonico(t *testing.T) {
	semPar := apperr.New(
		apperr.CodeSessionNotPaired,
		apperr.CategoryConflict,
		"session has a live connection but was never paired; there is no device to log out",
		false,
		nil,
	)
	sc := &contractsfake.SessionController{
		LogoutFunc: func(context.Context, string) error { return semPar },
	}
	h := NewLogoutHandler(session.NewLogoutUseCase(sc, &contractsfake.SessionDetacher{}, &contractsfake.Logger{}))

	rec, recs := serveSession(t, h, http.MethodPost, "/session/logout", "", "user-1", true)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409 — session_not_paired is CategoryConflict, must reach the HTTP boundary as such (body: %s)",
			rec.Code, rec.Body.String())
	}

	env := sessionEnvelope(t, rec)
	if success, _ := env["success"].(bool); success {
		t.Fatalf("envelope.success = true numa resposta de erro: %s", rec.Body.String())
	}
	if code, _ := env["code"].(float64); int(code) != http.StatusConflict {
		t.Fatalf("envelope.code = %v, want 409: %s", env["code"], rec.Body.String())
	}
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope.error is not the typed {code,message} object — it is %T: %s", env["error"], rec.Body.String())
	}
	if errObj["code"] != "session_not_paired" {
		t.Fatalf("error.code = %v, want session_not_paired", errObj["code"])
	}
	if _, hasMessage := errObj["message"].(string); !hasMessage {
		t.Fatalf("error.message ausente ou não é string: %v", errObj["message"])
	}
	// success: false, code: 409, data ausente, error: {code,message} — a
	// forma canônica do CONTRATO-ARQUITETURAL, não success/code/data.
	if _, hasData := env["data"]; hasData {
		t.Fatalf("envelope de erro carrega `data`: %s", rec.Body.String())
	}

	got := logassert.OutcomeLogged(t, recs, "never paired")
	if got.str("level") != "warn" {
		t.Fatalf("level %q for a 409 (client-side), want warn", got.str("level"))
	}
}

// TestSyncContactRoster_AusenteVsInvalido_PelaRota exercita os TRÊS corpos
// JSON que em Go colapsam no mesmo valor — campo omitido, null e "" — pela
// rota registrada, e confirma que os três dão o código de campo em falta,
// distinto do código de valor inválido.
//
// É pela rota e não pelo use case porque só o decode distingue os três
// corpos; um teste no use case só consegue exercitar Mode: "".
func TestSyncContactRoster_AusenteVsInvalido_PelaRota(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
		// wantMsgPart, quando não vazio, é o eco do valor recebido no corpo
		// da resposta: é o que permite ao consumidor ver o próprio erro de
		// digitação.
		wantMsgPart string
	}{
		{"campo omitido", `{}`, "missing_sync_mode", ""},
		{"campo null", `{"mode":null}`, "missing_sync_mode", ""},
		{"campo vazio", `{"mode":""}`, "missing_sync_mode", ""},
		{"valor desconhecido", `{"mode":"xpto"}`, "invalid_sync_mode", `xpto`},
		// Maiúsculas continuam INVÁLIDAS — a comparação é sensível a caixa.
		{"valor em maiusculas", `{"mode":"FULL"}`, "invalid_sync_mode", `FULL`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := &contractsfake.AppStateSyncer{}
			log := &contractsfake.Logger{}
			h := NewSyncContactRosterHandler(session.NewSyncContactRosterUseCase(as, log))

			rec, _ := serveSession(t, h, http.MethodPost, "/user/contacts/sync", tt.body, "user-1", true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			errObj, ok := sessionEnvelope(t, rec)["error"].(map[string]any)
			if !ok {
				t.Fatalf("envelope.error nao e' o objeto tipado do ADR-002: %s", rec.Body.String())
			}
			if errObj["code"] != tt.wantCode {
				t.Fatalf("error.code = %v, quero %v (corpo %s)", errObj["code"], tt.wantCode, rec.Body.String())
			}
			if tt.wantMsgPart != "" {
				msg, _ := errObj["message"].(string)
				if !strings.Contains(msg, tt.wantMsgPart) {
					t.Errorf("error.message = %q nao ecoa o valor recebido %q", msg, tt.wantMsgPart)
				}
			}
			if len(as.SyncContactRosterCalls) != 0 {
				t.Errorf("corpo recusado alcancou a porta: %+v", as.SyncContactRosterCalls)
			}
		})
	}
}

// TestSyncContactRoster_ValoresValidos_200_PelaRota é o caminho de SUCESSO:
// sem ele, uma regra estrita demais (por exemplo, recusar tudo) passaria
// despercebida por TestSyncContactRoster_AusenteVsInvalido_PelaRota.
func TestSyncContactRoster_ValoresValidos_200_PelaRota(t *testing.T) {
	for _, mode := range []string{"if_unsynced", "incremental", "full"} {
		t.Run(mode, func(t *testing.T) {
			as := &contractsfake.AppStateSyncer{}
			log := &contractsfake.Logger{}
			h := NewSyncContactRosterHandler(session.NewSyncContactRosterUseCase(as, log))

			rec, _ := serveSession(t, h, http.MethodPost, "/user/contacts/sync",
				`{"mode":"`+mode+`"}`, "user-1", true)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
			}
			if len(as.SyncContactRosterCalls) != 1 || as.SyncContactRosterCalls[0].Mode != mode {
				t.Fatalf("porta recebeu %+v, quero uma chamada com mode=%q", as.SyncContactRosterCalls, mode)
			}
		})
	}
}
