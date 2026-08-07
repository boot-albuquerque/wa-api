// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-noise/types"
)

// O zero value de State e' usavel: todos os mapas nascem preguicosamente sob o
// lock de escrita, e ler antes de qualquer gravacao devolve o zero.
func TestStateZeroValueIsUsable(t *testing.T) {
	var s State
	if got := s.GetRecent(RecentKey{testPeerJID, "X"}); !got.IsEmpty() {
		t.Errorf("leitura em cache nil = %+v, esperado zero", got)
	}
	if !s.LastStoreClear().IsZero() {
		t.Error("LastStoreClear deveria comecar zerada")
	}
	if s.Sema() != nil {
		t.Error("o padrao e' ilimitado (semaforo nil)")
	}
	s.CancelAllPendingPhone()
	s.UnregisterPendingPhone("X")
	s.CancelPendingPhone("X")
	s.LockSessionRecreate()
	if _, ok := s.LastSessionRecreate(testPeerJID); ok {
		t.Error("historico vazio nao deveria ter entrada")
	}
	s.UnlockSessionRecreate()
}

func TestIncrementIncoming(t *testing.T) {
	var s State
	key := IncomingKey{testPeerJID, "MSG1"}
	for want := 1; want <= 3; want++ {
		if got := s.IncrementIncoming(key); got != want {
			t.Errorf("contador = %d, esperado %d", got, want)
		}
	}
	// Chave diferente conta separado.
	if got := s.IncrementIncoming(IncomingKey{testPeerJID, "MSG2"}); got != 1 {
		t.Errorf("contador da segunda chave = %d, esperado 1", got)
	}
	if got := s.IncrementIncoming(IncomingKey{testOwnJID, "MSG1"}); got != 1 {
		t.Errorf("contador do segundo remetente = %d, esperado 1", got)
	}
}

// Os dois contadores de F36 sao secoes criticas de verdade: incrementos
// concorrentes nao podem perder contagem.
func TestCountersAreSerialized(t *testing.T) {
	var s State
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(2 * goroutines)
	for i := 0; i < goroutines; i++ {
		go func() { defer wg.Done(); s.IncrementIncoming(IncomingKey{testPeerJID, "MSG1"}) }()
		go func() { defer wg.Done(); s.BumpMessageRetries("MSG1", 0) }()
	}
	wg.Wait()
	if got := s.IncrementIncoming(IncomingKey{testPeerJID, "MSG1"}); got != goroutines+1 {
		t.Errorf("contador de entrada = %d, esperado %d", got, goroutines+1)
	}
	if got := s.BumpMessageRetries("MSG1", 0); got != goroutines+1 {
		t.Errorf("contador de saida = %d, esperado %d", got, goroutines+1)
	}
}

func TestBumpMessageRetriesResumeRule(t *testing.T) {
	t.Run("countInMsg so' vale na primeira", func(t *testing.T) {
		var s State
		if got := s.BumpMessageRetries("MSG1", 4); got != 5 {
			t.Errorf("primeira = %d, esperado 5", got)
		}
		if got := s.BumpMessageRetries("MSG1", 9); got != 6 {
			t.Errorf("segunda = %d, esperado 6 (o countInMsg e' ignorado)", got)
		}
	})
	t.Run("countInMsg zero nao muda nada", func(t *testing.T) {
		var s State
		if got := s.BumpMessageRetries("MSG1", 0); got != 1 {
			t.Errorf("contador = %d, esperado 1", got)
		}
	})
}

func TestSetMaxParallel(t *testing.T) {
	var s State
	s.SetMaxParallel(4)
	if s.Sema() == nil {
		t.Error("valor positivo deveria criar o semaforo")
	}
	for _, n := range []int64{0, -1} {
		s.SetMaxParallel(4)
		s.SetMaxParallel(n)
		if s.Sema() != nil {
			t.Errorf("%d deveria voltar a ilimitado", n)
		}
	}
}

// AddRecent concorrente nao pode corromper o anel: o mapa fica com exatamente
// tantas entradas quantas insercoes distintas, ate' o teto.
func TestAddRecentIsSerialized(t *testing.T) {
	var s State
	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			s.AddRecent(RecentKey{testPeerJID, types.MessageID(string(rune('A'+i%26)) + string(rune('a'+i/26)))}, RecentMessage{WA: waMessage("x")})
		}(i)
	}
	wg.Wait()
	s.recentLock.RLock()
	defer s.recentLock.RUnlock()
	if len(s.recentMap) != goroutines {
		t.Errorf("entradas = %d, esperado %d", len(s.recentMap), goroutines)
	}
	if s.recentPtr != goroutines {
		t.Errorf("ponteiro = %d, esperado %d", s.recentPtr, goroutines)
	}
}

func TestSessionRecreateHistory(t *testing.T) {
	var s State
	now := time.Now()
	s.LockSessionRecreate()
	s.MarkSessionRecreated(testPeerJID, now)
	got, ok := s.LastSessionRecreate(testPeerJID)
	s.UnlockSessionRecreate()
	if !ok || !got.Equal(now) {
		t.Errorf("historico = (%v, %v), esperado (%v, true)", got, ok, now)
	}
}

// O lock de recriacao e' exclusivo de verdade.
func TestSessionRecreateLockIsMutuallyExclusive(t *testing.T) {
	var s State
	s.LockSessionRecreate()
	acquired := make(chan struct{})
	go func() {
		s.LockSessionRecreate()
		close(acquired)
		s.UnlockSessionRecreate()
	}()
	select {
	case <-acquired:
		t.Fatal("o segundo Lock passou com o primeiro segurado")
	case <-time.After(50 * time.Millisecond):
	}
	s.UnlockSessionRecreate()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("o segundo Lock nao passou depois do Unlock")
	}
}

func TestRegisterPendingPhoneDeduplicates(t *testing.T) {
	var s State
	if !s.RegisterPendingPhone("MSG1", func() {}) {
		t.Fatal("o primeiro registro deveria ser aceito")
	}
	if s.RegisterPendingPhone("MSG1", func() {}) {
		t.Error("o segundo registro do mesmo ID deveria ser recusado")
	}
	s.UnregisterPendingPhone("MSG1")
	if !s.RegisterPendingPhone("MSG1", func() {}) {
		t.Error("depois do Unregister o ID deveria voltar a ser aceito")
	}
}

// CancelAllPendingPhone nao apaga o mapa: cada goroutine cancelada remove a
// propria entrada pelo defer dela. Comportamento do
// clearDelayedMessageRequests original.
func TestCancelAllPendingPhoneKeepsTheMap(t *testing.T) {
	var s State
	var n int
	s.RegisterPendingPhone("MSG1", func() { n++ })
	s.CancelAllPendingPhone()
	if n != 1 {
		t.Errorf("cancelamentos = %d, esperado 1", n)
	}
	s.pendingPhoneLock.RLock()
	defer s.pendingPhoneLock.RUnlock()
	if len(s.pendingPhone) != 1 {
		t.Errorf("entradas = %d, esperado 1 (a limpeza e' de quem espera)", len(s.pendingPhone))
	}
}

// --- coerencia das constantes de politica ---

func TestPolicyConstantsAreCoherent(t *testing.T) {
	if MinCountForSessionRecreate < 1 {
		t.Errorf("MinCountForSessionRecreate = %d: abaixo de 1 recriaria sessao no primeiro retry",
			MinCountForSessionRecreate)
	}
	// Nos paramos de mandar recibo de retry em MaxOutgoingReceipts; o teto do
	// lado que *responde* precisa ser pelo menos tao grande, senao pararamos de
	// responder a retries que nos mesmos ainda estariamos pedindo.
	if MaxIncomingRequests < MaxOutgoingReceipts {
		t.Errorf("MaxIncomingRequests (%d) < MaxOutgoingReceipts (%d)",
			MaxIncomingRequests, MaxOutgoingReceipts)
	}
	if ReceiptVersion != 1 {
		t.Errorf("ReceiptVersion = %d: mudar a versao do <retry> muda o wire format", ReceiptVersion)
	}
	if StoreFormatWA == StoreFormatFB {
		t.Error("os dois formatos do store de retry precisam ser distinguiveis")
	}
	if FBApplicationVersion != 2 {
		t.Errorf("FBApplicationVersion = %d: e' contrato de wire com o servidor", FBApplicationVersion)
	}
}

// Sanidade do contexto de fundo: e' o que sobrevive ao fim da requisicao.
func TestBackgroundCtxIsUsedByDelayedRequest(t *testing.T) {
	tr := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	tr.backgroundCtx = ctx
	tr.rerequestEnabled = true
	tr.rerequestDelay = 30 * time.Second

	done := make(chan struct{})
	go func() { defer close(done); DelayedRequestFromPhone(tr, phoneRef("MSG1")) }()
	time.Sleep(20 * time.Millisecond)
	cancel() // cancelar o contexto de fundo derruba a espera
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("o cancelamento do contexto de fundo nao interrompeu a espera")
	}
}
