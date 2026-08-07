// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package newsletter

import (
	"context"
	"errors"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// Sem params, o no leva so' a identificacao do canal — nenhum atributo de
// paginacao. Mandar count=0 faria o servidor devolver zero mensagens.
func TestMessagesAttrsSemParams(t *testing.T) {
	jid := testJID()
	attrs := messagesAttrs(jid, nil)

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

func TestMessagesAttrsCamposZeradosSaoOmitidos(t *testing.T) {
	attrs := messagesAttrs(testJID(), &GetMessagesParams{})
	if len(attrs) != 2 {
		t.Fatalf("params zerado deveria render so' type+jid, veio %v", attrs)
	}
}

func TestMessagesAttrsComPaginacao(t *testing.T) {
	attrs := messagesAttrs(testJID(), &GetMessagesParams{
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

func TestMessageUpdatesAttrsSemParams(t *testing.T) {
	attrs := messageUpdatesAttrs(nil)
	if len(attrs) != 0 {
		t.Fatalf("sem params o no de updates nao leva atributos, veio %v", attrs)
	}
}

func TestMessageUpdatesAttrsCamposZeradosSaoOmitidos(t *testing.T) {
	attrs := messageUpdatesAttrs(&GetUpdatesParams{})
	if len(attrs) != 0 {
		t.Fatalf("params zerado nao deveria gerar atributo, veio %v", attrs)
	}
}

// Since vai para o wire como epoch em SEGUNDOS. Mandar em milissegundos (ou o
// time.Time cru) faria o servidor devolver a janela errada de updates.
func TestMessageUpdatesAttrsSinceVaiEmSegundos(t *testing.T) {
	since := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	attrs := messageUpdatesAttrs(&GetUpdatesParams{
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

func TestMessageUpdatesAttrsSinceZeradoEOmitido(t *testing.T) {
	attrs := messageUpdatesAttrs(&GetUpdatesParams{Count: 5})
	if _, ok := attrs["since"]; ok {
		t.Error("time.Time zerado nao deveria virar since=-6795364578871")
	}
}

// messagesNode monta uma resposta de <iq> com um <messages> dentro.
func messagesNode() *waBinary.Node {
	return &waBinary.Node{
		Tag:     "iq",
		Content: []waBinary.Node{{Tag: messagesTag}},
	}
}

// O <iq> de mensagens vai para o SERVIDOR (nao para o canal) e o parsing e'
// delegado ao Transport.
func TestGetMessagesEnviaParaOServidorEDelegaOParsing(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = messagesNode()
	f.parsed = []*types.NewsletterMessage{{MessageServerID: 1}, {MessageServerID: 2}}

	msgs, err := GetMessages(context.Background(), f, testJID(), &GetMessagesParams{Count: 10})
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("esperava 2 mensagens, veio %d", len(msgs))
	}
	iq := f.iqs[0]
	if iq.Namespace != Namespace || iq.Type != IQGet {
		t.Errorf("iq = %+v", iq)
	}
	if iq.To != types.ServerJID {
		t.Errorf("to = %v, esperava %v (o IQ de mensagens vai para o servidor)", iq.To, types.ServerJID)
	}
	nodes := iq.Content.([]waBinary.Node)
	if nodes[0].Tag != messagesTag || nodes[0].Attrs["count"] != 10 {
		t.Errorf("no = %+v", nodes[0])
	}
	if f.parseCnt != 1 {
		t.Errorf("ParseMessages chamado %d vezes, esperava 1", f.parseCnt)
	}
}

func TestGetMessagesPropagaErroDoIQ(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	msgs, err := GetMessages(context.Background(), f, testJID(), nil)
	if msgs != nil {
		t.Errorf("msgs = %v, esperava nil", msgs)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestGetMessagesElementoAusente(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	_, err := GetMessages(context.Background(), f, testJID(), nil)
	var eme *elementMissingError
	if !errors.As(err, &eme) {
		t.Fatalf("err = %v (%T), esperava elementMissingError", err, err)
	}
	if eme.Tag != messagesTag || eme.In != messagesErrContext {
		t.Errorf("erro = %+v", eme)
	}
	if f.parseCnt != 0 {
		t.Error("ParseMessages nao deveria ter sido chamado")
	}
}

// O <iq> de updates vai para o JID DO CANAL (assimetria herdada do upstream) e
// o <messages> vem aninhado dentro de <message_updates>.
func TestGetMessageUpdatesEnviaParaOCanalELeAninhado(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     messageUpdatesTag,
			Content: []waBinary.Node{{Tag: messagesTag}},
		}},
	}
	f.parsed = []*types.NewsletterMessage{{MessageServerID: 9}}

	jid := testJID()
	msgs, err := GetMessageUpdates(context.Background(), f, jid, &GetUpdatesParams{Count: 3})
	if err != nil {
		t.Fatalf("GetMessageUpdates: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 mensagem, veio %d", len(msgs))
	}
	iq := f.iqs[0]
	if iq.To != jid {
		t.Errorf("to = %v, esperava %v (o IQ de updates vai para o canal)", iq.To, jid)
	}
	nodes := iq.Content.([]waBinary.Node)
	if nodes[0].Tag != messageUpdatesTag || nodes[0].Attrs["count"] != 3 {
		t.Errorf("no = %+v", nodes[0])
	}
}

func TestGetMessageUpdatesPropagaErroDoIQ(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if _, err := GetMessageUpdates(context.Background(), f, testJID(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// Sem o <messages> interno, o erro reporta a tag "messages" — nao
// "message_updates". Assimetria herdada do upstream, travada aqui.
func TestGetMessageUpdatesElementoAusenteReportaMessages(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{
		Tag:     "iq",
		Content: []waBinary.Node{{Tag: messageUpdatesTag}},
	}

	_, err := GetMessageUpdates(context.Background(), f, testJID(), nil)
	var eme *elementMissingError
	if !errors.As(err, &eme) {
		t.Fatalf("err = %v (%T), esperava elementMissingError", err, err)
	}
	if eme.Tag != messagesTag {
		t.Errorf("tag = %q, esperava %q", eme.Tag, messagesTag)
	}
}
