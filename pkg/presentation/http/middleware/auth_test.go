package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

func newAuthTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	if err := dbpkg.InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return db
}

// insertAuthUser grava um usuário. tokenHash vazio significa NULL — o estado de
// uma linha escrita por um writer que ainda não conhece token_hash.
func insertAuthUser(t *testing.T, db *sqlx.DB, id, token, tokenHash string) {
	t.Helper()
	var hash interface{}
	if tokenHash != "" {
		hash = tokenHash
	}
	if _, err := db.Exec(`INSERT INTO users
		(id, name, token, token_hash, webhook, jid, qrcode, events, proxy_url, history, s3_enabled, media_delivery)
		VALUES (?, ?, ?, ?, '', '', '', '', '', 0, 0, 'base64')`,
		id, "user-"+id, token, hash); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func serveAuth(db *sqlx.DB, userCache *cache.Cache, r *http.Request) *httptest.ResponseRecorder {
	handler := AuthAlice(db.DB, userCache)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	return rec
}

func TestAuthAliceValidToken(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "good-token")

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
}

// TestAuthAliceAcceptsRowWithoutTokenHash cobre a janela de transição: linhas
// gravadas antes da migração (token_hash NULL) continuam autenticando pelo
// token cru. É a mitigação que impede a fase de invalidar sessões existentes.
func TestAuthAliceAcceptsRowWithoutTokenHash(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "legacy-token", "")

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "legacy-token")

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
}

func TestAuthAliceInvalidToken(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "wrong-token")

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

// TestAuthAliceCachedEntryExpires é o controle de sec/F11: com
// cache.NoExpiration, deletar o usuário no banco não tinha efeito nenhum
// enquanto o processo vivesse. A entrada precisa carregar prazo de validade.
func TestAuthAliceCachedEntryExpires(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))
	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "good-token")
	serveAuth(db, userCache, r)

	item, found := userCache.Items()["good-token"]
	if !found {
		t.Fatal("successful auth did not populate the cache")
	}
	if item.Expiration == 0 {
		t.Error("cached auth entry has no expiration; a revoked user would stay authenticated forever")
	}
}

func TestAuthAliceQueryStringTokenAcceptedWithWarning(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	var logs bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = original })

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text?token=good-token", nil)

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}

	out := logs.String()
	if !strings.Contains(out, "token received via query string") {
		t.Errorf("no deprecation WARN emitted for query-string token; logs: %s", out)
	}
	if !strings.Contains(out, `"level":"warn"`) {
		t.Errorf("deprecation message was not logged at WARN; logs: %s", out)
	}
	if strings.Contains(out, "good-token") {
		t.Errorf("the token itself leaked into the log; logs: %s", out)
	}
}

func TestAuthAliceHeaderTokenEmitsNoDeprecationWarning(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	var logs bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = original })

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "good-token")
	serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r)

	if strings.Contains(logs.String(), "token received via query string") {
		t.Error("header-supplied token wrongly flagged as query-string usage")
	}
}
