// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-noise/types"
)

func phoneRef(id types.MessageID) MessageRef {
	return MessageRef{Chat: testPeerJID, Sender: testPeerJID, ID: id}
}

// Com o reenvio desligado (o padrao) nada e' agendado — nem entrada no mapa de
// pendentes, nem espera.
func TestDelayedRequestFromPhoneDisabled(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = false
	tr.rerequestDelay = time.Hour // se agendasse, o teste travaria
	DelayedRequestFromPhone(tr, phoneRef("MSG1"))
	if len(tr.state.pendingPhone) != 0 {
		t.Errorf("pendentes = %v, esperado vazio", tr.state.pendingPhone)
	}
	if len(tr.unavailReqs) != 0 {
		t.Errorf("pedidos = %v, esperado nenhum", tr.unavailReqs)
	}
	// Cancelar algo que nunca foi agendado tambem nao pode explodir.
	CancelDelayedFromPhone(tr, "MSG1")
}

// Caminho feliz: espera o delay e vai a' rede.
func TestDelayedRequestFromPhoneCompletes(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = true
	tr.rerequestDelay = time.Millisecond
	DelayedRequestFromPhone(tr, phoneRef("MSG1"))
	if len(tr.unavailReqs) != 1 || tr.unavailReqs[0] != "MSG1" {
		t.Errorf("pedidos = %v, esperado [MSG1]", tr.unavailReqs)
	}
	// O defer tem que ter limpado a entrada.
	if len(tr.state.pendingPhone) != 0 {
		t.Errorf("pendentes = %v, esperado que o defer limpasse", tr.state.pendingPhone)
	}
}

// O caminho que importa: o pedido espera RerequestDelay antes de ir a' rede, e
// CancelDelayedFromPhone (chamado quando a mensagem finalmente chega) tem que
// interromper a espera antes disso.
func TestCancelDelayedFromPhoneInterruptsTheWait(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = true
	tr.rerequestDelay = 30 * time.Second

	done := make(chan struct{})
	go func() {
		defer close(done)
		DelayedRequestFromPhone(tr, phoneRef("MSG1"))
	}()

	// Espera o agendamento aparecer no mapa antes de cancelar.
	deadline := time.Now().Add(2 * time.Second)
	for {
		tr.state.pendingPhoneLock.RLock()
		_, scheduled := tr.state.pendingPhone["MSG1"]
		tr.state.pendingPhoneLock.RUnlock()
		if scheduled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("o pedido nunca foi registrado como pendente")
		}
		time.Sleep(time.Millisecond)
	}

	CancelDelayedFromPhone(tr, "MSG1")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("o cancelamento nao interrompeu a espera")
	}
	if len(tr.unavailReqs) != 0 {
		t.Errorf("cancelado, mas foi a' rede: %v", tr.unavailReqs)
	}
	if len(tr.state.pendingPhone) != 0 {
		t.Errorf("pendentes = %v, esperado que o defer limpasse", tr.state.pendingPhone)
	}
}

// Um segundo pedido para o mesmo ID enquanto o primeiro espera e' descartado
// sem sobrescrever o cancelador do primeiro.
func TestDelayedRequestFromPhoneIsDeduplicated(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = true
	tr.rerequestDelay = 30 * time.Second

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		DelayedRequestFromPhone(tr, phoneRef("MSG1"))
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		tr.state.pendingPhoneLock.RLock()
		_, scheduled := tr.state.pendingPhone["MSG1"]
		tr.state.pendingPhoneLock.RUnlock()
		if scheduled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("o pedido nunca foi registrado")
		}
		time.Sleep(time.Millisecond)
	}

	// O segundo volta na hora, sem esperar os 30s.
	returned := make(chan struct{})
	go func() {
		DelayedRequestFromPhone(tr, phoneRef("MSG1"))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("o pedido duplicado nao voltou imediatamente")
	}

	CancelDelayedFromPhone(tr, "MSG1")
	wg.Wait()
}

// ClearDelayedRequests cancela tudo de uma vez no disconnect.
func TestClearDelayedRequests(t *testing.T) {
	tr := newFakeTransport()
	cancelled := make(chan string, 2)
	for _, id := range []types.MessageID{"MSG1", "MSG2"} {
		id := id
		tr.state.RegisterPendingPhone(id, func() { cancelled <- string(id) })
	}
	ClearDelayedRequests(tr)
	if len(cancelled) != 2 {
		t.Errorf("cancelamentos = %d, esperado 2", len(cancelled))
	}
}

// ImmediateRequestFromPhone nao checa RerequestFromPhoneEnabled: o caminho de
// decriptacao a chama direto quando SynchronousAck esta' ligado.
func TestImmediateRequestFromPhoneIgnoresTheFlag(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = false
	ImmediateRequestFromPhone(context.Background(), tr, phoneRef("MSG1"))
	if len(tr.unavailReqs) != 1 {
		t.Errorf("pedidos = %v, esperado 1", tr.unavailReqs)
	}
}

// Falha de rede e' logada e engolida — a funcao nao devolve nada.
func TestImmediateRequestFromPhoneLogsError(t *testing.T) {
	tr := newFakeTransport()
	tr.requestUnavailErr = errors.New("sem socket")
	ImmediateRequestFromPhone(context.Background(), tr, phoneRef("MSG1"))
	if len(tr.unavailReqs) != 1 {
		t.Errorf("pedidos = %v, esperado 1 tentativa", tr.unavailReqs)
	}
}

// CancelPendingPhone com ID desconhecido e' no-op.
func TestCancelPendingPhoneUnknownID(t *testing.T) {
	tr := newFakeTransport()
	tr.rerequestEnabled = true
	CancelDelayedFromPhone(tr, "nao-existe")
}
