// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	"wa-api/internal/wa-noise/types"
)

func TestNewsletterViewedItemsPreservaOrdemEIDs(t *testing.T) {
	items := newsletterViewedItems([]types.MessageServerID{7, 3, 11})
	if len(items) != 3 {
		t.Fatalf("esperava 3 itens, veio %d", len(items))
	}
	for i, want := range []types.MessageServerID{7, 3, 11} {
		if items[i].Tag != "item" {
			t.Errorf("item %d: tag = %q, esperava \"item\"", i, items[i].Tag)
		}
		got, ok := items[i].Attrs["server_id"].(types.MessageServerID)
		if !ok {
			t.Fatalf("item %d: server_id de tipo %T, esperava types.MessageServerID", i, items[i].Attrs["server_id"])
		}
		if got != want {
			t.Errorf("item %d: server_id = %d, esperava %d", i, got, want)
		}
	}
}

// Lista vazia continua produzindo um <list> sem filhos, nao nil: o servidor
// aceita o recibo vazio e o codigo nao deve tratar isso como erro.
func TestNewsletterViewedItemsListaVazia(t *testing.T) {
	items := newsletterViewedItems(nil)
	if items == nil {
		t.Fatal("esperava slice vazio nao-nil")
	}
	if len(items) != 0 {
		t.Fatalf("esperava 0 itens, veio %d", len(items))
	}
}

func TestNewsletterReactionAttrsComCodigo(t *testing.T) {
	jid := types.NewJID("1234567890", types.NewsletterServer)
	msgAttrs, reactionAttrs := newsletterReactionAttrs(jid, 42, "\U0001F600", "MSGID1")

	if msgAttrs["to"] != jid {
		t.Errorf("to = %v, esperava %v", msgAttrs["to"], jid)
	}
	if msgAttrs["id"] != types.MessageID("MSGID1") {
		t.Errorf("id = %v", msgAttrs["id"])
	}
	if msgAttrs["server_id"] != types.MessageServerID(42) {
		t.Errorf("server_id = %v", msgAttrs["server_id"])
	}
	if msgAttrs["type"] != "reaction" {
		t.Errorf("type = %v, esperava \"reaction\"", msgAttrs["type"])
	}
	if _, ok := msgAttrs["edit"]; ok {
		t.Error("reacao com codigo nao deveria carregar o atributo edit")
	}
	if reactionAttrs["code"] != "\U0001F600" {
		t.Errorf("code = %v", reactionAttrs["code"])
	}
}

// Reacao vazia significa REMOVER a reacao anterior. O fork nao manda code=""
// — manda uma revogacao do proprio remetente. Trocar isso faria o WhatsApp
// registrar uma reacao com codigo vazio em vez de apagar a existente.
func TestNewsletterReactionAttrsVaziaViraRevogacao(t *testing.T) {
	jid := types.NewJID("1234567890", types.NewsletterServer)
	msgAttrs, reactionAttrs := newsletterReactionAttrs(jid, 42, "", "MSGID2")

	if len(reactionAttrs) != 0 {
		t.Errorf("reacao vazia nao deveria ter atributos, veio %v", reactionAttrs)
	}
	if msgAttrs["edit"] != string(types.EditAttributeSenderRevoke) {
		t.Errorf("edit = %v, esperava %v", msgAttrs["edit"], types.EditAttributeSenderRevoke)
	}
}
