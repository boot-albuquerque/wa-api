// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"
	"time"

	"wa-api/internal/wa-noise/types"
)

func newsletterTestJID() types.JID {
	return types.NewJID("1234567890", types.NewsletterServer)
}

// Sem params, o no leva so' a identificacao do canal — nenhum atributo de
// paginacao. Mandar count=0 faria o servidor devolver zero mensagens.
func TestNewsletterMessagesAttrsSemParams(t *testing.T) {
	jid := newsletterTestJID()
	attrs := newsletterMessagesAttrs(jid, nil)

	if attrs["type"] != "jid" {
		t.Errorf("type = %v, esperava \"jid\"", attrs["type"])
	}
	if attrs["jid"] != jid {
		t.Errorf("jid = %v, esperava %v", attrs["jid"], jid)
	}
	if _, ok := attrs["count"]; ok {
		t.Error("sem params nao deveria haver count")
	}
	if _, ok := attrs["before"]; ok {
		t.Error("sem params nao deveria haver before")
	}
}

func TestNewsletterMessagesAttrsCamposZeradosSaoOmitidos(t *testing.T) {
	attrs := newsletterMessagesAttrs(newsletterTestJID(), &GetNewsletterMessagesParams{})
	if len(attrs) != 2 {
		t.Fatalf("params zerado deveria render so' type+jid, veio %v", attrs)
	}
}

func TestNewsletterMessagesAttrsComPaginacao(t *testing.T) {
	attrs := newsletterMessagesAttrs(newsletterTestJID(), &GetNewsletterMessagesParams{
		Count:  50,
		Before: 987,
	})
	if attrs["count"] != 50 {
		t.Errorf("count = %v, esperava 50", attrs["count"])
	}
	if attrs["before"] != types.MessageServerID(987) {
		t.Errorf("before = %v, esperava 987", attrs["before"])
	}
}

func TestNewsletterMessageUpdatesAttrsSemParams(t *testing.T) {
	attrs := newsletterMessageUpdatesAttrs(nil)
	if len(attrs) != 0 {
		t.Fatalf("sem params o no de updates nao leva atributos, veio %v", attrs)
	}
}

func TestNewsletterMessageUpdatesAttrsCamposZeradosSaoOmitidos(t *testing.T) {
	attrs := newsletterMessageUpdatesAttrs(&GetNewsletterUpdatesParams{})
	if len(attrs) != 0 {
		t.Fatalf("params zerado nao deveria gerar atributo, veio %v", attrs)
	}
}

// Since vai para o wire como epoch em SEGUNDOS. Mandar em milissegundos (ou o
// time.Time cru) faria o servidor devolver a janela errada de updates.
func TestNewsletterMessageUpdatesAttrsSinceVaiEmSegundos(t *testing.T) {
	since := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	attrs := newsletterMessageUpdatesAttrs(&GetNewsletterUpdatesParams{
		Count: 20,
		Since: since,
		After: 555,
	})
	if attrs["count"] != 20 {
		t.Errorf("count = %v, esperava 20", attrs["count"])
	}
	if attrs["since"] != since.Unix() {
		t.Errorf("since = %v, esperava %d", attrs["since"], since.Unix())
	}
	if attrs["after"] != types.MessageServerID(555) {
		t.Errorf("after = %v, esperava 555", attrs["after"])
	}
}

func TestNewsletterMessageUpdatesAttrsSinceZeradoEOmitido(t *testing.T) {
	attrs := newsletterMessageUpdatesAttrs(&GetNewsletterUpdatesParams{Count: 5})
	if _, ok := attrs["since"]; ok {
		t.Error("time.Time zerado nao deveria virar since=-6795364578871")
	}
}
