// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-noise/prekeys"
	"wa-api/internal/wa-noise/persistence/store"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// TestPreKeyTransportEspelhaOCliente confere que o adaptador entrega os mesmos
// objetos do cliente, e nao copias — em especial o ponteiro do estado, que
// carrega o mutex de upload e por isso nunca pode ser copiado por valor.
func TestPreKeyTransportEspelhaOCliente(t *testing.T) {
	t.Parallel()
	device := &store.Device{}
	cli := &Client{Log: waLog.Noop, Store: device}
	tp := cli.preKeyT()

	if tp.Store() != device {
		t.Error("Store() deveria devolver o mesmo *store.Device do cliente")
	}
	if tp.Log() != cli.Log {
		t.Error("Log() deveria devolver o logger do cliente")
	}
	if tp.State() != &cli.preKeyState || tp.State() != cli.preKeyT().State() {
		t.Error("State() deveria apontar sempre para o campo do cliente")
	}
}

// TestConstantesDePreKeySaoAsDoSubpacote trava o aliasing das duas constantes
// exportadas: elas continuam existindo na raiz com o nome historico, mas o
// valor vem do subpacote.
func TestConstantesDePreKeySaoAsDoSubpacote(t *testing.T) {
	t.Parallel()
	if WantedPreKeyCount != prekeys.WantedCount {
		t.Errorf("WantedPreKeyCount = %d, esperado %d", WantedPreKeyCount, prekeys.WantedCount)
	}
	if MinPreKeyCount != prekeys.MinCount {
		t.Errorf("MinPreKeyCount = %d, esperado %d", MinPreKeyCount, prekeys.MinCount)
	}
	// A politica so' faz sentido nesta ordem: o limiar que dispara um upload e'
	// menor que o lote regular, que e' menor que o lote inicial.
	if !(MinPreKeyCount < WantedPreKeyCount && WantedPreKeyCount < prekeys.InitialCount) {
		t.Errorf("politica incoerente: Min=%d Wanted=%d Initial=%d",
			MinPreKeyCount, WantedPreKeyCount, prekeys.InitialCount)
	}
}

// TestFachadaDePreKeysRecusaClientNil trava as guardas de receiver nil dos
// metodos-fachada. Antes deste lote todos estouravam nil deref (menos o
// fetchPreKeysNoError com lista vazia); agora devolvem ErrClientIsNil ou o
// zero equivalente, na mesma linha dos lotes 1 a 3. Inclui os nao exportados,
// que sao alcancaveis por DangerousInternalClient.
func TestFachadaDePreKeysRecusaClientNil(t *testing.T) {
	t.Parallel()
	var cli *Client
	ctx := context.Background()

	if _, err := cli.getServerPreKeyCount(ctx); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("getServerPreKeyCount: erro = %v, esperado ErrClientIsNil", err)
	}
	if _, err := cli.fetchPreKeys(ctx, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("fetchPreKeys: erro = %v, esperado ErrClientIsNil", err)
	}
	if got := cli.fetchPreKeysNoError(ctx, nil); got != nil {
		t.Errorf("fetchPreKeysNoError = %v, esperado nil", got)
	}
	// uploadPreKeys nao devolve nada; o contrato e' nao entrar em panico.
	cli.uploadPreKeys(ctx, false)
}
