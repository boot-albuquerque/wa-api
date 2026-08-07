// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// --- registro e remocao de handlers ---

// Os handlers sao chamados na ordem de registro. A ordem importa porque um
// handler que devolve false interrompe a cadeia.
func TestDispatchEventCallsHandlersInOrder(t *testing.T) {
	cli := connTestClient()
	var order []int
	for i := 1; i <= 3; i++ {
		cli.AddEventHandler(func(evt any) { order = append(order, i) })
	}

	if failed := cli.dispatchEvent("evt"); failed {
		t.Error("nenhum handler falhou, dispatchEvent nao deveria reportar falha")
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("ordem = %v, queria [1 2 3]", order)
	}
}

// Um handler que devolve false corta a cadeia ali: os seguintes nao rodam e
// dispatchEvent avisa a falha. E' o que sustenta o ack sincrono
// (Client.SynchronousAck) — o ack so' sai se todo mundo aceitou.
func TestDispatchEventStopsOnFailure(t *testing.T) {
	cli := connTestClient()
	var called []string
	cli.AddEventHandlerWithSuccessStatus(func(evt any) bool {
		called = append(called, "primeiro")
		return true
	})
	cli.AddEventHandlerWithSuccessStatus(func(evt any) bool {
		called = append(called, "segundo")
		return false
	})
	cli.AddEventHandlerWithSuccessStatus(func(evt any) bool {
		called = append(called, "terceiro")
		return true
	})

	if failed := cli.dispatchEvent("evt"); !failed {
		t.Error("dispatchEvent deveria reportar falha")
	}
	if len(called) != 2 {
		t.Errorf("chamados = %v, queria parar no segundo", called)
	}
}

// AddEventHandler (o wrapper sem status) sempre reporta sucesso: um handler
// legado nao pode passar a segurar o ack sem querer.
func TestAddEventHandlerAlwaysSucceeds(t *testing.T) {
	cli := connTestClient()
	cli.AddEventHandler(func(evt any) {})
	if failed := cli.dispatchEvent("evt"); failed {
		t.Error("handler sem status nao pode reportar falha")
	}
}

// Os IDs sao unicos e crescentes — sao a unica forma de remover um handler
// depois.
func TestEventHandlerIDsAreUnique(t *testing.T) {
	cli := connTestClient()
	seen := make(map[uint32]struct{})
	for i := 0; i < 10; i++ {
		id := cli.AddEventHandler(func(evt any) {})
		if _, dup := seen[id]; dup {
			t.Fatalf("id %d repetido", id)
		}
		seen[id] = struct{}{}
	}
}

// Remocao nas tres posicoes (primeira, meio, ultima) — a implementacao trata
// cada uma com um ramo diferente, e o erro classico e' deslocar errado e perder
// ou duplicar um handler vizinho.
func TestRemoveEventHandlerAtEveryPosition(t *testing.T) {
	for _, remove := range []int{0, 1, 2} {
		t.Run([]string{"primeiro", "meio", "ultimo"}[remove], func(t *testing.T) {
			cli := connTestClient()
			var called []int
			ids := make([]uint32, 3)
			for i := 0; i < 3; i++ {
				ids[i] = cli.AddEventHandler(func(evt any) { called = append(called, i) })
			}

			if !cli.RemoveEventHandler(ids[remove]) {
				t.Fatal("RemoveEventHandler devolveu false para um id existente")
			}
			cli.dispatchEvent("evt")

			if len(called) != 2 {
				t.Fatalf("chamados = %v, queria 2 handlers", called)
			}
			for _, got := range called {
				if got == remove {
					t.Errorf("o handler %d foi removido mas ainda rodou", remove)
				}
			}
		})
	}
}

// Remover duas vezes, ou remover um id que nunca existiu, tem que devolver
// false sem mexer no resto.
func TestRemoveEventHandlerUnknownID(t *testing.T) {
	cli := connTestClient()
	id := cli.AddEventHandler(func(evt any) {})

	if !cli.RemoveEventHandler(id) {
		t.Fatal("primeira remocao deveria dar true")
	}
	if cli.RemoveEventHandler(id) {
		t.Error("segunda remocao do mesmo id deveria dar false")
	}
	if cli.RemoveEventHandler(999999) {
		t.Error("id inexistente deveria dar false")
	}
}

func TestRemoveEventHandlersClearsAll(t *testing.T) {
	cli := connTestClient()
	var called int
	for i := 0; i < 5; i++ {
		cli.AddEventHandler(func(evt any) { called++ })
	}

	cli.RemoveEventHandlers()
	cli.dispatchEvent("evt")

	if called != 0 {
		t.Errorf("handlers chamados %d vezes apos RemoveEventHandlers", called)
	}
}

// --- recover do dispatch ---

// Um handler de evento e' codigo da aplicacao. Se ele entra em panic, o
// dispatchEvent tem que absorver: caso contrario um bug do consumidor derruba a
// conexao inteira. O recover tambem precisa liberar o RLock da lista, senao o
// proximo AddEventHandler trava para sempre.
func TestDispatchEventRecoversFromPanickingHandler(t *testing.T) {
	cli := connTestClient()
	cli.AddEventHandler(func(evt any) { panic("handler quebrado") })

	cli.dispatchEvent("evt")

	// Se o RLock tivesse vazado, este AddEventHandler (que pede Lock de
	// escrita) travaria e o teste estouraria o prazo.
	done := make(chan struct{})
	go func() {
		cli.AddEventHandler(func(evt any) {})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("o lock da lista de handlers vazou no recover")
	}
}

// --- guardas de envio de no' ---

// Sem socket, mandar no' tem que virar ErrNotConnected — nunca nil deref.
func TestSendNodeWithoutSocket(t *testing.T) {
	cli := connTestClient()
	err := cli.sendNode(t.Context(), waBinary.Node{Tag: "iq"})
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("err = %v, queria ErrNotConnected", err)
	}
}

func TestSendNodeNilClient(t *testing.T) {
	var cli *Client
	_, err := cli.sendNodeAndGetData(t.Context(), waBinary.Node{Tag: "iq"})
	if !errors.Is(err, ErrClientIsNil) {
		t.Errorf("err = %v, queria ErrClientIsNil", err)
	}
}

// --- constantes da fila de handlers ---

// A fila e' grande de proposito: se encher, handleFrame perde a garantia de
// ordem das mensagens (cai no ramo que despacha em goroutine separada).
func TestHandlerQueueConstants(t *testing.T) {
	if handlerQueueSize < 1024 {
		t.Errorf("handlerQueueSize = %d, pequeno demais para manter ordem", handlerQueueSize)
	}
	if handlerQueueSlowNodeThreshold >= handlerQueueSlowNodeWarnInterval {
		t.Error("o limite de 'demorou' tem que ser menor que o intervalo entre avisos")
	}
	if handlerQueueSlowNodeMaxWarnings <= 0 {
		t.Error("handlerQueueSlowNodeMaxWarnings tem que ser positivo")
	}
}

// --- NewClient ---

// Todo Tag que o servidor manda e que este cliente trata precisa de entrada em
// nodeHandlers. handleFrame consulta o mapa antes de enfileirar; uma entrada
// faltando faz o no' ser silenciosamente ignorado.
func TestNewClientRegistersNodeHandlers(t *testing.T) {
	cli := NewClient(nil, nil)
	want := []string{
		"message", "appdata", "receipt", "call", "chatstate", "presence",
		"notification", "success", "failure", "stream:error", "iq", "ib",
	}
	for _, tag := range want {
		if cli.nodeHandlers[tag] == nil {
			t.Errorf("nodeHandlers[%q] ausente", tag)
		}
	}
	if len(cli.nodeHandlers) != len(want) {
		t.Errorf("nodeHandlers tem %d entradas, queria %d", len(cli.nodeHandlers), len(want))
	}
}

// Os padroes que NewClient assume, e os mapas/canais que o resto do cliente
// espera ja' inicializados — um mapa nil aqui vira panic de escrita depois.
func TestNewClientDefaults(t *testing.T) {
	cli := NewClient(nil, nil)

	if !cli.EnableAutoReconnect {
		t.Error("EnableAutoReconnect deveria vir ligado")
	}
	if !cli.AutoTrustIdentity {
		t.Error("AutoTrustIdentity deveria vir ligado")
	}
	if cli.Log == nil {
		t.Error("logger nil deveria virar o no-op, nao ficar nil")
	}
	if cli.BackgroundEventCtx == nil {
		t.Error("BackgroundEventCtx nao pode ser nil")
	}
	if cli.responseWaiters == nil {
		t.Error("algum mapa interno ficou nil")
	}
	// Idem para o cache de dispositivos (lote 7): user.DeviceCache tambem cria
	// o mapa preguicosamente, entao o observavel e' o zero, nao o mapa nao-nil.
	if cli.userDevicesCache.Len() != 0 {
		t.Error("o cache de dispositivos deveria nascer vazio")
	}
	// O mapa do cache de grupo NAO e' mais criado aqui: group.Cache o cria
	// preguicosamente sob o lock de escrita (mesmo racional do lote 3, do
	// tctoken do lote 4 e do retry do lote 5). Uma leitura antes de qualquer
	// gravacao enxerga mapa nil, o que em Go devolve o zero.
	cli.groupCache.Lock()
	_, cachedGroup := cli.groupCache.GetLocked(types.EmptyJID)
	cli.groupCache.Unlock()
	if cachedGroup {
		t.Error("cache de grupo vazio nao deveria devolver entrada")
	}
	// Os mapas do dominio de retry NAO sao mais criados aqui: retry.State os
	// cria preguicosamente sob o lock de escrita (mesmo racional do lote 3 e do
	// tctoken do lote 4). Uma leitura antes de qualquer gravacao enxerga mapa
	// nil, o que em Go devolve o zero — mesmo resultado que um mapa vazio.
	if got := cli.getRecentMessage(types.EmptyJID, "nao-existe"); !got.IsEmpty() {
		t.Errorf("leitura em cache vazio = %+v, esperado zero", got)
	}
	if cli.handlerQueue == nil || cli.socketWait == nil ||
		cli.historySyncNotifications == nil || cli.expectedDisconnect == nil {
		t.Error("algum canal/evento interno ficou nil")
	}
	if cap(cli.handlerQueue) != handlerQueueSize {
		t.Errorf("cap(handlerQueue) = %d, queria %d", cap(cli.handlerQueue), handlerQueueSize)
	}
	if cap(cli.historySyncNotifications) != historySyncNotificationBufferSize {
		t.Errorf("cap(historySyncNotifications) = %d, queria %d",
			cap(cli.historySyncNotifications), historySyncNotificationBufferSize)
	}
	// GetMessageForRetry tem default nao-nil: o caminho de retry o chama sem
	// checar.
	if cli.GetMessageForRetry == nil {
		t.Error("GetMessageForRetry deveria ter um default nao-nil")
	}
}

// O uniqueID prefixa todo ID de request. Dois clientes tem que sortear
// prefixos independentes, senao dois processos sobre o mesmo device colidem os
// IDs e trocam respostas de IQ entre si.
func TestNewClientUniqueIDPrefixVaries(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		seen[NewClient(nil, nil).uniqueID] = struct{}{}
	}
	if len(seen) < 2 {
		t.Error("todos os clientes sortearam o mesmo uniqueID")
	}
}

// SetMaxParallelRetryReceiptHandling: valores <= 0 significam "ilimitado", que
// e' representado por semaforo nil.
func TestSetMaxParallelRetryReceiptHandling(t *testing.T) {
	cli := NewClient(nil, nil)
	if cli.retryState.Sema() != nil {
		t.Error("o padrao deveria ser ilimitado (semaforo nil)")
	}

	cli.SetMaxParallelRetryReceiptHandling(4)
	if cli.retryState.Sema() == nil {
		t.Error("valor positivo deveria criar o semaforo")
	}

	for _, n := range []int64{0, -1} {
		cli.SetMaxParallelRetryReceiptHandling(4)
		cli.SetMaxParallelRetryReceiptHandling(n)
		if cli.retryState.Sema() != nil {
			t.Errorf("%d deveria voltar a ilimitado", n)
		}
	}
}
