package wanoise

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/internal/wa-noise/capabilities/appstatesync"
	waLog "wa-api/internal/wa-noise/observability/log"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
)

// TestErrAppStateUpdateEOMesmoValor trava o aliasing do sentinela: se a raiz
// declarasse um errors.New proprio, errors.Is falharia para quem compara com o
// nome da raiz contra um erro produzido dentro do subpacote.
func TestErrAppStateUpdateEOMesmoValor(t *testing.T) {
	t.Parallel()
	if !errors.Is(ErrAppStateUpdate, appstatesync.ErrUpdate) {
		t.Error("ErrAppStateUpdate deveria ser o mesmo valor de appstatesync.ErrUpdate")
	}
	if !errors.Is(appstatesync.ErrUpdate, ErrAppStateUpdate) {
		t.Error("a identidade precisa valer nos dois sentidos")
	}
}

// TestAppStateTransportEspelhaOCliente confere que o adaptador entrega os
// mesmos objetos do cliente, e nao copias.
func TestAppStateTransportEspelhaOCliente(t *testing.T) {
	t.Parallel()
	device := &store.Device{}
	cli := &Client{
		Log:                          waLog.Noop,
		Store:                        device,
		EmitAppStateEventsOnFullSync: true,
		AppStateDebugLogs:            true,
		appStateProc:                 appstate.NewProcessor(device, waLog.Noop),
	}
	tp := cli.appStateT()

	if tp.Store() != device {
		t.Error("Store() deveria devolver o mesmo *store.Device do cliente")
	}
	if tp.Proc() != cli.appStateProc {
		t.Error("Proc() deveria devolver o mesmo processador do cliente")
	}
	// O ponteiro do estado precisa ser estavel: os mutexes de dentro nunca
	// podem ser copiados por valor.
	if tp.State() != &cli.appStateSync || tp.State() != cli.appStateT().State() {
		t.Error("State() deveria apontar sempre para o campo do cliente")
	}
	if !tp.EmitEventsOnFullSync() || !tp.DebugLogs() {
		t.Error("as flags do cliente nao chegaram no adaptador")
	}
	if tp.Log() != cli.Log {
		t.Error("Log() deveria devolver o logger do cliente")
	}
	var missing *ElementMissingError
	if !errors.As(tp.ElementMissing("x", "y"), &missing) {
		t.Error("ElementMissing deveria devolver o *ElementMissingError da raiz")
	}
}

// TestFachadaDeAppStateRecusaClientNil trava as guardas de receiver nil dos
// metodos-fachada. Antes da Fase F/G lote 3 varios deles estouravam nil deref;
// agora todos devolvem ErrClientIsNil (ou o zero equivalente), na mesma linha
// do que os lotes 1 e 2 fizeram em midia e newsletter.
func TestFachadaDeAppStateRecusaClientNil(t *testing.T) {
	t.Parallel()
	var cli *Client
	ctx := context.Background()

	if _, err := cli.fetchAppState(ctx, appstate.WAPatchRegular, false, false); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("fetchAppState: %v", err)
	}
	if err := cli.FetchAppState(ctx, appstate.WAPatchRegular, false, false); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("FetchAppState: %v", err)
	}
	if _, err := cli.fetchAppStatePatches(ctx, appstate.WAPatchRegular, 0, false); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("fetchAppStatePatches: %v", err)
	}
	if _, err := cli.applyAppStatePatches(ctx, appstate.WAPatchRegular, appstate.HashState{}, nil, false, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("applyAppStatePatches: %v", err)
	}
	if err := cli.collectEventsToDispatch(ctx, appstate.WAPatchRegular, nil, false, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("collectEventsToDispatch: %v", err)
	}
	if err := cli.SendAppState(ctx, appstate.PatchInfo{}); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("SendAppState: %v", err)
	}
	if err := cli.MarkNotDirty(ctx, "account_sync", time.Now()); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("MarkNotDirty: %v", err)
	}
	if got := cli.dispatchAppState(ctx, appstate.WAPatchRegular, appstate.Mutation{}, false); got != nil {
		t.Errorf("dispatchAppState: %T", got)
	}
	// handleAppStateRecovery devolve "nada a reprocessar" em vez de erro: o
	// chamador usa o retorno so' para decidir se da' ack.
	if !cli.handleAppStateRecovery(ctx, "REQ", nil) {
		t.Error("handleAppStateRecovery com cliente nil deveria devolver true")
	}
	// As duas que nao devolvem nada: o contrato e' nao entrar em panico.
	cli.requestMissingAppStateKeys(ctx, nil)
	cli.requestAppStateKeys(ctx, [][]byte{{1}})
}

// TestFilterContactsFachada confere que o wrapper da raiz continua devolvendo o
// mesmo que a funcao livre — internals.go depende dessa assinatura.
func TestFilterContactsFachada(t *testing.T) {
	t.Parallel()
	var cli *Client
	filtered, contacts := cli.filterContacts(nil)
	if len(filtered) != 0 || contacts == nil {
		t.Errorf("filtered = %v, contacts = %v", filtered, contacts)
	}
}
