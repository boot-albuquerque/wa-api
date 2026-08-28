package session

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"

	"net/http"
	"wa-api/pkg/domain/apperr"
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

// Session ownership (ADR-0005, D2).
//
// These tests exist because of a REAL gap found by measurement, not by review:
// the ownership filter covered only connectOnStartup, so a session connected at
// runtime (the /session/connect the dev panel and every new pairing use) ran
// with NO lease at all. Measured on 2026-08-08 with two live sessions:
//
//	teste-d2    connected=1  owner: MacBook-Pro-de-Lucas.local-80338
//	teste-d2-b  connected=1  owner: NO LEASE          <- paired via the panel
//
// Under N replicas that is the F89 disaster: the next replica sees connected=1,
// finds the lease free, takes it, connects the same session, and WhatsApp kills
// one of them permanently.

func TestStart_RefusesWhenOwnershipIsDenied(t *testing.T) {
	h := newHarness(t, WithOwnershipCheck(func(string) bool { return false }, nil))
	// Paired on purpose, even though the guard should stop us first: if the
	// guard is ever removed, Start would otherwise enter the BLOCKING
	// pairing path and this test would hang instead of failing. A hanging
	// test reports nothing (ARMADILHAS.md 16) — the negative control for
	// this guard timed out at 63s before this line existed.
	h.session.HasCredentialsFunc = func() bool { return true }

	err := h.orch.Start(context.Background(), "user-1", "token-1")
	if err == nil {
		t.Fatal("Start succeeded for a session owned by another replica")
	}

	// The guard must run BEFORE anything is materialized. Creating the session
	// and only then discovering it belongs elsewhere would leave client and
	// registries dirty, and the cleanup path would have to undo work that
	// should never have started.
	if indexOf(h.recorder.Calls, "SessionProvider.NewSession") >= 0 {
		t.Errorf("session was materialized despite the ownership refusal: %v", h.recorder.Calls)
	}
}

// TestStart_OwnershipRefusalIsClassified applies the F93 lesson: a raw error
// reaches the HTTP layer as an opaque 500. The refusal is not a server fault —
// the request simply reached the wrong replica — so it has to carry a category.
func TestStart_OwnershipRefusalIsClassified(t *testing.T) {
	h := newHarness(t, WithOwnershipCheck(func(string) bool { return false }, nil))
	// Paired on purpose, even though the guard should stop us first: if the
	// guard is ever removed, Start would otherwise enter the BLOCKING
	// pairing path and this test would hang instead of failing. A hanging
	// test reports nothing (ARMADILHAS.md 16) — the negative control for
	// this guard timed out at 63s before this line existed.
	h.session.HasCredentialsFunc = func() bool { return true }

	err := h.orch.Start(context.Background(), "user-1", "token-1")
	if err == nil {
		t.Fatal("Start succeeded for a session owned by another replica")
	}

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error is not an *apperr.AppError, so the HTTP layer turns it into an opaque 500: %T", err)
	}
	if appErr.Code != codeSessionOwnedByAnotherReplica {
		t.Errorf("code = %q, want %q", appErr.Code, codeSessionOwnedByAnotherReplica)
	}
	// 409 specifically, not merely "not 5xx": the request is well formed and
	// authorized, it just reached the wrong replica. 400 would tell the client
	// to fix a payload that has nothing wrong with it (F95).
	if status := appErr.Category.HTTPStatus(); status != http.StatusConflict {
		t.Errorf("category maps to HTTP %d, want %d: the caller should route to the owner, not fix the request",
			status, http.StatusConflict)
	}
	if appErr.Retryable {
		t.Error("marked retryable: repeating the same request against the same replica yields the same refusal")
	}
}

// TestStart_ProceedsWhenOwnershipIsGranted: the guard must not block the normal
// path. A check that always refuses would pass the test above and break
// everything.
func TestStart_ProceedsWhenOwnershipIsGranted(t *testing.T) {
	h := newHarness(t, WithOwnershipCheck(func(string) bool { return true }, nil))
	// Already paired: the unpaired path BLOCKS consuming the pairing channel,
	// so without this the test hangs instead of failing — and a hanging test
	// reports nothing (ARMADILHAS.md 16).
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start failed with ownership granted: %v", err)
	}
	if indexOf(h.recorder.Calls, "SessionProvider.NewSession") < 0 {
		t.Errorf("session was not materialized despite ownership being granted: %v", h.recorder.Calls)
	}
}

// TestStart_WithoutOwnershipCheckAllows pins `single` mode: with no check
// installed there is nobody to compete with, and sessions must start normally.
func TestStart_WithoutOwnershipCheckAllows(t *testing.T) {
	h := newHarness(t) // no WithOwnershipCheck
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start failed with no ownership check installed: %v", err)
	}
	if indexOf(h.recorder.Calls, "SessionProvider.NewSession") < 0 {
		t.Errorf("session was not materialized in single mode: %v", h.recorder.Calls)
	}
}

// TestStart_ReleasesOwnershipWhenSessionFailsToStart pins F96, a defect
// INTRODUCED by the ownership guard itself.
//
// Ownership is claimed BEFORE the session is materialized, on purpose: a denial
// must not leave client and registries dirty. The cost is that a failure
// afterwards would keep the lease alive forever — the heartbeat renewing
// ownership of a session that never came up, and no other replica ever able to
// take that user.
//
// Measured before the fix, with a user connected via the API but never paired:
//
//	t=6s   connected=0  expires_in=12s
//	t=12s  connected=0  expires_in=11s
//	t=18s  connected=0  expires_in=15s   <- renewed
func TestStart_ReleasesOwnershipWhenSessionFailsToStart(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)
	h.provider.NewSessionFunc = func(context.Context, port.SessionSpec) (port.Session, error) {
		return nil, errors.New("provider refused")
	}

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err == nil {
		t.Fatal("Start succeeded even though the provider failed")
	}

	if len(released) != 1 || released[0] != "user-1" {
		t.Errorf("ownership released = %v, want [user-1]: the lease would be renewed forever for a session that never came up", released)
	}
}

// TestStart_KeepsOwnershipOnSuccess is the other half, and the one that stops
// the fix above from becoming a worse bug: releasing on the SUCCESS path would
// hand the session away while it is being served.
func TestStart_KeepsOwnershipOnSuccess(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(released) != 0 {
		t.Errorf("ownership released on the SUCCESS path (%v): the session would be given away while running", released)
	}
}

// TestStart_DoesNotReleaseWhenClaimWasDenied: we never held it, so releasing
// would delete ANOTHER replica's lease — handing it a session it is serving.
func TestStart_DoesNotReleaseWhenClaimWasDenied(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return false },
			func(userID string) { released = append(released, userID) },
		),
	)
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err == nil {
		t.Fatal("Start succeeded despite the ownership refusal")
	}

	if len(released) != 0 {
		t.Errorf("released ownership we never held (%v): this would delete the lease of the replica that owns the session", released)
	}
}

// TestStart_ReleasesOwnershipWhenPairingTimesOut pins F98 — the half of F96
// that the fix above does NOT reach.
//
// The distinction is the whole point. The F96 defer fires when Start RETURNS an
// error. On the QR path Start already answered 200 {"status":"connecting"} and
// the pairing dies later, asynchronously, inside the goroutine consuming the
// events. runPairing then returns nil, so the defer never runs.
//
// Measured on the bench before the fix (Postgres, multi mode):
//
//	23:11:26  GET /session/connect -> 200 {"status":"connecting"}
//	23:13:06  log: "QR timeout killing channel"   <- pairing died here
//	23:16:41  lease still alive, expires_at renewed
//
// Under N pods that pins the user to the replica where they walked away from
// the QR: no other replica can ever take them, and /session/connect elsewhere
// answers 409 forever.
func TestStart_ReleasesOwnershipWhenPairingTimesOut(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)

	events := make(chan port.PairingEvent, 2)
	events <- port.PairingEvent{Kind: port.PairingEventKindQR, Code: "abc", Timeout: 20 * time.Second}
	events <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
	close(events)
	h.session.PairingEvents = events

	// Start returns nil here, and that is exactly the trap: a test asserting on
	// the returned error would pass both before and after the fix.
	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(released) != 1 || released[0] != "user-1" {
		t.Errorf("ownership released = %v, want [user-1]: the lease is renewed forever for a pairing that ended in nothing", released)
	}
}

// TestStart_KeepsOwnershipWhenPairingSucceeds is the control in the opposite
// direction: releasing on a pairing that WORKED would hand the session away the
// moment the user finishes scanning — turning the fix into a worse defect than
// the leak it repairs.
func TestStart_KeepsOwnershipWhenPairingSucceeds(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)

	events := make(chan port.PairingEvent, 2)
	events <- port.PairingEvent{Kind: port.PairingEventKindQR, Code: "abc", Timeout: 20 * time.Second}
	events <- port.PairingEvent{Kind: port.PairingEventKindSuccess}
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(released) != 0 {
		t.Errorf("ownership released after a SUCCESSFUL pairing (%v): the session would be given away right after the scan", released)
	}
}

// TestStart_PairingTimeoutWithoutOwnershipCheck pins `single` mode on the same
// path: with no release function installed, a QR timeout must not panic on a
// nil call.
func TestStart_PairingTimeoutWithoutOwnershipCheck(t *testing.T) {
	h := newHarness(t) // no WithOwnershipCheck

	events := make(chan port.PairingEvent, 1)
	events <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// --- F153: the QR-timeout that kills a session that JUST paired ---------
//
// Measured in production, real account, two consecutive pairing attempts
// against the same user:
//
//	First attempt (broke):
//	  12:00:02  loggedIn=TRUE                     <- authenticated
//	  12:00:02  Offline sync completed
//	  12:00:03  Message Received id=3AE00A2FF...   <- real messages arriving
//	  12:00:03  Message Received id=3A42A5C18...
//	  12:00:03  WARN  QR timeout killing channel   <- torn down anyway
//	  12:00:03  INFO  Received kill signal
//	  12:00:03  ERROR Failed to do initial fetch of app state (x5)
//	  12:00:03  no session
//
//	Second attempt (survived): connected=true, loggedIn=true, jid populated,
//	qrcode cleared, stable for 30s+, no "QR timeout killing channel" line.
//	(Log preserved at /tmp/waapi-live/server.log.)
//
// Root cause traced to internal/wa-noise/core/qrchan.go: emitQRs (a
// goroutine driven by a purely LOCAL per-QR-code timer) and handleEvent (a
// goroutine driven by the REAL PairSuccess arriving over the websocket) both
// race for a single atomic CAS on qrc.closed. Only the CAS winner's item
// (success XOR timeout — never both) ever reaches sess.Pair()'s output
// channel; the loser is dropped silently inside the SDK
// ("Got status ..., but channel is already closed"). When the local timer
// wins, this orchestrator's pairing channel sees ONLY "timeout", even though
// the session authenticated seconds earlier — confirmed by the completely
// separate session-event bus (Connected/PairSuccess, registered before the
// QR-channel's own handler in Start, and never gated by that CAS). Whether
// the timer or the network wins is a genuine, non-deterministic race,
// exactly matching why the SAME account paired twice with two different
// outcomes.
//
// Invariant written into runPairing: once pairing is CONFIRMED by the
// session-event bus, no later item from the QR-pairing channel — timeout or
// a stale QR code — may tear the session down or rewrite its QR state.
// Before that confirmation, timeout must still tear everything down, as it
// always has (F98).

// TestStart_KeepsSessionWhenChannelTimeoutArrivesAfterConfirmedPairing is
// the direct reproduction of the field defect: the session-event bus
// confirms Connected BEFORE this orchestrator reads the QR channel's own
// "timeout" item — the exact ordering measured in the broken attempt above.
func TestStart_KeepsSessionWhenChannelTimeoutArrivesAfterConfirmedPairing(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)

	h.session.PairingEvents = make(chan port.PairingEvent, 1)
	h.session.PairingEvents <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
	close(h.session.PairingEvents)

	// PairFunc runs inside Start(), synchronously, right after Subscribe
	// registers this orchestrator's session-event handler — so Emit here
	// deterministically precedes runPairing consuming the buffered Timeout
	// item. No sleep, no wall-clock: the order is forced, not raced.
	h.session.PairFunc = func(ctx context.Context) (<-chan port.PairingEvent, error) {
		h.session.Emit(port.SessionEvent{Kind: port.SessionEventKindConnected})
		return h.session.PairingEvents, nil
	}

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(h.registry.UnregisterCalls) != 0 {
		t.Errorf("Unregister after a CONFIRMED pairing (%v): the live session got torn down", h.registry.UnregisterCalls)
	}
	if len(h.attach.DetachCalls) != 0 {
		t.Errorf("Detach after a CONFIRMED pairing (%v): the live session got torn down", h.attach.DetachCalls)
	}
	if len(released) != 0 {
		t.Errorf("ownership released after a CONFIRMED pairing (%v): the lease was handed away from a live session", released)
	}
	if got := h.dispatcher.DispatchedTypes(); indexOf(got, "QRTimeout") >= 0 {
		t.Errorf("QRTimeout dispatched after a CONFIRMED pairing: %v", got)
	}
}

// TestStart_TimeoutWithoutConfirmationStillTearsDownEverything is the F98
// regression control in the opposite direction: a QR that the user simply
// never scans (no Connected/PairSuccess ever observed) must still tear down
// every holder the teardown enumerates today — Unregister, Detach,
// QRTimeout dispatch, ownership release, and the qrcode column clear. This
// is the scenario the guard must NOT weaken.
func TestStart_TimeoutWithoutConfirmationStillTearsDownEverything(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)

	events := make(chan port.PairingEvent, 1)
	events <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(h.registry.UnregisterCalls) != 1 {
		t.Fatalf("esperava exatamente 1 Unregister, obtive %v", h.registry.UnregisterCalls)
	}
	if len(h.attach.DetachCalls) != 1 {
		t.Fatalf("esperava exatamente 1 Detach, obtive %v", h.attach.DetachCalls)
	}
	if len(released) != 1 || released[0] != "user-1" {
		t.Fatalf("esperava ownership liberada para user-1, obtive %v", released)
	}
	if got := h.dispatcher.DispatchedTypes(); len(got) != 1 || got[0] != "QRTimeout" {
		t.Fatalf("esperava despacho de QRTimeout, obtive %v", got)
	}
	foundQRClear := false
	for _, q := range h.db.execQueries {
		if q == `UPDATE users SET qrcode='' WHERE id=$1` {
			foundQRClear = true
		}
	}
	if !foundQRClear {
		t.Fatalf("esperava UPDATE limpando qrcode, obtive %v", h.db.execQueries)
	}
}

// TestStart_LateSuccessAfterTimeoutDoesNotResurrectTornDownSession covers
// the reverse order: if timeout is read BEFORE any confirmation ever
// arrives, teardown already ran (as F98 requires) and a Success item
// arriving afterward on the same channel cannot undo it — it can only be a
// harmless no-op (flip `confirmed`, clear qrcode again).
func TestStart_LateSuccessAfterTimeoutDoesNotResurrectTornDownSession(t *testing.T) {
	var released []string
	h := newHarness(t,
		WithOwnershipCheck(
			func(string) bool { return true },
			func(userID string) { released = append(released, userID) },
		),
	)

	events := make(chan port.PairingEvent, 2)
	events <- port.PairingEvent{Kind: port.PairingEventKindTimeout}
	events <- port.PairingEvent{Kind: port.PairingEventKindSuccess}
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(h.registry.UnregisterCalls) != 1 {
		t.Fatalf("esperava exatamente 1 Unregister (do timeout), obtive %v", h.registry.UnregisterCalls)
	}
	if len(h.attach.DetachCalls) != 1 {
		t.Fatalf("esperava exatamente 1 Detach (do timeout), obtive %v", h.attach.DetachCalls)
	}
	if len(released) != 1 {
		t.Fatalf("esperava ownership liberada exatamente uma vez, obtive %v", released)
	}
}

// TestStart_LateQRAfterSuccessDoesNotResurrectQR covers a QR code event
// arriving after Success: it must not be dispatched nor rewrite the qrcode
// column with a stale code the user could scan into a session that is
// already alive.
func TestStart_LateQRAfterSuccessDoesNotResurrectQR(t *testing.T) {
	h := newHarness(t)

	events := make(chan port.PairingEvent, 2)
	events <- port.PairingEvent{Kind: port.PairingEventKindSuccess}
	events <- port.PairingEvent{Kind: port.PairingEventKindQR, Code: "late-code", Timeout: 20 * time.Second}
	close(events)
	h.session.PairingEvents = events

	if err := h.orch.Start(context.Background(), "user-1", "token-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := h.dispatcher.DispatchedTypes(); indexOf(got, "QR") >= 0 {
		t.Errorf("QR dispatched for a code that arrived AFTER success: %v", got)
	}
	for _, q := range h.db.execQueries {
		if strings.Contains(q, "qrcode=$1") {
			t.Errorf("qrcode column rewritten with a stale QR image after success: %v", h.db.execQueries)
		}
	}
}

// --- F192: um Start de cada vez por utilizador ---------------------------
//
// O defeito, medido em 2026-08-20 contra o servidor real com a sessão
// `qr-teste`: `GET /session/connect` numa sessão que JÁ estava a parear NÃO
// era um no-op — abria um SEGUNDO fluxo de pareamento e deixava o primeiro
// órfão. Os dois emissores de QR ficavam vivos, ambos a escrever
// `users.qrcode`, e os códigos chegavam intercalados ao painel:
//
//	1.080s HTTP connect(1)                     -> 200 connecting
//	1.838s WS MSG type=QR qrlen=1850
//	6.092s HTTP connect(2, durante pareamento) -> 200 connecting
//	6.736s WS MSG type=QR qrlen=1846   <- fluxo 2
//	19.681s WS MSG type=QR qrlen=1874  <- fluxo 1
//	21.904s WS MSG type=QR qrlen=1850  <- fluxo 2
//	26.772s WS MSG type=QR qrlen=1838  <- fluxo 1
//
// O que morde aqui é a CAUSA (um segundo Pair para o mesmo utilizador), não
// o sintoma (o painel piscar): silenciar o sintoma no painel deixaria os dois
// clientes de pé no servidor.

// startEmCurso põe um Start a correr e preso dentro do pareamento, e devolve
// uma função que o liberta. É o estado "a sessão está a parear agora" — o
// mesmo em que a medição acima chamou o segundo connect.
func startEmCurso(t *testing.T, h *harness, userID string) (liberta func()) {
	t.Helper()
	eventos := make(chan port.PairingEvent)
	h.session.PairingEvents = eventos

	emPareamento := make(chan struct{})
	var mu sync.Mutex
	primeira := true
	h.session.PairFunc = func(context.Context) (<-chan port.PairingEvent, error) {
		mu.Lock()
		defer mu.Unlock()
		if primeira {
			primeira = false
			close(emPareamento)
			return eventos, nil
		}
		// Qualquer Start SEGUINTE recebe um canal JÁ FECHADO, para retornar
		// de imediato em vez de bloquear no consumo de eventos.
		//
		// Sem isto, o controlo negativo desta trava falha por TRAVAMENTO em
		// vez de por asserção — e um controlo que trava não diz o que
		// quebrou. A guarda que estes testes protegem tem de ser provada por
		// uma contagem, não pela ausência de progresso.
		vazio := make(chan port.PairingEvent)
		close(vazio)
		return vazio, nil
	}

	terminou := make(chan struct{})
	go func() {
		defer close(terminou)
		_ = h.orch.Start(context.Background(), userID, "tok")
	}()

	select {
	case <-emPareamento:
	case <-time.After(2 * time.Second):
		t.Fatal("o primeiro Start não chegou a Pair")
	}
	return func() {
		close(eventos)
		select {
		case <-terminou:
		case <-time.After(2 * time.Second):
			t.Fatal("o primeiro Start não retornou depois de o canal fechar")
		}
	}
}

func TestStartRecusaSegundoFluxoEnquantoOPrimeiroPareia(t *testing.T) {
	h := newHarness(t)
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	pairesAntes := len(h.session.PairCalls)

	err := h.orch.Start(context.Background(), "u1", "tok")
	if err == nil {
		t.Fatal("o segundo Start devolveu nil: um segundo fluxo de pareamento foi iniciado para o mesmo utilizador")
	}
	if got := appErrCodeDe(err); got != codeSessionStartAlreadyInFlight {
		t.Fatalf("código = %q, quero %q (erro: %v)", got, codeSessionStartAlreadyInFlight, err)
	}
	if got := len(h.session.PairCalls); got != pairesAntes {
		t.Fatalf("Pair foi chamado %d vezes a mais: o segundo fluxo arrancou mesmo assim", got-pairesAntes)
	}
}

// TestStartRecusaAntesDeReivindicarPosse trava a ORDEM. A guarda corre ANTES
// da reivindicação de posse; instalada depois, dois Starts concorrentes
// tocariam o lease do mesmo utilizador antes de um desistir — e um teste que
// só olhasse para o erro devolvido continuaria verde.
func TestStartRecusaAntesDeReivindicarPosse(t *testing.T) {
	var reivindicacoes int
	h := newHarness(t, WithOwnershipCheck(
		func(string) bool { reivindicacoes++; return true },
		func(string) {},
	))
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	antes := reivindicacoes
	if err := h.orch.Start(context.Background(), "u1", "tok"); err == nil {
		t.Fatal("esperava recusa do segundo Start")
	}
	if reivindicacoes != antes {
		t.Fatalf("o Start recusado reivindicou posse %d vez(es): a guarda está DEPOIS da reivindicação",
			reivindicacoes-antes)
	}
}

// TestStartDeOutroUtilizadorNaoEBloqueado: a chave é por utilizador. Se fosse
// global, um pareamento em curso pararia o painel inteiro.
func TestStartDeOutroUtilizadorNaoEBloqueado(t *testing.T) {
	h := newHarness(t)
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	h.session.PairFunc = nil
	vazio := make(chan port.PairingEvent)
	close(vazio)
	h.session.PairingEvents = vazio

	if err := h.orch.Start(context.Background(), "u2", "tok"); err != nil {
		t.Fatalf("Start de outro utilizador foi bloqueado: %v", err)
	}
}

// TestStartLibertaAChaveAoTerminar é o teste da Regra 4 do CLAUDE.md: o
// conserto também é um mecanismo. Se a chave não fosse devolvida, a guarda
// trocaria "dois fluxos concorrentes" por "utilizador que nunca mais
// conecta" — estritamente pior que o defeito original.
func TestStartLibertaAChaveAoTerminar(t *testing.T) {
	h := newHarness(t)
	liberta := startEmCurso(t, h, "u1")
	liberta()

	h.session.PairFunc = nil
	vazio := make(chan port.PairingEvent)
	close(vazio)
	h.session.PairingEvents = vazio

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start depois de o anterior terminar foi recusado: %v", err)
	}
}

// TestStartCedeChaveEstagnada: mesmo que um Start nunca retorne, a chave
// caduca em startInFlightTTL. É o teto que impede o estado absorvente.
func TestStartCedeChaveEstagnada(t *testing.T) {
	agora := time.Unix(0, 0)
	h := newHarness(t, WithClock(func() time.Time { return agora }))
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	if err := h.orch.Start(context.Background(), "u1", "tok"); err == nil {
		t.Fatal("esperava recusa dentro do TTL")
	}

	agora = agora.Add(startInFlightTTL + time.Second)
	h.session.PairFunc = nil
	vazio := make(chan port.PairingEvent)
	close(vazio)
	h.session.PairingEvents = vazio

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("chave estagnada não foi cedida depois de %v: %v", startInFlightTTL, err)
	}
}

// --- F274: pré-check síncrono e libertação pelo Disconnect -----------------
//
// GET /session/connect depois de POST /session/disconnect devolvia 200 e não
// religava: a guarda de startInFlight recusava o Start de dentro da
// goroutine, mas o handler já tinha respondido sucesso. CheckStartAvailable é
// o pré-check síncrono (mesmo desenho de CheckOwnership/F108); ReleaseStart é
// o que o Disconnect chama para não deixar a chave presa até o TTL.

// TestCheckStartAvailable_RecusaEnquantoHaFluxoEmCurso é o teste do defeito:
// com um Start preso a parear, o pré-check TEM de ver a mesma recusa que o
// próprio Start veria — é o que falta para o handler responder 409 em vez de
// 200 antes de sequer disparar a goroutine.
func TestCheckStartAvailable_RecusaEnquantoHaFluxoEmCurso(t *testing.T) {
	h := newHarness(t)
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	err := h.orch.CheckStartAvailable("u1")
	if err == nil {
		t.Fatal("CheckStartAvailable devolveu nil com um Start em curso para o mesmo utilizador")
	}
	if got := appErrCodeDe(err); got != codeSessionStartAlreadyInFlight {
		t.Fatalf("código = %q, quero %q (erro: %v)", got, codeSessionStartAlreadyInFlight, err)
	}
}

// TestCheckStartAvailable_NaoReivindicaAChave prova que o pré-check é um
// PEEK e não um acquire: chamá-lo não pode fazer o Start seguinte encontrar a
// chave ocupada por si mesmo. Sem isto, o pré-check "que passa" bloquearia
// sempre o Start real que vem a seguir, na mesma requisição.
func TestCheckStartAvailable_NaoReivindicaAChave(t *testing.T) {
	h := newHarness(t)

	if err := h.orch.CheckStartAvailable("u1"); err != nil {
		t.Fatalf("CheckStartAvailable = %v, queria nil (nenhum Start em curso)", err)
	}

	h.session.PairFunc = nil
	vazio := make(chan port.PairingEvent)
	close(vazio)
	h.session.PairingEvents = vazio

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start depois de CheckStartAvailable foi recusado: %v — o pré-check reivindicou a chave", err)
	}
}

// TestCheckStartAvailable_LivreQuandoNaoHaFluxo é o controlo positivo: sem
// nenhum Start em curso, nil.
func TestCheckStartAvailable_LivreQuandoNaoHaFluxo(t *testing.T) {
	h := newHarness(t)
	if err := h.orch.CheckStartAvailable("u1"); err != nil {
		t.Fatalf("CheckStartAvailable = %v, queria nil", err)
	}
}

// TestReleaseStart_LibertaAChaveAntesDoTTL é o teste do defeito da outra
// metade da F274: sem ReleaseStart, um Start preso em pareamento (ou cuja
// meta nunca foi limpa por algum motivo) só cede a chave depois de
// startInFlightTTL. Disconnect precisa poder libertar de imediato.
func TestReleaseStart_LibertaAChaveAntesDoTTL(t *testing.T) {
	h := newHarness(t)
	liberta := startEmCurso(t, h, "u1")
	defer liberta()

	if err := h.orch.CheckStartAvailable("u1"); err == nil {
		t.Fatal("esperava chave ocupada antes de ReleaseStart")
	}

	h.orch.ReleaseStart("u1")

	if err := h.orch.CheckStartAvailable("u1"); err != nil {
		t.Fatalf("CheckStartAvailable depois de ReleaseStart = %v, queria nil", err)
	}
}

// TestReleaseStart_DeUtilizadorSemChaveENoop: liberar uma chave que não
// existe não deve entrar em pânico nem afetar outros utilizadores.
func TestReleaseStart_DeUtilizadorSemChaveENoop(t *testing.T) {
	h := newHarness(t)
	h.orch.ReleaseStart("ninguem-comecou")
	if err := h.orch.CheckStartAvailable("ninguem-comecou"); err != nil {
		t.Fatalf("CheckStartAvailable = %v, queria nil", err)
	}
}

// --- F78: Start numa sessão já conectada é no-op --------------------------

func TestStart_AlreadyConnected_IsNoop(t *testing.T) {
	h := newHarness(t)

	existingSess := &contractsfake.Session{
		Recorder:        h.recorder,
		IsConnectedFunc: func() bool { return true },
	}
	h.registry.Sessions = map[string]port.Session{"u1": existingSess}

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start on connected session should be no-op, got: %v", err)
	}

	if len(h.provider.NewSessionCalls) != 0 {
		t.Fatal("NewSession must not be called when session is already connected")
	}
	if len(h.registry.RegisterCalls) != 0 {
		t.Fatal("Register must not be called when session is already connected")
	}
}

func TestStart_NotConnected_ProceedsNormally(t *testing.T) {
	h := newHarness(t)

	existingSess := &contractsfake.Session{
		Recorder:        h.recorder,
		IsConnectedFunc: func() bool { return false },
	}
	h.registry.Sessions = map[string]port.Session{"u1": existingSess}

	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start on disconnected session should proceed: %v", err)
	}

	if len(h.provider.NewSessionCalls) == 0 {
		t.Fatal("NewSession must be called when existing session is not connected")
	}
}

func TestStart_NoExistingSession_ProceedsNormally(t *testing.T) {
	h := newHarness(t)
	h.session.HasCredentialsFunc = func() bool { return true }

	if err := h.orch.Start(context.Background(), "u1", "tok"); err != nil {
		t.Fatalf("Start with no existing session should proceed: %v", err)
	}

	if len(h.provider.NewSessionCalls) == 0 {
		t.Fatal("NewSession must be called when no session exists in registry")
	}
}

func appErrCodeDe(err error) string {
	var e *apperr.AppError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
