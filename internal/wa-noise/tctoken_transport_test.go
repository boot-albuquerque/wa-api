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
	"time"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/tctoken"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// TestTCTokenTransportEspelhaOCliente confere que o adaptador entrega os mesmos
// objetos do cliente, e nao copias — em especial o ponteiro do estado, que
// carrega os dois mutexes do dominio.
func TestTCTokenTransportEspelhaOCliente(t *testing.T) {
	t.Parallel()
	device := &store.Device{}
	ctx := context.Background()
	cli := &Client{Log: waLog.Noop, Store: device, BackgroundEventCtx: ctx}
	tp := cli.tcTokenT()

	if tp.Store() != device {
		t.Error("Store() deveria devolver o mesmo *store.Device do cliente")
	}
	if tp.Log() != cli.Log {
		t.Error("Log() deveria devolver o logger do cliente")
	}
	if tp.BackgroundCtx() != ctx {
		t.Error("BackgroundCtx() deveria devolver o BackgroundEventCtx do cliente")
	}
	if tp.State() != &cli.tcToken || tp.State() != cli.tcTokenT().State() {
		t.Error("State() deveria apontar sempre para o campo do cliente")
	}
}

// shouldSendTCTokenInChatAction (fachada de tctoken.ShouldSendInChatAction) e
// shouldSendCsToken tem corpo identico hoje. O teste roda a mesma tabela nos
// dois para que uma divergencia futura apareca — era um teste so' antes da
// extracao, e continua cobrindo os dois lados.
func TestShouldSendTokenPerJIDType(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		jid  types.JID
		want bool
	}{
		"usuario comum":       {types.NewJID("5511999999999", types.DefaultUserServer), true},
		"usuario oculto/LID":  {types.NewJID("12345", types.HiddenUserServer), true},
		"com device":          {types.JID{User: "5511999999999", Server: types.DefaultUserServer, Device: 3}, true},
		"grupo":               {types.NewJID("123-456", types.GroupServer), false},
		"broadcast de status": {types.StatusBroadcastJID, false},
		"newsletter":          {types.NewJID("123", types.NewsletterServer), false},
		"PSA":                 {types.PSAJID, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := shouldSendTCTokenInChatAction(tc.jid); got != tc.want {
				t.Errorf("shouldSendTCTokenInChatAction = %v, esperado %v", got, tc.want)
			}
			if got := shouldSendCsToken(tc.jid); got != tc.want {
				t.Errorf("shouldSendCsToken = %v, esperado %v", got, tc.want)
			}
		})
	}
}

// A fachada de shouldSendNewTCToken continua respondendo pela politica de
// bucket; o detalhe esta' coberto no subpacote.
func TestShouldSendNewTCTokenFachada(t *testing.T) {
	t.Parallel()
	if !shouldSendNewTCToken(time.Time{}) {
		t.Error("nunca emitido deveria pedir token novo")
	}
	if shouldSendNewTCToken(time.Now()) {
		t.Error("emitido agora nao deveria pedir token novo")
	}
}

// TestFachadaDeTCTokenRecusaClientNil trava as guardas de receiver nil. Antes
// deste lote todas estouravam nil deref; agora devolvem ErrClientIsNil ou o
// zero equivalente, na mesma linha dos lotes 1 a 3.
func TestFachadaDeTCTokenRecusaClientNil(t *testing.T) {
	t.Parallel()
	var cli *Client
	ctx := context.Background()
	jid := types.NewJID("5511999999999", types.DefaultUserServer)

	if _, err := cli.ensureTCToken(ctx, jid); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("ensureTCToken: erro = %v, esperado ErrClientIsNil", err)
	}
	if _, err := cli.issuePrivacyToken(ctx, jid, time.Now()); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("issuePrivacyToken: erro = %v, esperado ErrClientIsNil", err)
	}
	if got := cli.getTCTokenSenderTS(jid); !got.IsZero() {
		t.Errorf("getTCTokenSenderTS = %v, esperado zero", got)
	}
	if cli.validateAndSetTCTokenSenderTS(jid, time.Now()) {
		t.Error("validateAndSetTCTokenSenderTS deveria devolver false com cliente nil")
	}
	if got := cli.resolveTCTokenStorageLID(ctx, jid); got != jid.ToNonAD() {
		t.Errorf("resolveTCTokenStorageLID = %v, esperado %v", got, jid.ToNonAD())
	}
	// Os dois abaixo nao devolvem nada; o contrato e' nao entrar em panico.
	cli.setTCTokenSenderTS(jid, time.Now())
	cli.deleteExpiredPrivacyTokens()
	cli.issuePrivacyTokenAndSave(jid, time.Now())
}

// Guarda de identidade das constantes reexportadas pelo dominio.
func TestConstantesDeTCTokenSaoAsDoSubpacote(t *testing.T) {
	t.Parallel()
	if tctoken.NumBuckets*tctoken.BucketDuration != 4*604800 {
		t.Errorf("janela = %d s, esperado 4 buckets de 7 dias", tctoken.NumBuckets*tctoken.BucketDuration)
	}
	if tctoken.DBPruneInterval != 24*time.Hour {
		t.Errorf("DBPruneInterval = %v, esperado 24h", tctoken.DBPruneInterval)
	}
}
