package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	_ "modernc.org/sqlite"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// Fase 12 — os oito handlers de miscelanea: /health, /newsletter/list,
// DELETE /admin/users/{id}/complete, /chat/rejectcall, GET e POST
// /user/privacy, /chat/requestunavailablemessage e /chat/archive.
//
// Sao os handlers menos parecidos entre si do pacote — dois falam com o banco
// direto, um le variavel de rota do mux, cinco decodificam corpo. O que os une
// e' o que a fase mede: cada resposta >=400 tem de sair com a causa no log.

// ipmSQLite abre um banco de arquivo vazio em t.TempDir(), sem CGO e sem
// Docker (mesmo padrao de pkg/infra/db/migrations_test.go). Sem tabela
// `users`, e' o banco que faz o caminho de erro morder.
func ipmSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "wa.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// --- /health -----------------------------------------------------------

func TestGetHealthHandler_Success(t *testing.T) {
	uc := notification.NewGetHealthUseCase(ipmSQLite(t), &contractsfake.SessionCounter{}, &contractsfake.Logger{}, "test")

	rec, recs := ipmServe(t, NewGetHealthHandler(uc), http.MethodGet, "/health", "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	ipmAssertNoOutcomeLog(t, recs)
}

// TestGetHealthHandler_CounterFailure: /health e' a rota que o operador usa
// para saber se o servico esta' de pe'. Um 500 dela sem causa no log e' o pior
// caso do Cenario 2.
func TestGetHealthHandler_CounterFailure(t *testing.T) {
	counter := &contractsfake.SessionCounter{
		CountSessionsFunc: func(context.Context) (domain.SessionCounts, error) {
			return domain.SessionCounts{}, ipmErrBoom
		},
	}
	uc := notification.NewGetHealthUseCase(ipmSQLite(t), counter, &contractsfake.Logger{}, "test")

	rec, recs := ipmServe(t, NewGetHealthHandler(uc), http.MethodGet, "/health", "", nil)

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
}

// --- /newsletter/list --------------------------------------------------

func newsletterHandler(nr *contractsfake.NewsletterReader) http.Handler {
	return NewListNewsletterHandler(notification.NewListNewsletterUseCase(nr, &contractsfake.Logger{}))
}

func TestListNewsletterHandler_Success(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, recs := ipmServe(t, newsletterHandler(nr), http.MethodGet, "/newsletter/list", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if len(nr.ListSubscribedCalls) != 1 {
		t.Fatalf("ListSubscribed chamado %d vez(es), quero 1", len(nr.ListSubscribedCalls))
	}
	ipmAssertNoOutcomeLog(t, recs)
}

func TestListNewsletterHandler_Unauthorized(t *testing.T) {
	nr := &contractsfake.NewsletterReader{}

	rec, _ := ipmServe(t, newsletterHandler(nr), http.MethodGet, "/newsletter/list", "", nil)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if len(nr.EnsureSessionCalls) != 0 {
		t.Fatal("requisicao nao autenticada alcancou a porta")
	}
}

func TestListNewsletterHandler_SessionFailure(t *testing.T) {
	nr := &contractsfake.NewsletterReader{SessionGuard: contractsfake.FailSession(ipmErrBoom)}

	rec, recs := ipmServe(t, newsletterHandler(nr), http.MethodGet, "/newsletter/list", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
}

func TestListNewsletterHandler_ListFailure(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		ListSubscribedFunc: func(context.Context, string) (any, error) { return nil, ipmErrBoom },
	}

	rec, recs := ipmServe(t, newsletterHandler(nr), http.MethodGet, "/newsletter/list", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
}

// --- DELETE /admin/users/{id}/complete ---------------------------------

func deleteUserCompleteHandler(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	uc := user.NewDeleteUserCompleteUseCase(db, &contractsfake.SessionController{}, &contractsfake.Logger{}, t.TempDir())
	return NewDeleteUserCompleteHandler(uc)
}

// TestDeleteUserCompleteHandler_MissingID exercita o unico 4xx do arquivo que
// nasce de variavel de rota: sem o mux no caminho, mux.Vars devolve mapa
// vazio, que e' exatamente o que uma rota mal registrada produziria.
func TestDeleteUserCompleteHandler_MissingID(t *testing.T) {
	rec, recs := ipmServe(t, deleteUserCompleteHandler(t, ipmSQLite(t)),
		http.MethodDelete, "/admin/users//complete", "", nil)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, recs, "missing ID")
}

// TestDeleteUserCompleteHandler_Success e' o unico caso do arquivo que precisa
// de um banco com esquema: o use case le o usuario, apaga e devolve o registro
// apagado.
func TestDeleteUserCompleteHandler_Success(t *testing.T) {
	db := ipmSQLite(t)
	if _, err := db.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT, jid TEXT, token TEXT, s3_enabled BOOLEAN)`); err != nil {
		t.Fatalf("criar tabela: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, name, jid, token, s3_enabled) VALUES ('user-1','Alice','5511@s.whatsapp.net','tok',0)`); err != nil {
		t.Fatalf("inserir usuario: %v", err)
	}

	router := mux.NewRouter()
	router.Handle("/admin/users/{id}/complete", deleteUserCompleteHandler(t, db))

	rec, recs := ipmServe(t, router, http.MethodDelete, "/admin/users/user-1/complete", "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&remaining); err != nil {
		t.Fatalf("contar usuarios: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("usuario sobreviveu a exclusao completa (%d linhas)", remaining)
	}
	ipmAssertNoOutcomeLog(t, recs)
}

func TestDeleteUserCompleteHandler_DatabaseFailure(t *testing.T) {
	router := mux.NewRouter()
	router.Handle("/admin/users/{id}/complete", deleteUserCompleteHandler(t, ipmSQLite(t)))

	rec, recs := ipmServe(t, router, http.MethodDelete, "/admin/users/user-1/complete", "", nil)

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, "database error")
}

// --- tabela dos handlers de corpo --------------------------------------

// miscBodyCase descreve um handler de miscelanea que decodifica corpo JSON.
type miscBodyCase struct {
	name string
	path string
	// build liga o handler as portas fake.
	build func(ops *contractsfake.ChatOperations, pm *contractsfake.PrivacyManager, jids *contractsfake.JIDResolver) http.Handler
	// validBody e' o menor corpo aceito pelo use case.
	validBody string
	// emptyBodyErr e' a causa produzida pelo corpo `{}`.
	emptyBodyErr string
	// failOp injeta falha na operacao final.
	failOp func(ops *contractsfake.ChatOperations, pm *contractsfake.PrivacyManager, err error)
	// opErr e' a causa que o use case produz a partir dessa falha.
	opErr string
}

func miscBodyCases() []miscBodyCase {
	log := &contractsfake.Logger{}
	return []miscBodyCase{
		{
			name: "RejectCall",
			path: "/chat/rejectcall",
			build: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, jids *contractsfake.JIDResolver) http.Handler {
				return NewRejectCallHandler(chat.NewRejectCallUseCase(ops, jids, log))
			},
			validBody:    `{"call_from":"5511999999999","call_id":"CALL1"}`,
			emptyBodyErr: "missing call_from in Payload",
			failOp: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, err error) {
				ops.RejectCallFunc = func(context.Context, string, domain.JID, string) error { return err }
			},
			opErr: "error rejecting call",
		},
		{
			name: "SetPrivacySetting",
			path: "/user/privacy",
			build: func(_ *contractsfake.ChatOperations, pm *contractsfake.PrivacyManager, _ *contractsfake.JIDResolver) http.Handler {
				return NewSetPrivacySettingHandler(user.NewSetPrivacySettingUseCase(pm, log))
			},
			validBody:    `{"privacy_setting":"groupadd","value":"contacts"}`,
			emptyBodyErr: "invalid privacy setting name",
			failOp: func(_ *contractsfake.ChatOperations, pm *contractsfake.PrivacyManager, err error) {
				pm.SetPrivacySettingFunc = func(context.Context, string, string, string) (any, error) { return nil, err }
			},
			opErr: "failed to set privacy setting",
		},
		{
			name: "RequestUnavailableMessage",
			path: "/chat/requestunavailablemessage",
			build: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, jids *contractsfake.JIDResolver) http.Handler {
				return NewRequestUnavailableMessageHandler(chat.NewRequestUnavailableMessageUseCase(ops, jids, log))
			},
			validBody:    `{"chat":"5511999999999","sender":"5511888888888","id":"MSG1"}`,
			emptyBodyErr: "missing Chat in Payload",
			failOp: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, err error) {
				ops.RequestUnavailableMessageFunc = func(context.Context, string, domain.JID, domain.JID, string) (domain.UnavailableMessageAck, error) {
					return domain.UnavailableMessageAck{}, err
				}
			},
			opErr: "failed to send unavailable message request",
		},
		{
			name: "ArchiveChat",
			path: "/chat/archive",
			build: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, jids *contractsfake.JIDResolver) http.Handler {
				return NewArchiveChatHandler(chat.NewArchiveChatUseCase(ops, jids, log))
			},
			validBody:    `{"jid":"5511999999999","archive":true}`,
			emptyBodyErr: "missing jid in Payload",
			failOp: func(ops *contractsfake.ChatOperations, _ *contractsfake.PrivacyManager, err error) {
				ops.ArchiveChatFunc = func(context.Context, string, domain.JID, bool) error { return err }
			},
			opErr: "failed to archive chat",
		},
	}
}

func miscBodyDeps() (*contractsfake.ChatOperations, *contractsfake.PrivacyManager, *contractsfake.JIDResolver) {
	return &contractsfake.ChatOperations{}, &contractsfake.PrivacyManager{}, &contractsfake.JIDResolver{}
}

func TestMiscBodyHandlers_Success(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if env := decodeEnvelope(t, rec); !env.Success {
				t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
			}
			ipmAssertNoOutcomeLog(t, recs)
		})
	}
}

func TestMiscBodyHandlers_Unauthorized(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, tc.validBody, nil)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, recs, "unauthorized")
			if len(ops.EnsureSessionCalls)+len(pm.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao nao autenticada alcancou a porta")
			}
		})
	}
}

func TestMiscBodyHandlers_MissingSessionID(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "missing session id")
			if len(ops.EnsureSessionCalls)+len(pm.EnsureSessionCalls) != 0 {
				t.Fatal("requisicao sem session id alcancou a porta")
			}
		})
	}
}

func TestMiscBodyHandlers_MalformedBody(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, `{"jid":`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "could not decode payload")
			if len(ops.EnsureSessionCalls)+len(pm.EnsureSessionCalls) != 0 {
				t.Fatal("corpo malformado alcancou a porta")
			}
		})
	}
}

func TestMiscBodyHandlers_IncompletePayload(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, `{}`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, tc.emptyBodyErr)
		})
	}
}

func TestMiscBodyHandlers_SessionFailure(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()
			ops.SessionGuard = contractsfake.FailSession(ipmErrBoom)
			pm.SessionGuard = contractsfake.FailSession(ipmErrBoom)

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
		})
	}
}

func TestMiscBodyHandlers_OperationFailure(t *testing.T) {
	for _, tc := range miscBodyCases() {
		t.Run(tc.name, func(t *testing.T) {
			ops, pm, jids := miscBodyDeps()
			tc.failOp(ops, pm, ipmErrBoom)

			rec, recs := ipmServe(t, tc.build(ops, pm, jids), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, tc.opErr)
		})
	}
}

// --- GET /user/privacy -------------------------------------------------

func getPrivacyHandler(pm *contractsfake.PrivacyManager) http.Handler {
	return NewGetPrivacySettingsHandler(user.NewGetPrivacySettingsUseCase(pm, &contractsfake.Logger{}))
}

func TestGetPrivacySettingsHandler_Success(t *testing.T) {
	pm := &contractsfake.PrivacyManager{}

	rec, recs := ipmServe(t, getPrivacyHandler(pm), http.MethodGet, "/user/privacy", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if len(pm.GetPrivacySettingsCalls) != 1 {
		t.Fatalf("GetPrivacySettings chamado %d vez(es), quero 1", len(pm.GetPrivacySettingsCalls))
	}
	ipmAssertNoOutcomeLog(t, recs)
}

func TestGetPrivacySettingsHandler_Unauthorized(t *testing.T) {
	pm := &contractsfake.PrivacyManager{}

	rec, _ := ipmServe(t, getPrivacyHandler(pm), http.MethodGet, "/user/privacy", "", nil)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if len(pm.EnsureSessionCalls) != 0 {
		t.Fatal("requisicao nao autenticada alcancou a porta")
	}
}

func TestGetPrivacySettingsHandler_SessionFailure(t *testing.T) {
	pm := &contractsfake.PrivacyManager{SessionGuard: contractsfake.FailSession(ipmErrBoom)}

	rec, recs := ipmServe(t, getPrivacyHandler(pm), http.MethodGet, "/user/privacy", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
}

func TestGetPrivacySettingsHandler_ReadFailure(t *testing.T) {
	pm := &contractsfake.PrivacyManager{
		GetPrivacySettingsFunc: func(context.Context, string) (any, error) { return nil, ipmErrBoom },
	}

	rec, recs := ipmServe(t, getPrivacyHandler(pm), http.MethodGet, "/user/privacy", "",
		func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
	logassert.OutcomeLogged(t, recs, "failed to get privacy settings")
}
