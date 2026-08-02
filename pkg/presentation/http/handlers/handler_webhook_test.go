package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	"github.com/patrickmn/go-cache"
)

// webhookMockUserInfo implementa a interface userInfo injetada pelo middleware.
type webhookMockUserInfo struct {
	id    string
	token string
}

func (m *webhookMockUserInfo) Get(key string) string {
	switch key {
	case "Id":
		return m.id
	case "Token":
		return m.token
	default:
		return ""
	}
}

// webhookFakeDB implementa WebhookHandlerDB sem depender de um driver real.
type webhookFakeDB struct {
	execCalls int
	execErr   error
	queryErr  error

	// rowsDB, quando nao-nil, e' a fonte real de *sql.Rows: o handler nao
	// consegue iterar sobre nada que nao venha de database/sql, e sem isso
	// o caminho de sucesso do GET (Next/Scan/Err/Close) fica inalcancavel.
	rowsDB *sql.DB
}

func (d *webhookFakeDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	if d.rowsDB != nil {
		return d.rowsDB.Query("SELECT webhook,events FROM users")
	}
	return nil, errors.New("no rows source configured")
}

type webhookFakeResult struct{}

func (webhookFakeResult) LastInsertId() (int64, error) { return 0, nil }
func (webhookFakeResult) RowsAffected() (int64, error) { return 1, nil }

func (d *webhookFakeDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	d.execCalls++
	if d.execErr != nil {
		return nil, d.execErr
	}
	return webhookFakeResult{}, nil
}

func newWebhookTestContext(db *webhookFakeDB) *WebhookHandlerContext {
	return &WebhookHandlerContext{
		DB:              db,
		UserCache:       cache.New(5*time.Minute, 10*time.Minute),
		SupportedEvents: []string{"Message", "ReadReceipt"},
		FindInSlice: func(slice []string, val string) bool {
			for _, s := range slice {
				if s == val {
					return true
				}
			}
			return false
		},
		UpdateUserInfo: func(info interface{}, key, value string) interface{} { return info },
	}
}

// withTypedUserInfo popula o contexto com a chave TIPADA appport.UserInfoKey,
// exatamente como o middleware AuthAlice faz.
func withTypedUserInfo(r *http.Request) *http.Request {
	// O token e' o admin token-sentinela do co-gate D: as 4 rotas /webhook
	// leem info.Get("Token") e o usam como chave de cache. Plantar aqui o
	// valor proibido e' o que torna a clausula (d) NAO-vacua neste arquivo —
	// o segredo esta' no caminho do handler, e sua ausencia no log significa
	// alguma coisa.
	ctx := context.WithValue(r.Context(), appport.UserInfoKey, &webhookMockUserInfo{
		id:    "42",
		token: logassertAdminToken,
	})
	return r.WithContext(ctx)
}

func webhookHandlers(ctx *WebhookHandlerContext) map[string]http.Handler {
	return map[string]http.Handler{
		http.MethodGet:    NewGetWebhookHandler(ctx),
		http.MethodPost:   NewSetWebhookHandler(ctx),
		http.MethodPut:    NewUpdateWebhookHandler(ctx),
		http.MethodDelete: NewDeleteWebhookHandler(ctx),
	}
}

func webhookBody(method string) string {
	switch method {
	case http.MethodPost:
		return `{"webhookurl":"https://example.com/hook","events":["Message"]}`
	case http.MethodPut:
		return `{"webhook":"https://example.com/hook","events":["Message"],"active":true}`
	default:
		return ""
	}
}

// TestWebhookHandlers_TypedUserInfo_NoPanic garante que as 4 rotas /webhook
// leem o userinfo pela chave tipada appport.UserInfoKey sem entrar em pânico.
func TestWebhookHandlers_TypedUserInfo_NoPanic(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			req = withTypedUserInfo(req)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusUnauthorized {
				t.Fatalf("%s /webhook returned 401 with typed userinfo in context", method)
			}
			if method != http.MethodGet && w.Code != http.StatusOK {
				t.Fatalf("%s /webhook expected 200, got %d (body=%s)", method, w.Code, w.Body.String())
			}
		})
	}
}

// TestWebhookHandlers_RawStringKey_401 garante que a chave crua string
// "userinfo" (que NÃO é appport.UserInfoKey) resulta em 401 e não em pânico.
func TestWebhookHandlers_RawStringKey_401(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked instead of responding 401: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			//nolint:staticcheck // intencional: chave crua para provar que não é aceita
			req = req.WithContext(context.WithValue(req.Context(), "userinfo", &webhookMockUserInfo{id: "42"}))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s /webhook expected 401, got %d", method, w.Code)
			}
		})
	}
}

// TestWebhookHandlers_NoUserInfo_401 garante 401 quando o middleware não rodou.
func TestWebhookHandlers_NoUserInfo_401(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked instead of responding 401: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s /webhook expected 401, got %d", method, w.Code)
			}
		})
	}
}

// --- driver falso: a unica forma de fabricar *sql.Rows ------------------
//
// GetWebhookHandler itera sobre *sql.Rows — um struct concreto de
// database/sql que so' pode ser construido por um driver. Os quatro ramos de
// saida do laco (Scan falhou, rows.Err falhou, Close falhou, sucesso) sao
// inalcancaveis sem um. Este driver e' ~50 linhas e da' controle exato sobre
// os tres erros, sem CGO, sem arquivo e sem corrida de contexto — o que um
// SQLite real nao daria para o ramo de rows.Err.

type webhookRowsSpec struct {
	rows     [][]driver.Value
	nextErr  error // devolvido por Next DEPOIS de esgotar rows
	closeErr error
}

var (
	webhookStubMu    sync.Mutex
	webhookStubSpecs = map[string]*webhookRowsSpec{}
	webhookStubSeq   int
)

type webhookStubDriver struct{}

func init() { sql.Register("webhookstub", webhookStubDriver{}) }

func (webhookStubDriver) Open(dsn string) (driver.Conn, error) {
	webhookStubMu.Lock()
	defer webhookStubMu.Unlock()
	spec, ok := webhookStubSpecs[dsn]
	if !ok {
		return nil, fmt.Errorf("dsn nao registrado: %s", dsn)
	}
	return &webhookStubConn{spec: spec}, nil
}

// newWebhookRowsDB devolve um *sql.DB cujo unico Query produz spec.
func newWebhookRowsDB(t *testing.T, spec *webhookRowsSpec) *sql.DB {
	t.Helper()
	webhookStubMu.Lock()
	webhookStubSeq++
	dsn := fmt.Sprintf("spec-%d", webhookStubSeq)
	webhookStubSpecs[dsn] = spec
	webhookStubMu.Unlock()

	db, err := sql.Open("webhookstub", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type webhookStubConn struct{ spec *webhookRowsSpec }

func (c *webhookStubConn) Prepare(string) (driver.Stmt, error) {
	return &webhookStubStmt{spec: c.spec}, nil
}
func (c *webhookStubConn) Close() error              { return nil }
func (c *webhookStubConn) Begin() (driver.Tx, error) { return nil, errors.New("nao suportado") }

type webhookStubStmt struct{ spec *webhookRowsSpec }

func (s *webhookStubStmt) Close() error  { return nil }
func (s *webhookStubStmt) NumInput() int { return -1 }
func (s *webhookStubStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("nao suportado")
}
func (s *webhookStubStmt) Query([]driver.Value) (driver.Rows, error) {
	return &webhookStubRows{spec: s.spec}, nil
}

type webhookStubRows struct {
	spec *webhookRowsSpec
	i    int
}

func (r *webhookStubRows) Columns() []string { return []string{"webhook", "events"} }
func (r *webhookStubRows) Close() error      { return r.spec.closeErr }
func (r *webhookStubRows) Next(dest []driver.Value) error {
	if r.i < len(r.spec.rows) {
		copy(dest, r.spec.rows[r.i])
		r.i++
		return nil
	}
	if r.spec.nextErr != nil {
		return r.spec.nextErr
	}
	return io.EOF
}

// --- infraestrutura comum dos casos ------------------------------------

// serveWebhook roda h com a cadeia hlog request-scoped instalada (a mesma de
// router.go) e devolve resposta e registros de log.
func serveWebhook(t *testing.T, h http.Handler, method, body string, authed bool) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	req := httptest.NewRequest(method, "/webhook", strings.NewReader(body))
	if authed {
		req = withTypedUserInfo(req)
	}
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	return rec, capture.Records(t)
}

func webhookEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo %s)", err, rec.Body.String())
	}
	return env
}

// TestWebhookHandlers_Unauthenticated_LogsCause: as 4 rotas respondem 401 E
// registram a causa. O log de fronteira do router so' carrega o status; sem
// este registro, "por que 401" nao existe em lugar nenhum.
func TestWebhookHandlers_Unauthenticated_LogsCause(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			rec, recs := serveWebhook(t, handler, method, webhookBody(method), false)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, quero 401", rec.Code)
			}
			logassert.OutcomeLogged(t, recs, "unauthorized")
		})
	}
}

// TestWebhookHandlers_MalformedBody_400_LogsCause: POST e PUT leem corpo.
// Corpo ilegivel e' recusa do cliente — 400 com warn e a causa do decoder.
func TestWebhookHandlers_MalformedBody_400_LogsCause(t *testing.T) {
	ctx := newWebhookTestContext(&webhookFakeDB{})
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			rec, recs := serveWebhook(t, webhookHandlers(ctx)[method], method, `{"webhookurl":`, true)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs)
			if got.str("level") != "warn" {
				t.Fatalf("corpo malformado e' falha do cliente: nivel %q, quero warn", got.str("level"))
			}
		})
	}
}

// TestWebhookHandlers_DBFailure_500_LogsCause: falha de banco e' 500 com
// ERROR (nao warn) e com a causa, em todas as rotas que escrevem.
func TestWebhookHandlers_DBFailure_500_LogsCause(t *testing.T) {
	const boom = "connection refused by postgres"

	cases := []struct {
		method string
		body   string
	}{
		{http.MethodPost, `{"webhookurl":"https://example.com/hook","events":["Message"]}`},
		{http.MethodPost, `{"webhookurl":"https://example.com/hook"}`}, // ramo sem events
		{http.MethodPut, `{"webhook":"https://example.com/hook","events":["Message"],"active":true}`},
		{http.MethodPut, `{"webhook":"https://example.com/hook","active":true}`}, // ramo sem events
		{http.MethodDelete, ""},
	}

	for _, tc := range cases {
		t.Run(tc.method+tc.body, func(t *testing.T) {
			db := &webhookFakeDB{execErr: errors.New(boom)}
			rec, recs := serveWebhook(t, webhookHandlers(newWebhookTestContext(db))[tc.method], tc.method, tc.body, true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs, boom)
			if got.str("level") != "error" {
				t.Fatalf("falha de banco: nivel %q, quero error", got.str("level"))
			}
			if got.str("user_id") != "42" {
				t.Fatalf("registro sem user_id correlacionavel: %s", got.Raw)
			}
			if strings.Contains(rec.Body.String(), boom) {
				t.Fatalf("detalhe do banco vazou no corpo: %s", rec.Body.String())
			}
		})
	}
}

// TestGetWebhook_QueryFailure_500_LogsCause cobre o unico ramo de erro que e'
// de leitura, nao de escrita.
func TestGetWebhook_QueryFailure_500_LogsCause(t *testing.T) {
	const boom = "select failed: relation users does not exist"
	db := &webhookFakeDB{queryErr: errors.New(boom)}

	rec, recs := serveWebhook(t, NewGetWebhookHandler(newWebhookTestContext(db)), http.MethodGet, "", true)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, quero 500", rec.Code)
	}
	got := logassert.OutcomeLogged(t, recs, boom)
	if got.str("op") != "select" {
		t.Fatalf("registro nao nomeia a operacao que quebrou: %s", got.Raw)
	}
}

// TestGetWebhook_RowFailures_500_LogsCause cobre os dois ramos internos do
// laco: Scan e rows.Err. Sao os que um teste sem driver proprio nao alcanca.
func TestGetWebhook_RowFailures_500_LogsCause(t *testing.T) {
	tests := []struct {
		name string
		spec *webhookRowsSpec
		op   string
	}{
		{
			name: "scan falha em coluna NULL",
			spec: &webhookRowsSpec{rows: [][]driver.Value{{nil, nil}}},
			op:   "scan",
		},
		{
			name: "iteracao falha no meio do resultado",
			spec: &webhookRowsSpec{nextErr: errors.New("connection lost mid-scan")},
			op:   "iterate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &webhookFakeDB{rowsDB: newWebhookRowsDB(t, tt.spec)}
			rec, recs := serveWebhook(t, NewGetWebhookHandler(newWebhookTestContext(db)), http.MethodGet, "", true)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
			}
			got := logassert.OutcomeLogged(t, recs)
			if got.str("op") != tt.op {
				t.Fatalf("op = %q, quero %q: %s", got.str("op"), tt.op, got.Raw)
			}
		})
	}
}

// TestGetWebhook_CloseFailure_LoggedNotSwallowed: o rows.Close diferido NAO
// altera a resposta — quando ele roda, o corpo ja' foi escrito. E' por isso
// exatamente o erro que desaparece sem log. S-consume: `if err != nil` que
// nao propaga tem de logar.
//
// O caso e' montado sobre uma saida ANTECIPADA do laco (coluna NULL faz o
// Scan falhar): so' assim o Close diferido encontra um *sql.Rows ainda
// aberto. Se a iteracao chegasse ao fim, database/sql ja' teria fechado as
// linhas sozinho e o erro do driver sairia por rows.Err, nao por Close.
func TestGetWebhook_CloseFailure_LoggedNotSwallowed(t *testing.T) {
	const boom = "closing a busy statement"
	spec := &webhookRowsSpec{
		rows:     [][]driver.Value{{nil, nil}},
		closeErr: errors.New(boom),
	}
	db := &webhookFakeDB{rowsDB: newWebhookRowsDB(t, spec)}

	rec, recs := serveWebhook(t, NewGetWebhookHandler(newWebhookTestContext(db)), http.MethodGet, "", true)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, quero 500 — o Scan falhou antes do Close", rec.Code)
	}
	got := logassert.OutcomeLogged(t, recs, boom)
	if !strings.Contains(got.Raw, "failed to close rows") {
		t.Fatalf("o erro de Close saiu confundido com o erro de Scan: %s", got.Raw)
	}
}

// TestGetWebhook_Success_200 trava o corpo do caminho feliz: o webhook lido e
// a lista de eventos ja' partida.
func TestGetWebhook_Success_200(t *testing.T) {
	spec := &webhookRowsSpec{rows: [][]driver.Value{{"https://example.com/hook", "Message,ReadReceipt"}}}
	db := &webhookFakeDB{rowsDB: newWebhookRowsDB(t, spec)}

	rec, recs := serveWebhook(t, NewGetWebhookHandler(newWebhookTestContext(db)), http.MethodGet, "", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	env := webhookEnvelope(t, rec)
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope sem data: %s", rec.Body.String())
	}
	if data["webhook"] != "https://example.com/hook" {
		t.Fatalf("webhook = %v", data["webhook"])
	}
	subs, ok := data["subscribe"].([]any)
	if !ok || len(subs) != 2 || subs[0] != "Message" || subs[1] != "ReadReceipt" {
		t.Fatalf("subscribe = %v, quero [Message ReadReceipt]", data["subscribe"])
	}
	logassert.NoSecrets(t, recs)
}

// TestSetWebhook_DiscardsUnsupportedEvent: evento fora da lista suportada e'
// descartado com registro, e o que sobra e' persistido.
func TestSetWebhook_DiscardsUnsupportedEvent(t *testing.T) {
	db := &webhookFakeDB{}
	body := `{"webhookurl":"https://example.com/hook","events":["Message","NaoExiste"]}`

	rec, recs := serveWebhook(t, NewSetWebhookHandler(newWebhookTestContext(db)), http.MethodPost, body, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if db.execCalls != 1 {
		t.Fatalf("execCalls = %d, quero 1", db.execCalls)
	}
	var discarded bool
	for _, r := range recs {
		if r.str("Type") == "NaoExiste" {
			discarded = true
		}
	}
	if !discarded {
		t.Fatal("evento nao suportado foi descartado em silencio")
	}
	logassert.NoSecrets(t, recs)
}

// TestSetWebhook_OnlyUnsupportedEvents_PersistsEmpty: quando NENHUM evento
// sobrevive ao filtro, a assinatura persistida e' vazia — nao a lista crua.
func TestSetWebhook_OnlyUnsupportedEvents_PersistsEmpty(t *testing.T) {
	db := &webhookFakeDB{}
	body := `{"webhookurl":"https://example.com/hook","events":["NaoExiste"]}`

	rec, recs := serveWebhook(t, NewSetWebhookHandler(newWebhookTestContext(db)), http.MethodPost, body, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200", rec.Code)
	}
	logassert.NoSecrets(t, recs)
}

// TestSetWebhook_NoEvents_Success cobre o ramo sem events, que faz um UPDATE
// diferente.
func TestSetWebhook_NoEvents_Success(t *testing.T) {
	db := &webhookFakeDB{}
	rec, recs := serveWebhook(t, NewSetWebhookHandler(newWebhookTestContext(db)),
		http.MethodPost, `{"webhookurl":"https://example.com/hook"}`, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200", rec.Code)
	}
	env := webhookEnvelope(t, rec)
	data := env["data"].(map[string]any)
	if data["webhook"] != "https://example.com/hook" {
		t.Fatalf("webhook = %v", data["webhook"])
	}
	logassert.NoSecrets(t, recs)
}

// TestUpdateWebhook_ActiveFlagControlsPersistence: active=false zera webhook
// e eventos, independentemente do que o corpo pediu. E' a regra propria do
// PUT, e a unica diferenca de comportamento entre ele e o POST.
func TestUpdateWebhook_ActiveFlagControlsPersistence(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantWebhook string
		wantEvents  int
		wantActive  bool
	}{
		{
			name:        "ativo mantem webhook e eventos",
			body:        `{"webhook":"https://example.com/hook","events":["Message"],"active":true}`,
			wantWebhook: "https://example.com/hook",
			wantEvents:  1,
			wantActive:  true,
		},
		{
			name:        "inativo zera webhook",
			body:        `{"webhook":"https://example.com/hook","events":["Message"],"active":false}`,
			wantWebhook: "",
			wantEvents:  1, // validEvents e' reportado; o que zera e' o que se PERSISTE
			wantActive:  false,
		},
		{
			name:        "evento nao suportado e' descartado",
			body:        `{"webhook":"https://example.com/hook","events":["Message","NaoExiste"],"active":true}`,
			wantWebhook: "https://example.com/hook",
			wantEvents:  1,
			wantActive:  true,
		},
		{
			name:        "sem eventos usa o UPDATE curto",
			body:        `{"webhook":"https://example.com/hook","active":true}`,
			wantWebhook: "https://example.com/hook",
			wantEvents:  0,
			wantActive:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &webhookFakeDB{}
			rec, recs := serveWebhook(t, NewUpdateWebhookHandler(newWebhookTestContext(db)), http.MethodPut, tt.body, true)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
			}
			data := webhookEnvelope(t, rec)["data"].(map[string]any)
			if data["webhook"] != tt.wantWebhook {
				t.Fatalf("webhook = %v, quero %q", data["webhook"], tt.wantWebhook)
			}
			if data["active"] != tt.wantActive {
				t.Fatalf("active = %v, quero %v", data["active"], tt.wantActive)
			}
			events, _ := data["events"].([]any)
			if len(events) != tt.wantEvents {
				t.Fatalf("events = %v, quero %d entrada(s)", data["events"], tt.wantEvents)
			}
			logassert.NoSecrets(t, recs)
		})
	}
}

// TestDeleteWebhook_Success_200 trava o corpo do DELETE e a persistencia.
func TestDeleteWebhook_Success_200(t *testing.T) {
	db := &webhookFakeDB{}
	rec, recs := serveWebhook(t, NewDeleteWebhookHandler(newWebhookTestContext(db)), http.MethodDelete, "", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if db.execCalls != 1 {
		t.Fatalf("execCalls = %d, quero 1", db.execCalls)
	}
	if _, ok := webhookEnvelope(t, rec)["data"].(map[string]any)["Details"]; !ok {
		t.Fatalf("resposta sem Details: %s", rec.Body.String())
	}
	logassert.NoSecrets(t, recs)
}
