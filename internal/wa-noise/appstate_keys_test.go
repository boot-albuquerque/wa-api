// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"testing"
	"time"

	waLog "wa-api/internal/wa-noise/util/log"
)

// TestRequestAppStateKeysSemChaves trava a guarda de lista vazia: sem ela, o
// codigo enviaria uma mensagem de peer com zero key IDs a cada patch que
// falhasse por outro motivo. Com a guarda, nada e' enviado — e por isso este
// Client sem socket nem Store nao entra em panico.
func TestRequestAppStateKeysSemChaves(t *testing.T) {
	t.Parallel()
	cli := &Client{Log: waLog.Noop}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("requestAppStateKeys tentou enviar com lista vazia: %v", r)
		}
	}()
	cli.requestAppStateKeys(context.Background(), nil)
	cli.requestAppStateKeys(context.Background(), [][]byte{})
}

// TestAppStateKeyRequestInterval documenta o valor do throttle de pedidos de
// chave: um dia entre dois pedidos da mesma chave.
func TestAppStateKeyRequestInterval(t *testing.T) {
	t.Parallel()
	if appStateKeyRequestInterval != 24*time.Hour {
		t.Errorf("appStateKeyRequestInterval = %v, esperado 24h", appStateKeyRequestInterval)
	}
}
