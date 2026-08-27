package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"

	"github.com/patrickmn/go-cache"
)

// AUDITORIA ADVERSARIAL (feature/capability-final-audit) — invariante 5:
// "uma sessão superseded NUNCA volta a operar". Os testes existentes em
// auth_ownership_test.go (TestAuthAlice_SessionSuperseded_E409 etc.) sempre
// passam por um MISS no userCache (cada teste usa um cache novo). Este
// arquivo ataca especificamente o caminho de CACHE HIT: será que uma
// requisição cujo usuário já está em cache (e portanto não bate no banco
// para resolver `Values`) ainda assim é barrada quando a sessão é
// superseded DEPOIS de entrar no cache?
//
// Lendo auth.go: a checagem de ownership (linha ~267) fica FORA do
// `if !found { ... } else { ... }` que resolve Values — ela roda
// incondicionalmente após os dois ramos, usando sempre `ownership.
// CurrentStatusForSession(ctx, txtid)`, uma consulta ao banco que não passa
// pelo cache. A hipótese adversarial é que essa leitura fosse otimizada para
// só rodar no cache-miss (não é o caso aqui, mas o teste trava o
// comportamento correto e serve de controle negativo).
func TestAudit_AuthAlice_CacheHitStillEnforcesSupersede(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "tok-cache-hit", domain.HashToken("tok-cache-hit"))

	ownership := dbpkg.NewAccountOwnershipRepository(db)
	insertOwnershipRow(t, db, "claim-1", "5511@s.whatsapp.net", "wa_noise", "u1", "u1", "active", 1)

	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	nextRanCount := 0
	handler := AuthAlice(db.DB, userCache, ownership)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextRanCount++
			w.WriteHeader(http.StatusOK)
		}))

	// Requisição 1: preenche o cache (u1 está ativo).
	r1 := httptest.NewRequest(http.MethodGet, "/user/info", nil)
	r1.Header.Set("token", "tok-cache-hit")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, r1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("requisição 1: status = %d, queria 200 (corpo: %s)", rec1.Code, rec1.Body.String())
	}
	if _, found := userCache.Get("tok-cache-hit"); !found {
		t.Fatal("pré-condição falhou: token não ficou em cache após a requisição 1")
	}

	// u2 supersede u1 SEM tocar no userCache — só na tabela account_ownership,
	// exatamente como um segundo processo faria em produção.
	if _, err := db.Exec(`UPDATE account_ownership SET status='superseded', superseded_by_session_id='u2' WHERE id='claim-1'`); err != nil {
		t.Fatalf("marcando claim-1 como superseded: %v", err)
	}
	insertOwnershipRow(t, db, "claim-2", "5511@s.whatsapp.net", "wa_noise", "u2", "u2", "active", 2)

	// Requisição 2: MESMO token, agora com cache HIT garantido (Values já
	// está no cache; não passa pelo SELECT em `users`). Se a checagem de
	// ownership dependesse do caminho de cache-miss, esta requisição
	// passaria — ela NÃO deve passar.
	r2 := httptest.NewRequest(http.MethodGet, "/user/info", nil)
	r2.Header.Set("token", "tok-cache-hit")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, r2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("QUEBRARIA invariante 5 se isto passasse: requisição 2 (cache hit, sessão superseded) "+
			"status = %d, queria 409 session_superseded (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	if nextRanCount != 1 {
		t.Fatalf("next handler rodou %d vezes; queria exatamente 1 (só a requisição 1, pré-supersede)", nextRanCount)
	}
	t.Log("RESISTIU: cache hit não contorna a checagem de ownership — CurrentStatusForSession roda incondicionalmente")
}

// TestAudit_AuthAlice_ConcurrentRequestsAfterSupersede ataca com concorrência
// real (goroutines) em vez de duas chamadas sequenciais: depois que a sessão
// é superseded, várias requisições concorrentes com o token antigo devem
// TODAS ser rejeitadas — não há janela em que uma delas "escorregue" por
// alguma condição de corrida na leitura do cache ou do banco.
func TestAudit_AuthAlice_ConcurrentRequestsAfterSupersede(t *testing.T) {
	db := newAuthTestDB(t)
	insertAuthUser(t, db, "u1", "tok-concurrent", domain.HashToken("tok-concurrent"))

	ownership := dbpkg.NewAccountOwnershipRepository(db)
	insertOwnershipRow(t, db, "claim-1", "5511@s.whatsapp.net", "wa_noise", "u1", "u1", "superseded", 1)
	insertOwnershipRow(t, db, "claim-2", "5511@s.whatsapp.net", "wa_noise", "u2", "u2", "active", 2)

	userCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	handler := AuthAlice(db.DB, userCache, ownership)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	const n = 50
	results := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			r := httptest.NewRequest(http.MethodGet, "/user/info", nil)
			r.Header.Set("token", "tok-concurrent")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r.WithContext(context.Background()))
			results <- rec.Code
		}()
	}

	for i := 0; i < n; i++ {
		code := <-results
		if code != http.StatusConflict {
			t.Fatalf("requisição concorrente #%d: status = %d, queria 409 em TODAS", i, code)
		}
	}
	t.Log("RESISTIU: 50 requisições concorrentes com token de sessão superseded, todas rejeitadas com 409")
}
