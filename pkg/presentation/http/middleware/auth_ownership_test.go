package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

// insertOwnershipRow grava uma linha de account_ownership diretamente por
// SQL, e não via ClaimAccountIdentity: aquele método usa
// pg_advisory_xact_lock/hashtextextended, funções só do Postgres (ver o
// comentário do próprio pkg/infra/db/account_ownership.go), e os testes deste
// pacote rodam contra SQLite. O schema é o mesmo nos dois dialetos — só o
// mecanismo de arbitragem concorrente é que não existe aqui, e não é o que
// AuthAlice consome (CurrentStatusForSession é um SELECT simples).
func insertOwnershipRow(t *testing.T, db *sqlx.DB, id, identity, engine, sessionID, ownerID, status string, revision int64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO account_ownership
		(id, canonical_account_identity, engine, session_id, owner_id, status, ownership_revision, claimed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, identity, engine, sessionID, ownerID, status, revision, "2026-08-26T00:00:00Z"); err != nil {
		t.Fatalf("insert account_ownership row %s: %v", id, err)
	}
}

// TestAuthAlice_SessionSuperseded_E409 trava o item 6 do prompt arquitetural
// (F276): um token cuja sessão foi substituída por um claim mais novo para
// de operar. sessionID é o ID do próprio usuário (txtid) — ver a decisão
// documentada no comentário de AuthAlice em auth.go.
func TestAuthAlice_SessionSuperseded_E409(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "tok-superseded", domain.HashToken("tok-superseded"))

	ownership := dbpkg.NewAccountOwnershipRepository(db)
	// u1 fez um claim que foi SUPERSEDIDO por u2 para o mesmo
	// (identity, engine) — exatamente o estado que CurrentStatusForSession
	// lê: a linha mais recente para o session_id "u1" é a superseded, não a
	// active.
	insertOwnershipRow(t, db, "claim-1", "5511@s.whatsapp.net", "noise", "u1", "u1", "superseded", 1)
	insertOwnershipRow(t, db, "claim-2", "5511@s.whatsapp.net", "noise", "u2", "u2", "active", 2)

	handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration), ownership)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler ran for a superseded session — operação downstream NÃO deveria executar")
		}))

	r := httptest.NewRequest(http.MethodGet, "/user/info", nil)
	r.Header.Set("token", "tok-superseded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, queria 409 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"session_superseded"`) {
		t.Errorf("corpo = %s, queria conter session_superseded", rec.Body.String())
	}
	// Sem vazar detalhe da nova sessão (F276: "sem vazar detalhes da nova
	// sessão").
	if strings.Contains(rec.Body.String(), "claim-2") || strings.Contains(rec.Body.String(), `"u2"`) {
		t.Errorf("corpo vazou detalhe da nova sessão: %s", rec.Body.String())
	}
}

// TestAuthAlice_SessionAtiva_NaoBloqueia é o controle POSITIVO: uma sessão
// que É a dona ativa (ou que nunca fez claim algum) não é bloqueada.
func TestAuthAlice_SessionAtiva_NaoBloqueia(t *testing.T) {
	t.Run("dono ativo", func(t *testing.T) {
		db := newAuthTestDB(t)
		insertAuthUser(t, db, "u1", "tok-ativo", domain.HashToken("tok-ativo"))

		ownership := dbpkg.NewAccountOwnershipRepository(db)
		insertOwnershipRow(t, db, "claim-1", "5511@s.whatsapp.net", "noise", "u1", "u1", "active", 1)

		nextRan := false
		handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration), ownership)(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextRan = true
				w.WriteHeader(http.StatusOK)
			}))

		r := httptest.NewRequest(http.MethodGet, "/user/info", nil)
		r.Header.Set("token", "tok-ativo")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, queria 200 (corpo: %s)", rec.Code, rec.Body.String())
		}
		if !nextRan {
			t.Error("next handler não rodou para uma sessão ativa")
		}
	})

	t.Run("session_id nunca fez claim (found=false)", func(t *testing.T) {
		db := newAuthTestDB(t)
		insertAuthUser(t, db, "u9", "tok-sem-claim", domain.HashToken("tok-sem-claim"))

		ownership := dbpkg.NewAccountOwnershipRepository(db)
		nextRan := false
		handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration), ownership)(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextRan = true
				w.WriteHeader(http.StatusOK)
			}))

		r := httptest.NewRequest(http.MethodGet, "/user/info", nil)
		r.Header.Set("token", "tok-sem-claim")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, queria 200 — found=false não é superseded (corpo: %s)", rec.Code, rec.Body.String())
		}
		if !nextRan {
			t.Error("next handler não rodou")
		}
	})
}

// TestAuthAlice_OwnershipNil_PulaAChecagem trava o caso ownership==nil
// (mecanismo não wireado, ex.: modo `single`): o comportamento é IDÊNTICO ao
// de antes desta mudança — nunca bloqueia por causa de ownership.
func TestAuthAlice_OwnershipNil_PulaAChecagem(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "tok-sem-ownership", domain.HashToken("tok-sem-ownership"))

	nextRan := false
	handler := AuthAlice(db.DB, cache.New(cache.NoExpiration, cache.NoExpiration), nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextRan = true
			w.WriteHeader(http.StatusOK)
		}))

	r := httptest.NewRequest(http.MethodGet, "/user/info", nil)
	r.Header.Set("token", "tok-sem-ownership")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, queria 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if !nextRan {
		t.Error("next handler não rodou com ownership nil")
	}
}
