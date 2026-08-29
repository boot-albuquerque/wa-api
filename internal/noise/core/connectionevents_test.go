package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types/events"
)

// collectEvents registra um handler que guarda tudo que for despachado. Devolve
// uma funcao que espera ate' `want` eventos chegarem (o despacho e' feito em
// goroutine na maioria dos ramos) e devolve o que juntou.
func collectEvents(cli *Client) func(t *testing.T, want int, timeout time.Duration) []any {
	var mu sync.Mutex
	var got []any
	arrived := make(chan struct{}, 64)
	cli.AddEventHandler(func(evt any) {
		mu.Lock()
		got = append(got, evt)
		mu.Unlock()
		arrived <- struct{}{}
	})
	return func(t *testing.T, want int, timeout time.Duration) []any {
		t.Helper()
		deadline := time.After(timeout)
		for i := 0; i < want; i++ {
			select {
			case <-arrived:
			case <-deadline:
				mu.Lock()
				defer mu.Unlock()
				t.Fatalf("so' chegaram %d de %d eventos: %+v", len(got), want, got)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]any(nil), got...)
	}
}

func failureNode(reason string) *waBinary.Node {
	return &waBinary.Node{Tag: "failure", Attrs: waBinary.Attrs{"reason": reason}}
}

// --- BUG DO LOTE 10: RefreshCAT nil derrubava o processo ---

// Regressao do achado principal deste lote. `reason` vem do servidor e
// RefreshCAT so' e' preenchido por consumidores Messenger — em todo o
// repositorio ele e' sempre nil. Antes da correcao, um <failure reason="413">
// ou "414" chamava uma func nil dentro do goroutine do handlerQueueLoop, que
// nao tem recover: panic e processo inteiro no chao, disparado remotamente.
//
// Depois da correcao o motivo cai no tratamento generico: disconnect esperado e
// um events.ConnectFailure para a aplicacao decidir.
func TestHandleConnectFailureCATWithNilRefreshCATDoesNotPanic(t *testing.T) {
	for _, reason := range []string{"413", "414"} {
		t.Run(reason, func(t *testing.T) {
			cli := connTestClient()
			if cli.RefreshCAT != nil {
				t.Fatal("o teste exige RefreshCAT nil")
			}
			wait := collectEvents(cli)

			cli.handleConnectFailure(t.Context(), failureNode(reason))

			evts := wait(t, 1, 2*time.Second)
			cf, ok := evts[0].(*events.ConnectFailure)
			if !ok {
				t.Fatalf("evento = %T, queria *events.ConnectFailure", evts[0])
			}
			if int(cf.Reason) != 413 && int(cf.Reason) != 414 {
				t.Errorf("Reason = %d, queria 413 ou 414", int(cf.Reason))
			}
			if !cli.isExpectedDisconnect() {
				t.Error("sem RefreshCAT o motivo CAT tem que cair no default, que espera disconnect")
			}
		})
	}
}

// O outro lado da mesma correcao: com RefreshCAT preenchido o comportamento
// original continua valendo — a funcao e' chamada, e o disconnect nao e'
// esperado (a reconexao deve seguir depois do refresh).
func TestHandleConnectFailureCATCallsRefreshCAT(t *testing.T) {
	cli := connTestClient()
	var calls int
	cli.RefreshCAT = func(context.Context) error {
		calls++
		return nil
	}

	cli.handleConnectFailure(t.Context(), failureNode("414"))

	if calls != 1 {
		t.Errorf("RefreshCAT chamada %d vezes, queria 1", calls)
	}
	if cli.isExpectedDisconnect() {
		t.Error("refresh bem-sucedido nao pode marcar disconnect esperado")
	}
}

// Falha no refresh: ai' sim vira disconnect esperado e um CATRefreshError, para
// a aplicacao saber que a credencial nao pode ser renovada.
func TestHandleConnectFailureCATRefreshError(t *testing.T) {
	cli := connTestClient()
	boom := errors.New("token endpoint fora do ar")
	cli.RefreshCAT = func(context.Context) error { return boom }
	wait := collectEvents(cli)

	cli.handleConnectFailure(t.Context(), failureNode("413"))

	evts := wait(t, 1, 2*time.Second)
	refreshErr, ok := evts[0].(*events.CATRefreshError)
	if !ok {
		t.Fatalf("evento = %T, queria *events.CATRefreshError", evts[0])
	}
	if !errors.Is(refreshErr.Error, boom) {
		t.Errorf("Error = %v, queria %v", refreshErr.Error, boom)
	}
	if !cli.isExpectedDisconnect() {
		t.Error("falha de refresh tem que marcar disconnect esperado")
	}
}

// --- demais ramos de handleConnectFailure ---

// 503 e 500 sao os unicos motivos que deixam o auto-reconnect agir sozinho: nao
// podem marcar disconnect esperado, senao o cliente nunca voltaria.
func TestHandleConnectFailureTransientKeepsAutoReconnect(t *testing.T) {
	for _, reason := range []string{"500", "503"} {
		t.Run(reason, func(t *testing.T) {
			cli := connTestClient()
			cli.handleConnectFailure(t.Context(), failureNode(reason))
			if cli.isExpectedDisconnect() {
				t.Errorf("motivo %s nao pode marcar disconnect esperado", reason)
			}
		})
	}
}

func TestHandleConnectFailureTempBanned(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := &waBinary.Node{Tag: "failure", Attrs: waBinary.Attrs{
		"reason": "402",
		"code":   "101",
		"expire": "3600",
	}}

	cli.handleConnectFailure(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	ban, ok := evts[0].(*events.TemporaryBan)
	if !ok {
		t.Fatalf("evento = %T, queria *events.TemporaryBan", evts[0])
	}
	if ban.Code != events.TempBanSentToTooManyPeople {
		t.Errorf("Code = %v, queria TempBanSentToTooManyPeople", ban.Code)
	}
	// `expire` vem em segundos e precisa virar Duration; tratar como
	// nanossegundos daria uma hora virando 3.6 microssegundos.
	if ban.Expire != time.Hour {
		t.Errorf("Expire = %v, queria 1h", ban.Expire)
	}
	if !cli.isExpectedDisconnect() {
		t.Error("banimento temporario tem que marcar disconnect esperado")
	}
}

func TestHandleConnectFailureClientOutdated(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)

	cli.handleConnectFailure(t.Context(), failureNode("405"))

	evts := wait(t, 1, 2*time.Second)
	if _, ok := evts[0].(*events.ClientOutdated); !ok {
		t.Fatalf("evento = %T, queria *events.ClientOutdated", evts[0])
	}
}

// Motivo desconhecido nao pode ser engolido: vira ConnectFailure cru com o no'
// original, para a aplicacao poder diagnosticar.
func TestHandleConnectFailureUnknownReason(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := failureNode("9999")

	cli.handleConnectFailure(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	cf, ok := evts[0].(*events.ConnectFailure)
	if !ok {
		t.Fatalf("evento = %T, queria *events.ConnectFailure", evts[0])
	}
	if int(cf.Reason) != 9999 {
		t.Errorf("Reason = %d, queria 9999", int(cf.Reason))
	}
	if cf.Raw != node {
		t.Error("o no' bruto tem que ser repassado no evento")
	}
}

// --- handleStreamError ---

// <conflict type="replaced"> significa que outra sessao assumiu: o cliente tem
// que parar de tentar reconectar, senao as duas sessoes brigam em loop.
func TestHandleStreamErrorReplaced(t *testing.T) {
	cli := connTestClient()
	cli.isLoggedIn.Store(true)
	wait := collectEvents(cli)
	node := &waBinary.Node{
		Tag: "stream:error",
		Content: []waBinary.Node{{
			Tag:   streamErrorConflictTag,
			Attrs: waBinary.Attrs{"type": conflictTypeReplaced},
		}},
	}

	cli.handleStreamError(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	if _, ok := evts[0].(*events.StreamReplaced); !ok {
		t.Fatalf("evento = %T, queria *events.StreamReplaced", evts[0])
	}
	if !cli.isExpectedDisconnect() {
		t.Error("'replaced' tem que marcar disconnect esperado")
	}
	if cli.IsLoggedIn() {
		t.Error("qualquer stream error tem que derrubar o estado de logado")
	}
}

// 503 e' o servidor avisando que vai reiniciar: nenhum evento, nenhum
// disconnect esperado — o auto-reconnect resolve.
func TestHandleStreamErrorServiceUnavailable(t *testing.T) {
	cli := connTestClient()
	node := &waBinary.Node{
		Tag:   "stream:error",
		Attrs: waBinary.Attrs{"code": streamErrorServiceUnavailable},
	}

	cli.handleStreamError(t.Context(), node)

	if cli.isExpectedDisconnect() {
		t.Error("503 nao pode marcar disconnect esperado")
	}
}

// Espelho do bug principal, no irmao que ja' tinha a guarda: com RefreshCAT nil
// um code de CAT tem que cair no ramo default e virar StreamError, sem panic.
func TestHandleStreamErrorCATWithNilRefreshCAT(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := &waBinary.Node{
		Tag:   "stream:error",
		Attrs: waBinary.Attrs{"code": events.ConnectFailureCATExpired.NumberString()},
	}

	cli.handleStreamError(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	if _, ok := evts[0].(*events.StreamError); !ok {
		t.Fatalf("evento = %T, queria *events.StreamError", evts[0])
	}
}

// Code desconhecido vira StreamError com o no' bruto.
func TestHandleStreamErrorUnknownCode(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := &waBinary.Node{Tag: "stream:error", Attrs: waBinary.Attrs{"code": "777"}}

	cli.handleStreamError(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	se, ok := evts[0].(*events.StreamError)
	if !ok {
		t.Fatalf("evento = %T, queria *events.StreamError", evts[0])
	}
	if se.Code != "777" {
		t.Errorf("Code = %q, queria \"777\"", se.Code)
	}
}

// 515 com login autoreconnect desligado: em vez de reconectar sozinho, avisa a
// aplicacao. O ramo que reconecta de verdade precisa de socket e esta'
// documentado como lacuna.
func TestHandleStreamErrorRestartRequiredWithAutoReconnectDisabled(t *testing.T) {
	cli := connTestClient()
	cli.DisableLoginAutoReconnect = true
	wait := collectEvents(cli)
	node := &waBinary.Node{
		Tag:   "stream:error",
		Attrs: waBinary.Attrs{"code": streamErrorRestartRequired},
	}

	cli.handleStreamError(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	if _, ok := evts[0].(*events.ManualLoginReconnect); !ok {
		t.Fatalf("evento = %T, queria *events.ManualLoginReconnect", evts[0])
	}
}

// --- handleIB ---

func TestHandleIBOfflinePreview(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := &waBinary.Node{
		Tag: "ib",
		Content: []waBinary.Node{{
			Tag: "offline_preview",
			Attrs: waBinary.Attrs{
				"count":        "10",
				"appdata":      "1",
				"message":      "5",
				"notification": "3",
				"receipt":      "1",
			},
		}},
	}

	cli.handleIB(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	preview, ok := evts[0].(*events.OfflineSyncPreview)
	if !ok {
		t.Fatalf("evento = %T, queria *events.OfflineSyncPreview", evts[0])
	}
	if preview.Total != 10 || preview.Messages != 5 || preview.Notifications != 3 {
		t.Errorf("preview = %+v, contagens erradas", preview)
	}
}

func TestHandleIBOfflineCompleted(t *testing.T) {
	cli := connTestClient()
	wait := collectEvents(cli)
	node := &waBinary.Node{
		Tag:     "ib",
		Content: []waBinary.Node{{Tag: "offline", Attrs: waBinary.Attrs{"count": "42"}}},
	}

	cli.handleIB(t.Context(), node)

	evts := wait(t, 1, 2*time.Second)
	done, ok := evts[0].(*events.OfflineSyncCompleted)
	if !ok {
		t.Fatalf("evento = %T, queria *events.OfflineSyncCompleted", evts[0])
	}
	if done.Count != 42 {
		t.Errorf("Count = %d, queria 42", done.Count)
	}
}

// Um <ib> sem filhos, ou so' com filhos desconhecidos, nao pode gerar evento
// nem entrar em panic — o servidor manda <ib> para varias coisas que este
// cliente nao trata.
func TestHandleIBIgnoresUnknownAndEmpty(t *testing.T) {
	cli := connTestClient()
	cli.AddEventHandler(func(evt any) {
		t.Errorf("nao deveria despachar nada, veio %T", evt)
	})

	cli.handleIB(t.Context(), &waBinary.Node{Tag: "ib"})
	cli.handleIB(t.Context(), &waBinary.Node{
		Tag:     "ib",
		Content: []waBinary.Node{{Tag: "coisa_desconhecida"}},
	})
	// "dirty" tem o corpo comentado no upstream; tem que continuar inerte.
	cli.handleIB(t.Context(), &waBinary.Node{
		Tag:     "ib",
		Content: []waBinary.Node{{Tag: "dirty", Attrs: waBinary.Attrs{"type": "account_sync"}}},
	})
}
