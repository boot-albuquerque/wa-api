// session_config_concurrency_test.go — a MEDIÇÃO da linha "Concorrência" do
// scorecard de produção (docs/PRODUCTION-READINESS.md, secção 6).
//
// A linha afirma: "sem ETag, If-Match ou versão. Duas escritas simultâneas ao
// mesmo recurso perdem uma, em silêncio". A afirmação é verdadeira, mas não
// pelo motivo que ela dá — e é isso que estes testes fixam.
//
// A escrita de `POST /session/proxy` NÃO é read-modify-write da linha inteira:
// `SaveProxyConfig` emite um único `UPDATE users SET proxy_url = ?,
// webhook_use_proxy = ? WHERE id = ?` (session_config_repository.go:23), que é
// atómico. Duas escritas concorrentes de campos DIFERENTES da tabela `users`
// não se perdem.
//
// O que se perde é o `webhook_use_proxy` de quem o declarou, quando um segundo
// pedido que NÃO o declarou o reintroduz a partir de uma leitura anterior:
// `resolveWebhookUseProxy` (set_proxy.go) LÊ a coluna e a devolve para ser
// REESCRITA junto com o novo `proxy_url`. Esse par leitura→escrita é a janela,
// e ela existe com ou sem ETag — um cabeçalho condicional só a fecharia se o
// servidor comparasse a versão DENTRO do mesmo UPDATE.
//
// # Porque estes testes usam o repositório REAL e não o dublê
//
// O dublê `contractsfake.ProxyConfigStore` guarda o valor num campo de struct.
// Um teste de concorrência sobre ele mediria a atomicidade do Go, não a do
// SQL — e a ARMADILHA 1 deste repositório é exactamente essa: dublê divergente
// da produção abençoa código morto. Aqui o store é
// `db.NewSessionConfigRepository` sobre SQLite real, com o mesmo
// `db.InitializeSchema` e os mesmos pragmas da produção.
//
// # Porque há um encontro marcado (rendezvous) e porque ele não é um dublê
//
// O decorador `rendezvousStore` DELEGA todas as chamadas ao repositório real e
// não muda resposta nenhuma. O que ele controla é apenas o ESCALONAMENTO: faz
// a leitura de B acontecer antes da escrita de A. Um teste de concorrência que
// não controla a ordem mede a sorte do escalonador — o
// TestSetProxy_Concorrencia_MedidaSemEncontroMarcado abaixo é a medição dessa
// sorte, e o valor que ele imprime mostra porque ela não serve de gate.
package storage_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/db"
)

// concurrencyUserID is the row every test of this file writes to.
const concurrencyUserID = "concurrency-user"

// proxyStoreOverRealSQLite builds the REAL SessionConfigRepository over a real
// SQLite database carrying the production schema, with one users row.
func proxyStoreOverRealSQLite(t *testing.T) (port.ProxyConfigStore, *sqlx.DB) {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/concurrency.db"+db.SQLitePragmas)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO users (id, name, token, webhook, jid, qrcode, connected, expiration, events, proxy_url, webhook_use_proxy)
		 VALUES (?, 'concurrency', '', '', '', '', 0, 0, 'All', '', 1)`,
		concurrencyUserID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return db.NewSessionConfigRepository(database), database
}

// storedProxyRow reads the two columns straight from SQLite — the observer is
// the database, never the use case's own answer.
func storedProxyRow(t *testing.T, database *sqlx.DB) (proxyURL string, webhookUseProxy bool) {
	t.Helper()
	row := database.QueryRowx(`SELECT proxy_url, webhook_use_proxy FROM users WHERE id = ?`, concurrencyUserID)
	if err := row.Scan(&proxyURL, &webhookUseProxy); err != nil {
		t.Fatalf("read back: %v", err)
	}
	return proxyURL, webhookUseProxy
}

// --- doubles that only add a mutex ----------------------------------------
//
// contractsfake.UserInfoSessionCache and contractsfake.SessionStatusReader
// append to unguarded slices; the package comment of contractsfake.Logger says
// it is "o único fake do pacote seguro para uso concorrente". Under -race the
// fakes would report their OWN data race and hide the one being measured.
// These two add a mutex and nothing else: no branch, no stored answer that
// production would not give.

type lockedProxyCache struct {
	mu   sync.Mutex
	urls []string
}

var _ port.UserInfoProxyCache = (*lockedProxyCache)(nil)

func (c *lockedProxyCache) SetProxy(_ context.Context, _, proxyURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.urls = append(c.urls, proxyURL)
}

// disconnectedStatus answers what the production guard answers for a session
// that is not connected (pkg/infra/noise/runtime/session/guard.go:72
// returns client.IsConnected()), which is the state in which a proxy may be
// configured at all. It is stateless, hence safe to share.
type disconnectedStatus struct{}

var _ port.SessionStatusReader = disconnectedStatus{}

func (disconnectedStatus) SessionStatus(context.Context, string) (bool, bool) { return false, false }

// concurrentLogger is port.Logger with a mutex and no recording — the tests of
// this file assert on the DATABASE, not on log lines.
type concurrentLogger struct{ mu sync.Mutex }

var _ port.Logger = (*concurrentLogger)(nil)

func (l *concurrentLogger) Debug(context.Context, string, ...any) { l.mu.Lock(); l.mu.Unlock() } //nolint:staticcheck // empty critical section is the point: it serialises nothing but proves the double is not the racer
func (l *concurrentLogger) Info(context.Context, string, ...any)  { l.mu.Lock(); l.mu.Unlock() } //nolint:staticcheck
func (l *concurrentLogger) Warn(context.Context, string, ...any)  { l.mu.Lock(); l.mu.Unlock() } //nolint:staticcheck
func (l *concurrentLogger) Error(context.Context, string, ...any) { l.mu.Lock(); l.mu.Unlock() } //nolint:staticcheck

// --- the rendezvous -------------------------------------------------------

// requestTagKey carries which of the two requests is running, so the decorator
// can tell them apart. The use case passes the same ctx it received to both
// store calls, so this is the seam that needs no production change.
type requestTagKey struct{}

const (
	tagDeclaresFlag = "A" // sends webhook_use_proxy explicitly
	tagOmitsFlag    = "B" // omits it, so the use case READS the stored one
)

// rendezvousStore delegates every call to the real repository and only orders
// them: B's read completes first, then A's write, then B's write.
type rendezvousStore struct {
	real      port.ProxyConfigStore
	bHasRead  chan struct{}
	aHasSaved chan struct{}
}

var _ port.ProxyConfigStore = (*rendezvousStore)(nil)

func (s *rendezvousStore) LoadWebhookUseProxy(ctx context.Context, userID string) (bool, error) {
	v, err := s.real.LoadWebhookUseProxy(ctx, userID)
	if tag(ctx) == tagOmitsFlag {
		close(s.bHasRead) // B read the pre-A value
		<-s.aHasSaved     // and only resumes after A has written
	}
	return v, err
}

func (s *rendezvousStore) SaveProxyConfig(ctx context.Context, userID, proxyURL string, webhookUseProxy bool) error {
	if tag(ctx) == tagDeclaresFlag {
		<-s.bHasRead // A writes only after B has read the value A is about to replace
	}
	err := s.real.SaveProxyConfig(ctx, userID, proxyURL, webhookUseProxy)
	if tag(ctx) == tagDeclaresFlag {
		close(s.aHasSaved)
	}
	return err
}

func tag(ctx context.Context) string {
	v, _ := ctx.Value(requestTagKey{}).(string)
	return v
}

const (
	proxyOfA = "http://203.0.113.10:3128"
	proxyOfB = "http://203.0.113.11:3128"
)

// TestSetProxy_ConcorrenciaPerdeOFlagDeclarado is the measurement.
//
// A declares webhook_use_proxy=false and is answered 200 saying so. B, whose
// payload never mentions the flag, had already read `true` and writes it back
// together with its own URL. The database ends with `true`: A's declaration is
// gone, and NOTHING in either answer says a write was overwritten.
//
// The test locks the state that was MEASURED, not the state that is wanted.
// When the gap is closed — a conditional write, a single UPDATE that keeps the
// column, or If-Match — this test must be inverted, not deleted: the assertion
// naming the loss is the one that has to start failing.
func TestSetProxy_ConcorrenciaPerdeOFlagDeclarado(t *testing.T) {
	real, database := proxyStoreOverRealSQLite(t)
	store := &rendezvousStore{
		real:      real,
		bHasRead:  make(chan struct{}),
		aHasSaved: make(chan struct{}),
	}
	uc := storage.NewSetProxyUseCase(disconnectedStatus{}, store, &lockedProxyCache{},
		defaultWebhookUseProxyForTest, rfc6761Resolver{}, &concurrentLogger{})

	declared := false
	ctxA := context.WithValue(context.Background(), requestTagKey{}, tagDeclaresFlag)
	ctxB := context.WithValue(context.Background(), requestTagKey{}, tagOmitsFlag)

	var wg sync.WaitGroup
	var resA *domain.ProxyConfigResult
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		resA, errA = uc.Execute(ctxA, concurrencyUserID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyOfA, WebhookUseProxy: &declared,
		})
	}()
	go func() {
		defer wg.Done()
		_, errB = uc.Execute(ctxB, concurrencyUserID, domain.ProxyConfigRequest{
			Enable: true, ProxyURL: proxyOfB,
		})
	}()
	wg.Wait()

	if errA != nil || errB != nil {
		t.Fatalf("os dois pedidos deviam ter sido aceites: errA=%v errB=%v", errA, errB)
	}
	if resA == nil || resA.WebhookUseProxy == nil || *resA.WebhookUseProxy != false {
		t.Fatalf("o pedido A devia ter respondido webhook_use_proxy=false; respondeu %+v", resA)
	}

	url, flag := storedProxyRow(t, database)
	if url != proxyOfB {
		t.Fatalf("o UPDATE de B devia ter ficado como o último; proxy_url=%q", url)
	}
	if !flag {
		t.Fatalf("MUDANÇA DE COMPORTAMENTO: webhook_use_proxy=false sobreviveu à escrita concorrente de B.\n" +
			"Este teste travava a PERDA medida em 2026-08-26 (F292). Se a perda foi corrigida, inverta a\n" +
			"asserção e actualize docs/PRODUCTION-READINESS.md.")
	}

	// The loss, stated as the two facts that make it silent: A was told its
	// value was stored, and the database holds the opposite.
	t.Logf("perda silenciosa confirmada: A respondeu webhook_use_proxy=%v, o banco tem %v (proxy_url=%q)",
		*resA.WebhookUseProxy, flag, url)
}

// TestSetProxy_Sequencial_NaoPerdeNada is the EXECUTED negative control of the
// test above: the same two payloads, the same use case, the same real store —
// only without the interleaving. B now reads the value A already wrote, and
// writes it back unchanged. Nothing is lost.
//
// It is what proves the assertion above is measuring the read-modify-write
// window and not the payloads: if the loss came from B's body, it would happen
// here too.
func TestSetProxy_Sequencial_NaoPerdeNada(t *testing.T) {
	store, database := proxyStoreOverRealSQLite(t)
	uc := storage.NewSetProxyUseCase(disconnectedStatus{}, store, &lockedProxyCache{},
		defaultWebhookUseProxyForTest, rfc6761Resolver{}, &concurrentLogger{})

	declared := false
	ctx := context.Background()
	if _, err := uc.Execute(ctx, concurrencyUserID, domain.ProxyConfigRequest{
		Enable: true, ProxyURL: proxyOfA, WebhookUseProxy: &declared,
	}); err != nil {
		t.Fatalf("A: %v", err)
	}
	if _, err := uc.Execute(ctx, concurrencyUserID, domain.ProxyConfigRequest{
		Enable: true, ProxyURL: proxyOfB,
	}); err != nil {
		t.Fatalf("B: %v", err)
	}

	url, flag := storedProxyRow(t, database)
	if url != proxyOfB {
		t.Fatalf("proxy_url devia ser o de B; é %q", url)
	}
	if flag {
		t.Fatalf("sem entrelaçamento o webhook_use_proxy=false de A tinha de sobreviver; o banco tem true")
	}
}

// TestSetProxy_BDeclaraOFlag_NaoEPerdaEUltimaEscrita is the second discriminator.
//
// Under the SAME rendezvous, B now declares webhook_use_proxy itself, so
// resolveWebhookUseProxy never reads the database and there is no window. B's
// value wins because B wrote last — which is last-write-wins on a payload the
// caller actually sent, not a lost update. Without this test the first one
// could be read as "the second write always wins", which is a different and
// much less interesting claim.
func TestSetProxy_BDeclaraOFlag_NaoEPerdaEUltimaEscrita(t *testing.T) {
	real, database := proxyStoreOverRealSQLite(t)
	store := &rendezvousStore{
		real:      real,
		bHasRead:  make(chan struct{}),
		aHasSaved: make(chan struct{}),
	}
	// B declares the flag, so its LoadWebhookUseProxy is never called and the
	// rendezvous would deadlock. Releasing bHasRead upfront keeps the two
	// writes concurrent without waiting for a read that will not happen.
	close(store.bHasRead)

	uc := storage.NewSetProxyUseCase(disconnectedStatus{}, store, &lockedProxyCache{},
		defaultWebhookUseProxyForTest, rfc6761Resolver{}, &concurrentLogger{})

	aValue, bValue := false, false
	ctxA := context.WithValue(context.Background(), requestTagKey{}, tagDeclaresFlag)
	ctxB := context.WithValue(context.Background(), requestTagKey{}, tagOmitsFlag)

	if _, err := uc.Execute(ctxA, concurrencyUserID, domain.ProxyConfigRequest{
		Enable: true, ProxyURL: proxyOfA, WebhookUseProxy: &aValue,
	}); err != nil {
		t.Fatalf("A: %v", err)
	}
	if _, err := uc.Execute(ctxB, concurrencyUserID, domain.ProxyConfigRequest{
		Enable: true, ProxyURL: proxyOfB, WebhookUseProxy: &bValue,
	}); err != nil {
		t.Fatalf("B: %v", err)
	}

	url, flag := storedProxyRow(t, database)
	if url != proxyOfB || flag {
		t.Fatalf("com os dois a declararem o flag, o banco devia ter o par de B (%q,false); tem (%q,%v)",
			proxyOfB, url, flag)
	}
}

// TestSetProxy_Concorrencia_MedidaSemEncontroMarcado is the MEASUREMENT of how
// often the window closes on its own, and the reason the gate above needs the
// rendezvous.
//
// It asserts only the invariant that HOLDS — the row is never torn: the stored
// proxy_url always belongs to one of the two requests, never a mix — and
// reports the loss count with t.Logf. A count is not an assertion on purpose:
// making it a gate would make the suite depend on the scheduler.
func TestSetProxy_Concorrencia_MedidaSemEncontroMarcado(t *testing.T) {
	const rodadas = 200
	perdas := 0

	for i := 0; i < rodadas; i++ {
		store, database := proxyStoreOverRealSQLite(t)
		uc := storage.NewSetProxyUseCase(disconnectedStatus{}, store, &lockedProxyCache{},
			defaultWebhookUseProxyForTest, rfc6761Resolver{}, &concurrentLogger{})

		declared := false
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = uc.Execute(context.Background(), concurrencyUserID, domain.ProxyConfigRequest{
				Enable: true, ProxyURL: proxyOfA, WebhookUseProxy: &declared,
			})
		}()
		go func() {
			defer wg.Done()
			_, _ = uc.Execute(context.Background(), concurrencyUserID, domain.ProxyConfigRequest{
				Enable: true, ProxyURL: proxyOfB,
			})
		}()
		wg.Wait()

		url, flag := storedProxyRow(t, database)
		if url != proxyOfA && url != proxyOfB {
			t.Fatalf("rodada %d: linha rasgada — proxy_url=%q não é de nenhum dos dois pedidos", i, url)
		}
		// A asked for false and got an answer saying so; the row holding true
		// means B's stale read won.
		if flag {
			perdas++
		}
	}

	t.Log(fmt.Sprintf("perdas naturais em %d rodadas: %d (%.1f%%) — sem encontro marcado a janela fecha por sorte do escalonador",
		rodadas, perdas, 100*float64(perdas)/float64(rodadas)))
}
