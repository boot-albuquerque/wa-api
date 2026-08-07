// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"
)

// Testes relocados de internal/wa-noise/appstate_keys_test.go (Fase E lote 3) e
// adaptados ao duble de Transport, mais os caminhos que antes exigiriam um
// *Client com socket.

// TestRequestAppStateKeysSemChaves trava a guarda de lista vazia: sem ela, o
// codigo enviaria uma mensagem de peer com zero key IDs a cada patch que
// falhasse por outro motivo.
func TestRequestAppStateKeysSemChaves(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	RequestKeys(context.Background(), tp, nil)
	RequestKeys(context.Background(), tp, [][]byte{})
	if len(tp.peerMsgs) != 0 {
		t.Errorf("nao deveria ter enviado nada, enviou %d", len(tp.peerMsgs))
	}
}

// TestAppStateKeyRequestInterval documenta o valor do throttle de pedidos de
// chave: um dia entre dois pedidos da mesma chave.
func TestAppStateKeyRequestInterval(t *testing.T) {
	t.Parallel()
	if KeyRequestInterval != 24*time.Hour {
		t.Errorf("KeyRequestInterval = %v, esperado 24h", KeyRequestInterval)
	}
}

func TestRequestKeysMontaAMensagem(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	RequestKeys(context.Background(), tp, [][]byte{{1, 2}, {3, 4}})
	if len(tp.peerMsgs) != 1 {
		t.Fatalf("esperado 1 mensagem, veio %d", len(tp.peerMsgs))
	}
	req := tp.peerMsgs[0].GetProtocolMessage().GetAppStateSyncKeyRequest()
	if len(req.GetKeyIDs()) != 2 {
		t.Fatalf("esperado 2 key IDs, veio %d", len(req.GetKeyIDs()))
	}
	if hex.EncodeToString(req.GetKeyIDs()[1].GetKeyID()) != "0304" {
		t.Errorf("segundo key ID = %X", req.GetKeyIDs()[1].GetKeyID())
	}
}

// TestRequestKeysErroDeEnvioSoLoga: a falha no envio nao propaga (a funcao nem
// devolve erro) — o sync sera' retentado na proxima notificacao.
func TestRequestKeysErroDeEnvioSoLoga(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	tp.peerMsgErr = errors.New("sem sessao")
	RequestKeys(context.Background(), tp, [][]byte{{9}})
}

// --- State.FilterKeyIDs ---

func TestFilterKeyIDsThrottle(t *testing.T) {
	t.Parallel()
	var s State
	now := time.Now()
	raw := func() [][]byte { return [][]byte{{1}, {2}} }

	first := s.FilterKeyIDs(now, raw)
	if len(first) != 2 {
		t.Fatalf("primeiro pedido deveria passar as 2 chaves, veio %d", len(first))
	}
	// Dentro da janela, nenhuma passa de novo.
	if again := s.FilterKeyIDs(now.Add(time.Hour), raw); len(again) != 0 {
		t.Errorf("dentro da janela nenhuma chave deveria passar, veio %d", len(again))
	}
	// Passada a janela, voltam a passar.
	if later := s.FilterKeyIDs(now.Add(KeyRequestInterval+time.Second), raw); len(later) != 2 {
		t.Errorf("depois da janela as 2 deveriam voltar, veio %d", len(later))
	}
}

// TestFilterKeyIDsCalculaSobOLock trava a decisao de desenho: a funcao que
// produz os key IDs e' chamada DENTRO da secao critica. Se fosse chamada fora,
// duas goroutines poderiam calcular a mesma lista e as duas passarem no filtro.
func TestFilterKeyIDsCalculaSobOLock(t *testing.T) {
	t.Parallel()
	var s State
	var mu sync.Mutex
	var chamadas int
	var total int
	var wg sync.WaitGroup
	now := time.Now()
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := s.FilterKeyIDs(now, func() [][]byte {
				mu.Lock()
				chamadas++
				mu.Unlock()
				return [][]byte{{7}}
			})
			mu.Lock()
			total += len(got)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if chamadas != 8 {
		t.Errorf("a funcao produtora deveria rodar 1x por chamada, rodou %d", chamadas)
	}
	// Exatamente uma goroutine pode ter passado a chave adiante.
	if total != 1 {
		t.Errorf("a chave deveria ter passado exatamente 1 vez, passou %d", total)
	}
}

// --- State.ReadKeyRequests ---

func TestReadKeyRequests(t *testing.T) {
	t.Parallel()
	var s State
	// Antes de qualquer gravacao o mapa e' nil; ler dele e' valido em Go e nao
	// pode entrar em panico.
	s.ReadKeyRequests(func(wasRequested func(string) bool) {
		if wasRequested("00") {
			t.Error("mapa vazio nao deveria conter nada")
		}
	})

	s.FilterKeyIDs(time.Now(), func() [][]byte { return [][]byte{{0xab}} })
	s.ReadKeyRequests(func(wasRequested func(string) bool) {
		if !wasRequested("ab") {
			t.Error("a chave pedida deveria constar")
		}
		if wasRequested("cd") {
			t.Error("chave nunca pedida nao deveria constar")
		}
	})
}

// TestRequestMissingKeysUsaOFiltro liga as duas pontas: o que sai do filtro e'
// o que vai para o envio, e um segundo pedido imediato nao reenvia nada.
func TestRequestMissingKeysNaoReenviaDentroDaJanela(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	patches := removeOnlyPatchList()

	RequestMissingKeys(context.Background(), tp, patches)
	primeiro := len(tp.peerMsgs)
	RequestMissingKeys(context.Background(), tp, patches)
	if len(tp.peerMsgs) != primeiro {
		t.Errorf("o segundo pedido nao deveria reenviar: %d -> %d", primeiro, len(tp.peerMsgs))
	}
}

// TestLockSyncSerializa: LockSync/UnlockSync sao o mesmo mutex nao reentrante
// que serializava fetchAppState antes da extracao.
func TestLockSyncSerializa(t *testing.T) {
	t.Parallel()
	var s State
	var wg sync.WaitGroup
	var dentro, maxDentro int
	var mu sync.Mutex
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.LockSync()
			mu.Lock()
			dentro++
			if dentro > maxDentro {
				maxDentro = dentro
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			dentro--
			mu.Unlock()
			s.UnlockSync()
		}()
	}
	wg.Wait()
	if maxDentro != 1 {
		t.Errorf("ate %d goroutines dentro da secao critica, esperado 1", maxDentro)
	}
}
