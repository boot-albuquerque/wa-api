package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	appsession "wa-api/pkg/application/session"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/presentation/http/handlers"
)

// blockingProvider is the minimal appport.SessionProvider double
// busyOrchestrator needs: NewSession blocks until the test closes block,
// which is enough to keep the orchestrator's in-flight guard busy without
// standing up a real registry/dispatcher/attach-hook chain — Start returns
// (with an error) before touching any of those once NewSession fails.
type blockingProvider struct{ block <-chan struct{} }

func (p blockingProvider) NewSession(context.Context, appport.SessionSpec) (appport.Session, error) {
	<-p.block
	return nil, errors.New("blocking provider: no real session available")
}

// noopRegistry is the minimal appport.SessionRegistry double: Start calls
// Get(userID) BEFORE NewSession, so it must be non-nil and answer "no
// session registered" even though this test never reaches Register.
type noopRegistry struct{}

func (noopRegistry) Register(string, appport.Session)            {}
func (noopRegistry) Get(string) (appport.Session, bool)          { return nil, false }
func (noopRegistry) Unregister(string)                           {}
func (noopRegistry) ProvisionWebhookClient(string, string) error { return nil }

// F274 — GET /session/connect after POST /session/disconnect returned 200
// and did not reconnect: the disconnect tore down the transport but left
// the orchestrator's per-user "start in flight" mark busy for up to
// startInFlightTTL (3 minutes), and the ConnectHandler responded 200 without
// ever checking whether Start would actually run.
//
// Two wiring locks, mirroring F108's `.WithCheckOwnership` lock
// (connect_ownership_wiring_test.go): `.WithCheckStartInFlight` on the
// ConnectHandler, and `disconnectInFlightReleaser` wrapping the
// SessionDisconnector fed to DisconnectUseCase.

const startInFlightWiringUser = "FIX274"

// TestConnectStartInFlightCheckIsWired: the production router must call
// CheckStartInFlight on the ConnectHandler. Tested via the REGISTERED
// ROUTE, not the handler directly (ARMADILHAS.md #2).
func TestConnectStartInFlightCheckIsWired(t *testing.T) {
	prev := customHandlerSet
	t.Cleanup(func() { customHandlerSet = prev })

	initCustomHandlers(&server{DB: newChatHistoryDB(t), ExPath: t.TempDir()})

	if customHandlerSet.Session.Connect.CheckStartInFlight == nil {
		t.Fatal("CheckStartInFlight is nil after initCustomHandlers — " +
			"the wiring in initConnectHandler (wiring_handlers.go) no longer calls " +
			".WithCheckStartInFlight(...). Without it, GET /session/connect responds " +
			"200 {\"status\":\"connecting\"} while a pairing flow is already in " +
			"flight for this user: the response lies (F274).")
	}

	// A checagem de wiring acima (linha 63) já provou que a produção liga
	// CheckStartInFlight. A partir daqui o teste troca o Connect por um
	// construído sobre um registry de teste — o mesmo padrão de
	// connect_ownership_wiring_test.go's starterRegistry — porque
	// `.StartSession` deixou de existir no ConnectHandler (F273/F281): quem
	// arranca a sessão agora é o Starter que o registry resolve, e
	// CheckStartInFlight roda ANTES dele ser sequer tocado — a ordem já
	// garante que um StartSession real nunca é alcançado quando o
	// in-flight check rejeita, sem precisar de um duble para provar isso
	// aqui de novo (isso já está travado em
	// TestConnectHandler_StartInFlight_409, pkg/presentation/http/handlers).
	starter := &contractsfake.SessionStarter{
		StartSessionFunc: func(context.Context, string, string) {
			t.Fatal("StartSession must not be called when a start is already in flight")
		},
	}
	customHandlerSet.Session.Connect = handlers.NewConnectHandler(
		session.NewConnectUseCase(&contractsfake.Logger{}),
		starterRegistry(starter),
	).WithCheckStartInFlight(func(string) error {
		return apperr.New(
			"session_start_already_in_flight",
			apperr.CategoryConflict,
			"a session start is already in flight for this user; read the current QR from GET /session/qr",
			false,
			nil,
		)
	})

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), appport.UserInfoKey, *userValues(startInFlightWiringUser, 0))))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), customHandlerSet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/connect?engine=wa_noise", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 — a start already in flight must reach the HTTP "+
			"boundary through the registered route, not be swallowed behind a 200 "+
			"(body: %s)", rec.Code, rec.Body.String())
	}
}

// TestConnectStartInFlightCheck_ToleratesNilOrchestrator: some tests (and
// this file's own wiring-lock test above) build a bare *server without
// setting SessionOrchestrator. The pre-check must treat that as "no guard
// installed yet", not crash — mirrors connectOwnershipCheck's tolerance of a
// nil s.Leases.
func TestConnectStartInFlightCheck_ToleratesNilOrchestrator(t *testing.T) {
	check := connectStartInFlightCheck(&server{})
	if err := check("u1"); err != nil {
		t.Fatalf("check = %v, want nil with no SessionOrchestrator wired", err)
	}
}

// fakeDisconnector is the minimal appport.SessionDisconnector double this
// file needs: EnsureSession is unused by DisconnectUseCase's happy path, so
// only Disconnect (and the embedded SessionStatusReader, satisfied via the
// zero value) matter here.
type fakeDisconnector struct {
	appport.SessionDisconnector
	disconnectErr error
	calls         []string
}

func (f *fakeDisconnector) EnsureSession(context.Context, string) error { return nil }

func (f *fakeDisconnector) Disconnect(_ context.Context, txtID string) error {
	f.calls = append(f.calls, txtID)
	return f.disconnectErr
}

// TestDisconnectInFlightReleaser_ChamaReleaseStartQuandoDisconnectTemSucesso
// prova a ligação fim-a-fim: Disconnect bem-sucedido chama
// Orchestrator.ReleaseStart com o MESMO userID, usando um Start real
// bloqueado em pareamento para produzir o estado "em voo" sem tocar em
// símbolos não exportados do pacote appsession.
func TestDisconnectInFlightReleaser_ChamaReleaseStartQuandoDisconnectTemSucesso(t *testing.T) {
	orch, unblock := busyOrchestrator(t, "u1")
	defer unblock()

	if err := orch.CheckStartAvailable("u1"); err == nil {
		t.Fatal("esperava chave ocupada antes do Disconnect")
	}

	fd := &fakeDisconnector{}
	d := disconnectInFlightReleaser{SessionDisconnector: fd, orch: orch}

	if err := d.Disconnect(context.Background(), "u1"); err != nil {
		t.Fatalf("Disconnect = %v, queria nil", err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "u1" {
		t.Fatalf("delegate.Disconnect chamado com %v, queria [u1]", fd.calls)
	}

	if err := orch.CheckStartAvailable("u1"); err != nil {
		t.Fatalf("CheckStartAvailable depois do Disconnect = %v, queria nil — a chave não foi liberada", err)
	}
}

// TestDisconnectInFlightReleaser_NaoLiberaQuandoDisconnectFalha: se o
// Disconnect falhar, o transporte (e o que ele estava a fazer) continuam de
// pé — não há nada obsoleto para limpar, e liberar mesmo assim esconderia a
// falha de um fluxo que continua vivo.
func TestDisconnectInFlightReleaser_NaoLiberaQuandoDisconnectFalha(t *testing.T) {
	orch, unblock := busyOrchestrator(t, "u1")
	defer unblock()

	boom := errors.New("boom")
	fd := &fakeDisconnector{disconnectErr: boom}
	d := disconnectInFlightReleaser{SessionDisconnector: fd, orch: orch}

	if err := d.Disconnect(context.Background(), "u1"); !errors.Is(err, boom) {
		t.Fatalf("Disconnect = %v, queria embrulhar %v", err, boom)
	}

	if err := orch.CheckStartAvailable("u1"); err == nil {
		t.Fatal("a chave foi liberada mesmo com o Disconnect a falhar — o fluxo continua vivo")
	}
}

// TestDisconnectInFlightReleaser_ToleraOrchestratorNil: alguns testes
// constroem DisconnectUseCase sem SessionOrchestrator (ver
// connectStartInFlightCheck para o mesmo padrão do lado do connect).
func TestDisconnectInFlightReleaser_ToleraOrchestratorNil(t *testing.T) {
	fd := &fakeDisconnector{}
	d := disconnectInFlightReleaser{SessionDisconnector: fd, orch: nil}

	if err := d.Disconnect(context.Background(), "u1"); err != nil {
		t.Fatalf("Disconnect = %v, queria nil", err)
	}
}

// busyOrchestrator devolve um *appsession.Orchestrator com um Start preso a
// "parear" para userID, e a função que o liberta. Não usa nenhum símbolo não
// exportado do pacote appsession — só a superfície pública (Start bloqueia
// até o provider devolver ou o canal de pareamento fechar).
func busyOrchestrator(t *testing.T, userID string) (orch *appsession.Orchestrator, unblock func()) {
	t.Helper()

	block := make(chan struct{})
	// dispatcher, attach hook and db (userStore) are all nil: NewSession
	// always fails after unblocking, so Start returns before ever touching
	// them — see Orchestrator.Start's order (registry.Get, then
	// provider.NewSession, then everything else).
	orch = appsession.NewOrchestrator(
		blockingProvider{block: block},
		noopRegistry{},
		nil,
		nil,
		nil,
	)

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		_ = orch.Start(context.Background(), userID, "tok")
	}()
	<-started

	// Dá tempo do Start reivindicar a chave antes do teste checar
	// CheckStartAvailable — o provider bloqueia exatamente nesse ponto
	// (NewSession), que é DEPOIS do acquire (ver Orchestrator.Start).
	for i := 0; i < 100 && orch.CheckStartAvailable(userID) == nil; i++ {
		<-time.After(time.Millisecond)
	}

	return orch, func() {
		close(block)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Start bloqueado não retornou depois de o provider ser liberado")
		}
	}
}
