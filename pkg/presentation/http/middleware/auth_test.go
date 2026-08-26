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
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "auth.db")+dbpkg.SQLitePragmas)
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
	handler := AuthAlice(db.DB, userCache, nil)(http.HandlerFunc(
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

// TestAuthAliceRecusaLinhaSoComTextoClaro fecha a janela de transição que o
// teste anterior mantinha aberta.
//
// Ele AFIRMAVA o contrário: linha com `token_hash` NULL autenticava pelo token
// cru, e essa mitigação existia para a migração 11 não invalidar sessões. A
// janela fechou na F97 etapa 2 — a consulta casa só por hash.
//
// O que torna isso seguro NÃO é este teste, é a migração 16: ela preenche o
// hash que faltar e ABORTA se sobrar alguma linha sem ele, em vez de apagar o
// texto claro e deixar alguém sem acesso. Aqui a linha é construída à mão,
// justamente no estado que a migração se recusa a deixar existir.
func TestAuthAliceRecusaLinhaSoComTextoClaro(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "legacy-token", "")

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "legacy-token")

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d: o texto claro ainda autentica", got, http.StatusUnauthorized)
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

// TestAuthAliceQueryStringTokenRecusadaForaDoWebSocket fixa a F75.
//
// A query string deixou de autenticar nas rotas comuns — nelas o header sempre
// foi possível, e a query só sobrevivia por compatibilidade. O teste antes
// AFIRMAVA o contrário (aceitar com aviso de depreciação); ele foi reescrito, e
// não relaxado: agora exige a recusa.
func TestAuthAliceQueryStringTokenRecusadaForaDoWebSocket(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	var logs bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = original })

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text?token=good-token", nil)

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d: a query string continua autenticando fora do WebSocket", got, http.StatusUnauthorized)
	}

	out := logs.String()
	if !strings.Contains(out, "recusado nesta rota") {
		t.Errorf("a recusa nao foi registrada; quem operar nao vai saber por que o cliente parou: %s", out)
	}
	if strings.Contains(out, "good-token") {
		t.Errorf("o token vazou para o log: %s", out)
	}
}

// TestAuthAliceQueryStringTokenAceitaNoWebSocket é a exceção, e o motivo dela
// não é preferência nossa: a API `WebSocket` do navegador não permite header
// customizado no handshake. Recusar aqui quebraria todo painel de navegador sem
// oferecer saída.
//
// O par com o teste acima é o que importa: a exceção precisa ser EXATAMENTE uma
// rota. Uma regra que aceitasse "rotas de sessão" ou qualquer prefixo mais largo
// passaria nos dois e deixaria o buraco aberto.
func TestAuthAliceQueryStringTokenAceitaNoWebSocket(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "good-token", domain.HashToken("good-token"))

	r := httptest.NewRequest(http.MethodGet, wsPath+"?token=good-token", nil)

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d: o WebSocket de navegador ficou sem forma de autenticar", got, http.StatusOK)
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

// TestAuthAliceEmptyTokenNeverMatchesBlankRow fixa a F100.
//
// A consulta casa por `token = $1`, e uma requisição sem token nenhum produz
// $1 = "". Bastava UMA linha com token vazio para essa requisição anônima
// autenticar como aquele usuário. Medido na bancada: a resposta caiu de 401
// para 400 no instante em que a coluna foi branqueada.
//
// O usuário aqui é gravado JÁ com token vazio de propósito — é o estado que a
// etapa 1 da F97 vai produzir para todo usuário novo. Um teste que só mandasse
// requisição sem token contra uma tabela normal passaria antes e depois da
// guarda, sem provar nada.
func TestAuthAliceEmptyTokenNeverMatchesBlankRow(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u-blank", "", "")

	t.Run("sem header algum", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
		if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d: requisição anônima autenticou como o usuário de token vazio", got, http.StatusUnauthorized)
		}
	})

	t.Run("header presente e vazio", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
		r.Header.Set("token", "")
		if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", got, http.StatusUnauthorized)
		}
	})

	t.Run("query string vazia", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/chat/send/text?token=", nil)
		if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", got, http.StatusUnauthorized)
		}
	})
}

// TestAuthAliceStillAcceptsRealTokenAlongsideBlankRow é o controle na direção
// oposta: recusar token vazio não pode virar "recusar tudo". A tabela tem a
// linha em branco E uma linha legítima; a legítima continua entrando.
func TestAuthAliceStillAcceptsRealTokenAlongsideBlankRow(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u-blank", "", "")
	insertAuthUser(t, db, "u-real", "real-token", domain.HashToken("real-token"))

	r := httptest.NewRequest(http.MethodGet, "/chat/send/text", nil)
	r.Header.Set("token", "real-token")

	if got := serveAuth(db, cache.New(cache.NoExpiration, cache.NoExpiration), r).Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d: a guarda de token vazio recusou um token legítimo", got, http.StatusOK)
	}
}
