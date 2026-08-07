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

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/pairing"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// TestPairTransportEspelhaOCliente confere que o adaptador entrega os mesmos
// objetos do cliente, e nao copias.
func TestPairTransportEspelhaOCliente(t *testing.T) {
	t.Parallel()
	device := &store.Device{}
	cli := &Client{Log: waLog.Noop, Store: device, QRClientType: PairClientFirefox}
	tp := cli.pairT()

	if tp.Store() != device {
		t.Error("Store() deveria devolver o mesmo *store.Device do cliente")
	}
	if tp.Log() != cli.Log {
		t.Error("Log() deveria devolver o logger do cliente")
	}
	if tp.State() != &cli.pairState || tp.State() != cli.pairT().State() {
		t.Error("State() deveria apontar sempre para o campo do cliente")
	}
	if tp.ConfiguredClientType() != PairClientFirefox {
		t.Errorf("ConfiguredClientType = %q, esperado %q", tp.ConfiguredClientType(), PairClientFirefox)
	}
	var missing *ElementMissingError
	if !errors.As(tp.ElementMissing("x", "y"), &missing) {
		t.Error("ElementMissing deveria devolver o *ElementMissingError da raiz")
	}
}

// PrePairAllowed reproduz o `cli.PrePairCallback != nil && !cli.PrePairCallback(...)`
// original: sem callback, o pareamento passa.
func TestPairTransportPrePairAllowed(t *testing.T) {
	t.Parallel()
	jid := types.NewJID("55119", types.DefaultUserServer)

	cli := &Client{Log: waLog.Noop, Store: &store.Device{}}
	if !cli.pairT().PrePairAllowed(jid, "android", "Loja") {
		t.Error("sem callback configurado o pareamento deveria ser permitido")
	}

	var gotJID types.JID
	var gotPlatform, gotBusiness string
	cli.PrePairCallback = func(j types.JID, platform, businessName string) bool {
		gotJID, gotPlatform, gotBusiness = j, platform, businessName
		return false
	}
	if cli.pairT().PrePairAllowed(jid, "android", "Loja") {
		t.Error("o callback devolveu false; o pareamento deveria ser recusado")
	}
	if gotJID != jid || gotPlatform != "android" || gotBusiness != "Loja" {
		t.Errorf("callback recebeu (%v, %q, %q), esperado (%v, \"android\", \"Loja\")",
			gotJID, gotPlatform, gotBusiness, jid)
	}
}

// TestErrosDePareamentoSaoOsMesmosValores trava o aliasing dos sentinelas: se a
// raiz declarasse errors.New proprios, errors.Is falharia para quem compara com
// o nome da raiz contra um erro produzido dentro do subpacote.
func TestErrosDePareamentoSaoOsMesmosValores(t *testing.T) {
	t.Parallel()
	for name, pair := range map[string][2]error{
		"ErrPairInvalidDeviceIdentityHMAC": {ErrPairInvalidDeviceIdentityHMAC, pairing.ErrInvalidDeviceIdentityHMAC},
		"ErrPairInvalidDeviceSignature":    {ErrPairInvalidDeviceSignature, pairing.ErrInvalidDeviceSignature},
		"ErrPairRejectedLocally":           {ErrPairRejectedLocally, pairing.ErrRejectedLocally},
		"ErrPhoneNumberTooShort":           {ErrPhoneNumberTooShort, pairing.ErrPhoneNumberTooShort},
		"ErrPhoneNumberIsNotInternational": {ErrPhoneNumberIsNotInternational, pairing.ErrPhoneNumberIsNotInternational},
	} {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(pair[0], pair[1]) || !errors.Is(pair[1], pair[0]) {
				t.Errorf("%s nao e' o mesmo valor do sentinela do subpacote", name)
			}
		})
	}
}

// TestFachadaDePareamentoRecusaClientNil trava as guardas de receiver nil das
// fachadas. Inclui as nao exportadas, alcancaveis por DangerousInternalClient.
func TestFachadaDePareamentoRecusaClientNil(t *testing.T) {
	t.Parallel()
	var cli *Client
	ctx := context.Background()
	node := &waBinary.Node{Tag: "iq"}

	if _, err := cli.PairPhone(ctx, "5511999999999", false, PairClientChrome, "Chrome (Linux)"); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("PairPhone: erro = %v, esperado ErrClientIsNil", err)
	}
	if err := cli.handlePair(ctx, nil, "", "", "", types.EmptyJID, types.EmptyJID); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("handlePair: erro = %v, esperado ErrClientIsNil", err)
	}
	if err := cli.handleCodePairNotification(ctx, node); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("handleCodePairNotification: erro = %v, esperado ErrClientIsNil", err)
	}
	if got := cli.makeQRData(nil, PairClientChrome); got != "" {
		t.Errorf("makeQRData = %q, esperado string vazia", got)
	}
	// getQRClientType com cliente nil cai na deducao por DeviceProps, que
	// nunca entra em panico e sempre devolve algum tipo.
	if got := cli.getQRClientType(); got == "" {
		t.Error("getQRClientType deveria devolver algum tipo mesmo com cliente nil")
	}
	// Os quatro abaixo nao devolvem nada; o contrato e' nao entrar em panico.
	cli.handleIQ(ctx, node)
	cli.handlePairDevice(ctx, node)
	cli.handlePairSuccess(ctx, node)
	cli.sendPairError(ctx, "id", 500, "internal-error")
	cli.tryHandleCodePairNotification(ctx, node)
}
