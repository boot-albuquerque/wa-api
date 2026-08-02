package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

// captureGlobalLogForMiddleware redireciona o logger de pacote para um
// buffer. ResolveConnectEvents não recebe *http.Request, então escreve pelo
// logger global e não pelo de requisição.
func captureGlobalLogForMiddleware(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = original })
	return &buf
}

// serveWithRequestLogger instala um logger de requisição à frente do
// middleware, como faz a cadeia hlog de bootstrap/router.go. Sem ele
// hlog.FromRequest devolve um logger desabilitado e nada do que o middleware
// registra seria observável.
func serveWithRequestLogger(t *testing.T, handler http.Handler, r *http.Request) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	rec := httptest.NewRecorder()
	hlog.NewHandler(zerolog.New(&buf))(handler).ServeHTTP(rec, r)
	return rec, &buf
}

func TestValuesGet(t *testing.T) {
	v := NewValues(map[string]string{"Id": "u1", "Empty": ""})

	if got := v.Get("Id"); got != "u1" {
		t.Errorf("Get(\"Id\") = %q, want %q", got, "u1")
	}
	if got := v.Get("Empty"); got != "" {
		t.Errorf("Get(\"Empty\") = %q, want empty", got)
	}
	if got := v.Get("Missing"); got != "" {
		t.Errorf("Get on an absent key = %q, want empty", got)
	}

	// O zero value chega ao Get: AuthAlice guarda Values no cache e um
	// asserted-cast de entrada malformada produziria M == nil. Get tem de
	// devolver "" em vez de entrar em pânico — é o que separa uma
	// autenticação negada de um 500.
	var zero Values
	if got := zero.Get("Id"); got != "" {
		t.Errorf("zero Values.Get = %q, want empty", got)
	}
}

// TestAuthAdminAcceptsMatchingToken: o caminho feliz precisa chamar o
// próximo handler; se AuthAdmin respondesse sozinho, toda rota /admin
// devolveria 200 vazio.
func TestAuthAdminAcceptsMatchingToken(t *testing.T) {
	called := false
	handler := AuthAdmin("admin-secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	r.Header.Set("Authorization", "admin-secret")

	rec, logs := serveWithRequestLogger(t, handler, r)

	if !called {
		t.Error("AuthAdmin did not call the next handler for a matching token")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if out := logs.String(); strings.Contains(out, `"level":"warn"`) {
		t.Errorf("a successful admin request emitted a rejection record: %s", out)
	}
}

func TestAuthAdminRejectsAndLogs(t *testing.T) {
	tests := []struct {
		name             string
		header           string
		wantTokenPresent string
	}{
		{"wrong token", "not-the-secret", `"token_present":true`},
		{"missing header", "", `"token_present":false`},
		{"prefix of the secret", "admin-secre", `"token_present":true`},
		{"secret plus suffix", "admin-secretx", `"token_present":true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := AuthAdmin("admin-secret")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("next handler ran for a rejected admin request")
			}))

			r := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}

			rec, logs := serveWithRequestLogger(t, handler, r)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			out := logs.String()
			if !strings.Contains(out, "admin authentication rejected") {
				t.Fatalf("rejection was not logged; logs: %s", out)
			}
			if !strings.Contains(out, `"level":"warn"`) {
				t.Errorf("rejection not logged at WARN; logs: %s", out)
			}
			if !strings.Contains(out, tt.wantTokenPresent) {
				t.Errorf("log record missing %s; logs: %s", tt.wantTokenPresent, out)
			}
			// O token apresentado nunca entra no registro: quem manda um
			// token errado pode ter mandado o token CERTO de outra conta.
			if tt.header != "" && strings.Contains(out, tt.header) {
				t.Errorf("the presented token leaked into the log: %s", out)
			}
		})
	}
}

// TestAuthAliceCacheHitSkipsDatabase passa um *sql.DB nil de propósito: se o
// caminho de cache tocasse o banco, o teste entraria em pânico. É a única
// forma de provar que o acerto de cache não consulta — e é o que dá sentido
// ao TTL de 10 minutos.
func TestAuthAliceCacheHitSkipsDatabase(t *testing.T) {
	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	userCache.Set("cached-token", NewValues(map[string]string{"Id": "u1", "Name": "user-u1"}), cache.NoExpiration)

	var seen string
	handler := AuthAlice(nil, userCache)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(appport.UserInfoKey).(Values); ok {
			seen = v.Get("Id")
		}
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "cached-token")

	rec, _ := serveWithRequestLogger(t, handler, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if seen != "u1" {
		t.Errorf("downstream handler saw user id %q, want %q", seen, "u1")
	}
}

// TestAuthAliceCachedEntryWithoutIDIsRejected: uma entrada de cache sem "Id"
// não pode autenticar. Sem esta checagem, gravar um Values vazio no cache
// (por bug ou por corrida) deixaria passar qualquer request com aquele token.
func TestAuthAliceCachedEntryWithoutIDIsRejected(t *testing.T) {
	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	userCache.Set("hollow-token", NewValues(map[string]string{"Name": "no id here"}), cache.NoExpiration)

	handler := AuthAlice(nil, userCache)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("next handler ran for a cache entry with no user id")
	}))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "hollow-token")

	rec, logs := serveWithRequestLogger(t, handler, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	out := logs.String()
	if !strings.Contains(out, "authentication rejected") {
		t.Fatalf("rejection was not logged; logs: %s", out)
	}
	if !strings.Contains(out, `"cache_hit":true`) {
		t.Errorf("log record does not record that the rejection came from cache; logs: %s", out)
	}
}

// TestAuthAliceQueryErrorLogsAndReturns500 cobre o caminho em que a consulta
// falha: o banco existe mas não tem a tabela. O corpo devolvido é genérico,
// então o registro de ERROR é o único lugar onde a causa fica.
func TestAuthAliceQueryErrorLogsAndReturns500(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration))(
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("next handler ran after the user lookup failed")
		}))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "any-token")

	rec, logs := serveWithRequestLogger(t, handler, r)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	out := logs.String()
	if !strings.Contains(out, "user lookup query failed") {
		t.Fatalf("query failure was not logged; logs: %s", out)
	}
	if !strings.Contains(out, `"level":"error"`) {
		t.Errorf("query failure not logged at ERROR; logs: %s", out)
	}
	if !strings.Contains(out, `"error":`) {
		t.Errorf("log record carries no cause; logs: %s", out)
	}
	if strings.Contains(rec.Body.String(), "no such table") {
		t.Errorf("the SQL error leaked into the response body: %s", rec.Body.String())
	}
}

func TestResolveConnectEvents(t *testing.T) {
	supported := []string{"Message", "Receipt", "Presence"}

	tests := []struct {
		name        string
		subscribe   []string
		existing    string
		wantEvents  string
		wantChanged bool
	}{
		{"empty subscribe keeps existing", nil, "Message", "Message", false},
		{"single supported", []string{"Message"}, "", "Message", true},
		{"multiple supported", []string{"Message", "Receipt"}, "", "Message,Receipt", true},
		{"same as existing is not a change", []string{"Message"}, "Message", "Message", false},
		{"duplicates collapse", []string{"Message", "Message"}, "", "Message", true},
		{"unsupported is discarded", []string{"Message", "NotAnEvent"}, "", "Message", true},
		{"all unsupported yields empty", []string{"NotAnEvent"}, "Message", "", true},
		{"all unsupported matching empty existing", []string{"NotAnEvent"}, "", "", false},
		{"order is the request order", []string{"Presence", "Message"}, "", "Presence,Message", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ResolveConnectEvents(customhttp.Find, supported, tt.subscribe, tt.existing)
			if got != tt.wantEvents {
				t.Errorf("events = %q, want %q", got, tt.wantEvents)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
		})
	}
}

// TestResolveConnectEventsWarnsOnDiscard: descartar um tipo de evento que o
// operador pediu é silencioso do ponto de vista da resposta — a sessão
// conecta normalmente, só não entrega aquele evento. O WARN é o único aviso.
func TestResolveConnectEventsWarnsOnDiscard(t *testing.T) {
	logs := captureGlobalLogForMiddleware(t)

	ResolveConnectEvents(customhttp.Find, []string{"Message"}, []string{"Message", "Typo"}, "")

	out := logs.String()
	if !strings.Contains(out, "Event type discarded") {
		t.Fatalf("discard was not logged; logs: %s", out)
	}
	if !strings.Contains(out, `"Type":"Typo"`) {
		t.Errorf("log record does not name the discarded type; logs: %s", out)
	}
}

// TestAuthAliceScanErrorLogsAndReturns500 cobre o segundo caminho de falha do
// lookup: a consulta roda, mas o valor de uma coluna não cabe no destino do
// Scan. SQLite tem tipagem dinâmica, então uma linha com texto na coluna
// INTEGER `history` é gravável e só explode aqui. O efeito visível é um 500
// sem causa no corpo; o registro de ERROR é o que torna o incidente
// diagnosticável.
func TestAuthAliceScanErrorLogsAndReturns500(t *testing.T) {
	db := newAuthTestDB(t)
	if _, err := db.Exec(`INSERT INTO users
		(id, name, token, token_hash, webhook, jid, qrcode, events, proxy_url, history, s3_enabled, media_delivery)
		VALUES ('u1', 'user-u1', 'bad-history-token', NULL, '', '', '', '', '', 'not-a-number', 0, 'base64')`); err != nil {
		t.Fatalf("insert row with non-integer history: %v", err)
	}

	handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration))(
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("next handler ran after the row scan failed")
		}))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "bad-history-token")

	rec, logs := serveWithRequestLogger(t, handler, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	out := logs.String()
	if !strings.Contains(out, "scanning user row failed") {
		t.Fatalf("scan failure was not logged; logs: %s", out)
	}
	if !strings.Contains(out, `"level":"error"`) {
		t.Errorf("scan failure not logged at ERROR; logs: %s", out)
	}
}
