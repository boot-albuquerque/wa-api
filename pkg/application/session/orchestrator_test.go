package session

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
)

// emptyDriver é um driver database/sql que aceita qualquer query e devolve
// zero linhas. Existe porque *sql.Row não pode ser fabricado fora do pacote
// database/sql (o zero-value entra em pânico no Scan), e userStore.QueryRow
// tem esse tipo concreto no retorno. Zero linhas faz Scan devolver
// sql.ErrNoRows, que é o caminho degradado "sem proxy" do orchestrator.
type emptyDriver struct{}

func (emptyDriver) Open(string) (driver.Conn, error) { return emptyConn{}, nil }

type emptyConn struct{}

func (emptyConn) Prepare(string) (driver.Stmt, error) { return emptyStmt{}, nil }
func (emptyConn) Close() error                        { return nil }
func (emptyConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }

type emptyStmt struct{}

func (emptyStmt) Close() error                               { return nil }
func (emptyStmt) NumInput() int                              { return -1 }
func (emptyStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (emptyStmt) Query([]driver.Value) (driver.Rows, error)  { return &emptyRows{}, nil }

type emptyRows struct{}

func (*emptyRows) Columns() []string         { return []string{"proxy_url", "webhook_use_proxy"} }
func (*emptyRows) Close() error              { return nil }
func (*emptyRows) Next([]driver.Value) error { return io.EOF }

func init() { sql.Register("wa-api-empty", emptyDriver{}) }

// fakeUserStore implementa a superfície userStore, registrando as escritas e
// delegando as leituras ao emptyDriver.
type fakeUserStore struct {
	db *sql.DB

	execQueries []string
	execArgs    [][]any
	execErr     error
}

func newFakeUserStore(t *testing.T) *fakeUserStore {
	t.Helper()
	db, err := sql.Open("wa-api-empty", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &fakeUserStore{db: db}
}

func (f *fakeUserStore) Exec(query string, args ...any) (sql.Result, error) {
	f.execQueries = append(f.execQueries, query)
	f.execArgs = append(f.execArgs, args)
	return nil, f.execErr
}

func (f *fakeUserStore) QueryRow(query string, args ...any) *sql.Row {
	return f.db.QueryRow(query, args...)
}

type harness struct {
	provider   *contractsfake.SessionProvider
	session    *contractsfake.Session
	registry   *contractsfake.SessionRegistry
	dispatcher *contractsfake.SessionEventDispatcher
	attach     *contractsfake.SessionAttachHook
	db         *fakeUserStore
	recorder   *contractsfake.CallRecorder
	orch       *Orchestrator
}

func newHarness(t *testing.T, opts ...Option) *harness {
	t.Helper()

	rec := &contractsfake.CallRecorder{}
	sess := &contractsfake.Session{Recorder: rec}
	h := &harness{
		provider:   &contractsfake.SessionProvider{Recorder: rec, NewSessionResult: sess},
		session:    sess,
		registry:   &contractsfake.SessionRegistry{Recorder: rec},
		dispatcher: &contractsfake.SessionEventDispatcher{Recorder: rec},
		attach:     &contractsfake.SessionAttachHook{Recorder: rec},
		db:         newFakeUserStore(t),
		recorder:   rec,
	}

	// Sleep no-op por padrão: os testes de retry não devem gastar tempo real.
	opts = append([]Option{WithSleep(func(time.Duration) {})}, opts...)
	h.orch = NewOrchestrator(h.provider, h.registry, h.dispatcher, h.attach, h.db, opts...)
	return h
}

func indexOf(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

// --- Ordem Attach → Pair/Connect ---------------------------------------

func TestStartAttachesBeforeConnectWhenPaired(t *testing.T) {
	h := newHarness(t)
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	attachAt := indexOf(h.recorder.Calls, "SessionAttachHook.Attach")
	connectAt := indexOf(h.recorder.Calls, "Session.Connect")
	if attachAt < 0 || connectAt < 0 {
		t.Fatalf("esperava Attach e Connect na sequência, obtive %v", h.recorder.Calls)
	}
	if attachAt >= connectAt {
		t.Fatalf("Attach (%d) deve ocorrer antes de Connect (%d): %v", attachAt, connectAt, h.recorder.Calls)
	}
}

func TestStartAttachesBeforePairWhenUnpaired(t *testing.T) {
	h := newHarness(t)
	// Zero-value de HasCredentials já é false; canal fechado encerra o
	// consumo de eventos de pareamento imediatamente.
	events := make(chan port.PairingEvent)
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	attachAt := indexOf(h.recorder.Calls, "SessionAttachHook.Attach")
	pairAt := indexOf(h.recorder.Calls, "Session.Pair")
	if attachAt < 0 || pairAt < 0 {
		t.Fatalf("esperava Attach e Pair na sequência, obtive %v", h.recorder.Calls)
	}
	if attachAt >= pairAt {
		t.Fatalf("Attach (%d) deve ocorrer antes de Pair (%d): %v", attachAt, pairAt, h.recorder.Calls)
	}
	if len(h.session.ConnectCalls) != 0 {
		t.Fatalf("caminho sem credenciais não deve chamar Connect")
	}
}

// --- Retry de conexão ---------------------------------------------------

func TestConnectRetriesUntilSuccess(t *testing.T) {
	h := newHarness(t)
	h.session.HasCredentialsFunc = func() bool { return true }

	attempts := 0
	h.session.ConnectFunc = func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("boom")
		}
		return nil
	}

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(h.session.ConnectCalls) != 3 {
		t.Fatalf("esperava 3 tentativas de Connect, obtive %d", len(h.session.ConnectCalls))
	}
	if len(h.attach.DetachCalls) != 0 {
		t.Fatalf("sucesso na conexão não deve desmontar a sessão")
	}
}

func TestConnectExhaustsRetriesAndTearsDown(t *testing.T) {
	h := newHarness(t)
	h.session.HasCredentialsFunc = func() bool { return true }
	h.session.ConnectFunc = func(context.Context) error { return errors.New("boom") }

	err := h.orch.Start(context.Background(), "u1", "tok")
	if err == nil {
		t.Fatal("esperava erro após esgotar as tentativas")
	}
	// defaultMaxConnectionRetries = 3.
	if len(h.session.ConnectCalls) != defaultMaxConnectionRetries {
		t.Fatalf("esperava %d tentativas, obtive %d", defaultMaxConnectionRetries, len(h.session.ConnectCalls))
	}
	if got := h.dispatcher.DispatchedTypes(); len(got) != 1 || got[0] != "ConnectFailure" {
		t.Fatalf("esperava despacho de ConnectFailure, obtive %v", got)
	}
	// Webhook antes do kill: Dispatch deve preceder Detach.
	if indexOf(h.recorder.Calls, "SessionEventDispatcher.Dispatch") >= indexOf(h.recorder.Calls, "SessionAttachHook.Detach") {
		t.Fatalf("Dispatch deve preceder Detach: %v", h.recorder.Calls)
	}
	if len(h.attach.DetachCalls) != 1 || len(h.registry.UnregisterCalls) != 1 {
		t.Fatalf("esperava Unregister e Detach exatamente uma vez: %v", h.recorder.Calls)
	}
}

func TestWithRetryPolicyOverridesAttempts(t *testing.T) {
	var waits []time.Duration
	h := newHarness(t,
		WithRetryPolicy(2, time.Second),
		WithSleep(func(d time.Duration) { waits = append(waits, d) }),
	)
	h.session.HasCredentialsFunc = func() bool { return true }
	h.session.ConnectFunc = func(context.Context) error { return errors.New("boom") }

	if err := h.orch.Start(context.Background(), "u1", "tok"); err == nil {
		t.Fatal("esperava erro")
	}
	if len(h.session.ConnectCalls) != 2 {
		t.Fatalf("esperava 2 tentativas, obtive %d", len(h.session.ConnectCalls))
	}
	// Backoff linear: espera só antes da 2ª tentativa, de 1*baseWait.
	if len(waits) != 1 || waits[0] != time.Second {
		t.Fatalf("esperava um único wait de 1s, obtive %v", waits)
	}
}

// --- Escritas em banco --------------------------------------------------

// TestOrchestratorNeverWritesConnected fixa uma escolha de design declarada em
// orchestrator.go: o orchestrator não é o escritor de users.connected. Quem
// marca connected=1 é o handler de domínio (evento Connected) e quem marca
// connected=0 é a goroutine de kill do SessionAttachHook — escritor único por
// caminho. Como não dá para testar uma ausência diretamente, o teste percorre
// os caminhos que escrevem no banco (QR, timeout, sucesso de pareamento e
// falha de conexão) e assere que toda escrita mira apenas a coluna qrcode.
func TestOrchestratorNeverWritesConnected(t *testing.T) {
	t.Run("pairing", func(t *testing.T) {
		h := newHarness(t)
		events := make(chan port.PairingEvent, 3)
		events <- port.PairingEvent{Kind: port.PairingEventKindQR, Code: "abc", Timeout: 20 * time.Second}
		events <- port.PairingEvent{Kind: port.PairingEventKindSuccess}
		events <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
		close(events)
		h.session.PairingEvents = events

		if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
			t.Fatalf("Start: %v", err)
		}
		assertOnlyQRCodeWrites(t, h.db.execQueries)
		if len(h.db.execQueries) == 0 {
			t.Fatal("esperava ao menos uma escrita no caminho de pareamento")
		}
	})

	t.Run("connect failure", func(t *testing.T) {
		h := newHarness(t)
		h.session.HasCredentialsFunc = func() bool { return true }
		h.session.ConnectFunc = func(context.Context) error { return errors.New("boom") }

		if err := h.orch.Start(context.Background(), "u1", "tok"); err == nil {
			t.Fatal("esperava erro")
		}
		assertOnlyQRCodeWrites(t, h.db.execQueries)
	})
}

func assertOnlyQRCodeWrites(t *testing.T, queries []string) {
	t.Helper()
	for _, q := range queries {
		if !strings.Contains(q, "qrcode") {
			t.Fatalf("escrita inesperada no banco: %q", q)
		}
		if strings.Contains(q, "connected") {
			t.Fatalf("orchestrator não deve escrever users.connected: %q", q)
		}
	}
}

// --- Caminhos de erro ---------------------------------------------------

func TestStartReturnsNewSessionError(t *testing.T) {
	h := newHarness(t)
	want := errors.New("no session")
	h.provider.NewSessionFunc = func(context.Context, port.SessionSpec) (port.Session, error) {
		return nil, want
	}

	if err := h.orch.Start(context.Background(), "u1", "tok"); !errors.Is(err, want) {
		t.Fatalf("esperava %v, obtive %v", want, err)
	}
	if len(h.registry.RegisterCalls) != 0 || len(h.attach.AttachCalls) != 0 {
		t.Fatalf("falha em NewSession não deve registrar nem anexar: %v", h.recorder.Calls)
	}
}

func TestStartUnregistersWhenAttachFails(t *testing.T) {
	h := newHarness(t)
	want := errors.New("attach failed")
	h.attach.AttachFunc = func(context.Context, string, string) error { return want }

	if err := h.orch.Start(context.Background(), "u1", "tok"); !errors.Is(err, want) {
		t.Fatalf("esperava %v, obtive %v", want, err)
	}
	if len(h.registry.UnregisterCalls) != 1 || h.registry.UnregisterCalls[0].UserID != "u1" {
		t.Fatalf("esperava Unregister de u1 após falha de Attach: %v", h.registry.UnregisterCalls)
	}
	if len(h.session.PairCalls) != 0 || len(h.session.ConnectCalls) != 0 {
		t.Fatal("falha em Attach não deve prosseguir para Pair/Connect")
	}
}

func TestStartReturnsPairError(t *testing.T) {
	h := newHarness(t)
	want := errors.New("pair failed")
	h.session.PairFunc = func(context.Context) (<-chan port.PairingEvent, error) { return nil, want }

	if err := h.orch.Start(context.Background(), "u1", "tok"); !errors.Is(err, want) {
		t.Fatalf("esperava %v, obtive %v", want, err)
	}
}

// --- Eventos de sessão e Stop ------------------------------------------

func TestSessionEventsAreDispatched(t *testing.T) {
	h := newHarness(t)
	h.session.HasCredentialsFunc = func() bool { return true }
	h.session.ConnectFunc = func(context.Context) error {
		// Emitido enquanto a inscrição está viva (unsubscribe é deferido).
		h.session.Emit(port.SessionEvent{Kind: port.SessionEventKindConnected})
		h.session.Emit(port.SessionEvent{
			Kind:         port.SessionEventKindDisconnected,
			Disconnected: &port.SessionDisconnectedEvent{Reason: "bye"},
		})
		return nil
	}

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := h.dispatcher.DispatchedTypes()
	if len(got) != 2 || got[0] != "Connected" || got[1] != "Disconnected" {
		t.Fatalf("esperava [Connected Disconnected], obtive %v", got)
	}
	if reason := h.dispatcher.DispatchCalls[1].Payload["reason"]; reason != "bye" {
		t.Fatalf("esperava reason=bye, obtive %v", reason)
	}
}

func TestStopUnregistersAndDetaches(t *testing.T) {
	h := newHarness(t)
	h.orch.Stop("u1")

	if len(h.registry.UnregisterCalls) != 1 || len(h.attach.DetachCalls) != 1 {
		t.Fatalf("Stop deve chamar Unregister e Detach: %v", h.recorder.Calls)
	}
}
