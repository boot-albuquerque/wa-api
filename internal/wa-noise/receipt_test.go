// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"strconv"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// --- infraestrutura compartilhada dos testes do lote 5 ---

var (
	receiptTestOwnJID   = types.NewJID("111", types.DefaultUserServer)
	receiptTestOwnLID   = types.NewJID("222", types.HiddenUserServer)
	receiptTestPeerJID  = types.NewJID("333", types.DefaultUserServer)
	receiptTestGroupJID = types.NewJID("444", types.GroupServer)
)

// notifTestClient monta o minimo de Client que os handlers de notificacao,
// retry e recibo precisam para rodar sem socket: logger no-op e um Store com
// identidade propria, que e' o que parseMessageSource e getOwnID exigem.
func notifTestClient() *Client {
	ownID := receiptTestOwnJID
	ownLID := receiptTestOwnLID
	return &Client{
		Log:   waLog.Noop,
		Store: &store.Device{ID: &ownID, LID: ownLID},
	}
}

// captureEvents registra um event handler que acumula tudo que passar por
// dispatchEvent, e devolve o acumulador. dispatchEvent nao copia o evento,
// entao os ponteiros guardados sao os mesmos que o handler recebeu.
func captureEvents(cli *Client) *[]any {
	captured := make([]any, 0, 4)
	cli.AddEventHandler(func(evt any) {
		captured = append(captured, evt)
	})
	return &captured
}

func unixAttr(ts time.Time) string {
	return strconv.FormatInt(ts.Unix(), 10)
}

// --- parseReceipt ---

func TestParseReceiptSingleMessageID(t *testing.T) {
	cli := notifTestClient()
	ts := time.Unix(1700000000, 0)
	receipt, err := cli.parseReceipt(&waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"from": receiptTestPeerJID,
			"id":   "MSG1",
			"t":    unixAttr(ts),
			"type": "read",
		},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if receipt == nil {
		t.Fatal("recibo nil")
	}
	if receipt.Type != types.ReceiptTypeRead {
		t.Errorf("Type = %q, esperado %q", receipt.Type, types.ReceiptTypeRead)
	}
	if !receipt.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, esperado %v", receipt.Timestamp, ts)
	}
	if len(receipt.MessageIDs) != 1 || receipt.MessageIDs[0] != "MSG1" {
		t.Errorf("MessageIDs = %v, esperado [MSG1]", receipt.MessageIDs)
	}
	if receipt.Sender != receiptTestPeerJID {
		t.Errorf("Sender = %v, esperado %v", receipt.Sender, receiptTestPeerJID)
	}
}

// O tipo ausente e' valido (recibo de entrega), e o parser precisa devolver
// string vazia em vez de erro — e' OptionalString, nao String.
func TestParseReceiptWithoutTypeIsDelivery(t *testing.T) {
	cli := notifTestClient()
	receipt, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestPeerJID, "id": "MSG1", "t": "1700000000"},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if receipt.Type != types.ReceiptTypeDelivered {
		t.Errorf("Type = %q, esperado vazio (delivered)", receipt.Type)
	}
}

// O <list> agrega IDs extras: o `id` do proprio <receipt> continua sendo o
// primeiro da lista, e os <item> entram depois, na ordem em que vieram.
func TestParseReceiptListAppendsExtraIDs(t *testing.T) {
	cli := notifTestClient()
	receipt, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestPeerJID, "id": "MSG1", "t": "1700000000"},
		Content: []waBinary.Node{{
			Tag: "list",
			Content: []waBinary.Node{
				{Tag: "item", Attrs: waBinary.Attrs{"id": "MSG2"}},
				// Filho de tag errada e' ignorado silenciosamente.
				{Tag: "outro", Attrs: waBinary.Attrs{"id": "MSG3"}},
				// <item> sem id tambem.
				{Tag: "item", Attrs: waBinary.Attrs{}},
				{Tag: "item", Attrs: waBinary.Attrs{"id": "MSG4"}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := []types.MessageID{"MSG1", "MSG2", "MSG4"}
	if len(receipt.MessageIDs) != len(want) {
		t.Fatalf("MessageIDs = %v, esperado %v", receipt.MessageIDs, want)
	}
	for i := range want {
		if receipt.MessageIDs[i] != want[i] {
			t.Errorf("MessageIDs[%d] = %q, esperado %q", i, receipt.MessageIDs[i], want[i])
		}
	}
}

// Mais de um filho, ou um filho que nao e' <list>, cai no caminho de ID unico.
func TestParseReceiptIgnoresNonListChildren(t *testing.T) {
	cli := notifTestClient()
	receipt, err := cli.parseReceipt(&waBinary.Node{
		Tag:     "receipt",
		Attrs:   waBinary.Attrs{"from": receiptTestPeerJID, "id": "MSG1", "t": "1700000000"},
		Content: []waBinary.Node{{Tag: "qualquer"}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(receipt.MessageIDs) != 1 || receipt.MessageIDs[0] != "MSG1" {
		t.Errorf("MessageIDs = %v, esperado [MSG1]", receipt.MessageIDs)
	}
}

func TestParseReceiptRequiresID(t *testing.T) {
	cli := notifTestClient()
	_, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestPeerJID, "t": "1700000000"},
	})
	if err == nil {
		t.Fatal("esperado erro por falta do atributo id")
	}
}

func TestParseReceiptWithoutSessionFails(t *testing.T) {
	cli := &Client{Log: waLog.Noop, Store: &store.Device{}}
	_, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestPeerJID, "id": "MSG1", "t": "1700000000"},
	})
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, esperado ErrNotLoggedIn", err)
	}
}

// Recibo de grupo sem `participant`: o parser nao devolve evento nenhum, ele
// dispara um evento por usuario dentro de <participants> e retorna (nil, nil).
func TestParseReceiptGroupedDispatchesPerUser(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	ts := time.Unix(1700000123, 0)
	receipt, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestGroupJID, "id": "IGNORADO", "t": "1700000000"},
		Content: []waBinary.Node{{
			Tag:   "participants",
			Attrs: waBinary.Attrs{"key": "MSGGRP"},
			Content: []waBinary.Node{
				{Tag: "user", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "t": unixAttr(ts)}},
				{Tag: "user", Attrs: waBinary.Attrs{"jid": receiptTestOwnJID, "t": unixAttr(ts)}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if receipt != nil {
		t.Fatalf("esperado recibo nil no caminho agrupado, veio %+v", receipt)
	}
	if len(*captured) != 2 {
		t.Fatalf("eventos = %d, esperado 2", len(*captured))
	}
	for i, evt := range *captured {
		r, ok := evt.(*events.Receipt)
		if !ok {
			t.Fatalf("evento %d = %T, esperado *events.Receipt", i, evt)
		}
		if len(r.MessageIDs) != 1 || r.MessageIDs[0] != "MSGGRP" {
			t.Errorf("evento %d MessageIDs = %v, esperado [MSGGRP] (a `key` do <participants>)", i, r.MessageIDs)
		}
		if !r.Timestamp.Equal(ts) {
			t.Errorf("evento %d Timestamp = %v, esperado %v", i, r.Timestamp, ts)
		}
		if !r.IsGroup {
			t.Errorf("evento %d deveria ser de grupo", i)
		}
	}
	if (*captured)[0].(*events.Receipt).Sender != receiptTestPeerJID {
		t.Errorf("Sender do primeiro evento = %v", (*captured)[0].(*events.Receipt).Sender)
	}
}

func TestParseReceiptGroupedWithoutParticipants(t *testing.T) {
	cli := notifTestClient()
	_, err := cli.parseReceipt(&waBinary.Node{
		Tag:   "receipt",
		Attrs: waBinary.Attrs{"from": receiptTestGroupJID, "id": "MSG1", "t": "1700000000"},
	})
	var missing *ElementMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, esperado ElementMissingError", err)
	}
	if missing.Tag != "participants" {
		t.Errorf("Tag = %q, esperado participants", missing.Tag)
	}
}

// --- handleGroupedReceipt ---

func TestHandleGroupedReceiptSkipsInvalidChildren(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	cli.handleGroupedReceipt(events.Receipt{}, &waBinary.Node{
		Tag:   "participants",
		Attrs: waBinary.Attrs{"key": "MSG"},
		Content: []waBinary.Node{
			// Tag errada: ignorado.
			{Tag: "device", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "t": "1700000000"}},
			// <user> sem jid: atributo obrigatorio faltando, ignorado.
			{Tag: "user", Attrs: waBinary.Attrs{"t": "1700000000"}},
			// <user> sem t: idem.
			{Tag: "user", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID}},
			{Tag: "user", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "t": "1700000000"}},
		},
	})
	if len(*captured) != 1 {
		t.Fatalf("eventos = %d, esperado 1 (so' o <user> completo)", len(*captured))
	}
}

// --- buildBaseReceipt ---

func TestBuildBaseReceiptCopiesOptionalAttrs(t *testing.T) {
	node := &waBinary.Node{Attrs: waBinary.Attrs{
		"from":        receiptTestPeerJID,
		"recipient":   receiptTestOwnJID,
		"participant": receiptTestOwnLID,
		"id":          "DO NO, NAO DO PARAMETRO",
	}}
	attrs := buildBaseReceipt("MSG1", node)
	if attrs["id"] != "MSG1" {
		t.Errorf("id = %v, esperado MSG1 (o parametro, nao o do no)", attrs["id"])
	}
	if attrs["to"] != receiptTestPeerJID {
		t.Errorf("to = %v, esperado o `from` do no", attrs["to"])
	}
	if attrs["recipient"] != receiptTestOwnJID || attrs["participant"] != receiptTestOwnLID {
		t.Errorf("recipient/participant nao repassados: %v", attrs)
	}
}

func TestBuildBaseReceiptOmitsAbsentOptionalAttrs(t *testing.T) {
	attrs := buildBaseReceipt("MSG1", &waBinary.Node{Attrs: waBinary.Attrs{"from": receiptTestPeerJID}})
	if _, ok := attrs["recipient"]; ok {
		t.Error("recipient nao deveria existir quando o no nao tem")
	}
	if _, ok := attrs["participant"]; ok {
		t.Error("participant nao deveria existir quando o no nao tem")
	}
}

// --- SetForceActiveDeliveryReceipts ---

func TestSetForceActiveDeliveryReceipts(t *testing.T) {
	cli := notifTestClient()
	cli.SetForceActiveDeliveryReceipts(true)
	if got := cli.sendActiveReceipts.Load(); got != activeDeliveryReceiptsForced {
		t.Errorf("sendActiveReceipts = %d, esperado %d", got, activeDeliveryReceiptsForced)
	}
	cli.SetForceActiveDeliveryReceipts(false)
	if got := cli.sendActiveReceipts.Load(); got != activeDeliveryReceiptsOff {
		t.Errorf("sendActiveReceipts = %d, esperado %d", got, activeDeliveryReceiptsOff)
	}
	// Contrato documentado: chamar com receptor nil e' no-op, nao panic.
	var nilCli *Client
	nilCli.SetForceActiveDeliveryReceipts(true)
}
