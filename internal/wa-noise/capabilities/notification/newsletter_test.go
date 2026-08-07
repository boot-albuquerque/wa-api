// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

func TestParseNewsletterMessages(t *testing.T) {
	tr := &fakeTransport{}
	plaintext, err := proto.Marshal(&waE2E.Message{Conversation: proto.String("oi")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	node := waBinary.Node{
		Tag: "live_updates",
		Content: []waBinary.Node{
			// Filho que nao e' <message> e' ignorado.
			{Tag: "ruido"},
			{
				Tag: "message",
				Attrs: waBinary.Attrs{
					"server_id": "42",
					"id":        "MSG1",
					"type":      "text",
					"t":         "1700000000",
				},
				Content: []waBinary.Node{
					{Tag: "plaintext", Content: plaintext},
					{Tag: "views_count", Attrs: waBinary.Attrs{"count": "7"}},
					{Tag: "reactions", Content: []waBinary.Node{
						{Tag: "reaction", Attrs: waBinary.Attrs{"code": "\U0001F44D", "count": "3"}},
						{Tag: "reaction", Attrs: waBinary.Attrs{"code": "❤", "count": "1"}},
					}},
					// Subfilho desconhecido nao atrapalha.
					{Tag: "desconhecido"},
				},
			},
		},
	}
	msgs := ParseNewsletterMessages(tr, &node)
	if len(msgs) != 1 {
		t.Fatalf("mensagens = %d, esperado 1", len(msgs))
	}
	msg := msgs[0]
	if msg.MessageServerID != 42 || msg.MessageID != "MSG1" || msg.Type != "text" {
		t.Errorf("atributos mal lidos: %+v", msg)
	}
	if !msg.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Timestamp = %v", msg.Timestamp)
	}
	if msg.Message.GetConversation() != "oi" {
		t.Errorf("Conversation = %q", msg.Message.GetConversation())
	}
	if msg.ViewsCount != 7 {
		t.Errorf("ViewsCount = %d, esperado 7", msg.ViewsCount)
	}
	if msg.ReactionCounts["\U0001F44D"] != 3 || msg.ReactionCounts["❤"] != 1 {
		t.Errorf("ReactionCounts = %v", msg.ReactionCounts)
	}
}

// Protobuf quebrado dentro de <plaintext> nao pode derrubar o parse da lista
// inteira: a mensagem entra com Message nil.
func TestParseNewsletterMessagesInvalidPlaintext(t *testing.T) {
	tr := &fakeTransport{}
	msgs := ParseNewsletterMessages(tr, &waBinary.Node{Content: []waBinary.Node{{
		Tag:   "message",
		Attrs: waBinary.Attrs{"server_id": "1", "id": "MSG1", "type": "text", "t": "1"},
		Content: []waBinary.Node{
			// Conteudo nao-binario e' ignorado sem sequer tentar desserializar.
			{Tag: "plaintext", Content: "texto"},
		},
	}, {
		Tag:     "message",
		Attrs:   waBinary.Attrs{"server_id": "2", "id": "MSG2", "type": "text", "t": "1"},
		Content: []waBinary.Node{{Tag: "plaintext", Content: []byte{0xFF, 0xFF, 0xFF}}},
	}}})
	if len(msgs) != 2 {
		t.Fatalf("mensagens = %d, esperado 2", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Message != nil {
			t.Errorf("mensagem %d deveria ficar com Message nil", i)
		}
	}
}

func TestParseNewsletterMessagesEmptyIsNonNil(t *testing.T) {
	msgs := ParseNewsletterMessages(&fakeTransport{}, &waBinary.Node{})
	if msgs == nil {
		t.Error("lista vazia deveria ser slice nao-nil")
	}
	if len(msgs) != 0 {
		t.Errorf("mensagens = %d, esperado 0", len(msgs))
	}
}

func TestHandleNewsletter(t *testing.T) {
	tr := &fakeTransport{}
	newsletterJID := types.NewJID("555", types.NewsletterServer)
	HandleNewsletter(tr, &waBinary.Node{
		Tag:   "notification",
		Attrs: waBinary.Attrs{"from": newsletterJID, "t": "1700000000"},
		Content: []waBinary.Node{{Tag: "live_updates", Content: []waBinary.Node{
			{Tag: "message", Attrs: waBinary.Attrs{"server_id": "1", "id": "MSG1", "type": "text", "t": "1"}},
		}}},
	})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	evt, ok := tr.events[0].(*events.NewsletterLiveUpdate)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.NewsletterLiveUpdate", tr.events[0])
	}
	if evt.JID != newsletterJID || len(evt.Messages) != 1 {
		t.Errorf("evento mal montado: %+v", evt)
	}
	if !evt.Time.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Time = %v", evt.Time)
	}
}

// Sem <live_updates>, GetChildByTag devolve o no zero e o evento sai com lista
// vazia — nao e' erro. Trava o comportamento do upstream.
func TestHandleNewsletterWithoutLiveUpdates(t *testing.T) {
	tr := &fakeTransport{}
	HandleNewsletter(tr, &waBinary.Node{Tag: "notification", Attrs: waBinary.Attrs{
		"from": types.NewJID("555", types.NewsletterServer), "t": "1",
	}})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	if msgs := tr.events[0].(*events.NewsletterLiveUpdate).Messages; len(msgs) != 0 {
		t.Errorf("mensagens = %d, esperado 0", len(msgs))
	}
}

func TestHandleMexRouting(t *testing.T) {
	for name, tc := range map[string]struct {
		json string
		want any
	}{
		"join":  {`{"data":{"xwa2_notify_newsletter_on_join":{}}}`, &events.NewsletterJoin{}},
		"leave": {`{"data":{"xwa2_notify_newsletter_on_leave":{}}}`, &events.NewsletterLeave{}},
		"mute":  {`{"data":{"xwa2_notify_newsletter_on_mute_change":{}}}`, &events.NewsletterMuteChange{}},
	} {
		t.Run(name, func(t *testing.T) {
			tr := &fakeTransport{}
			HandleMex(tr, &waBinary.Node{
				Content: []waBinary.Node{{Tag: "update", Content: []byte(tc.json)}},
			})
			if len(tr.events) != 1 {
				t.Fatalf("eventos = %d, esperado 1", len(tr.events))
			}
			if got, want := fmt.Sprintf("%T", tr.events[0]), fmt.Sprintf("%T", tc.want); got != want {
				t.Errorf("evento = %s, esperado %s", got, want)
			}
		})
	}
}

// A cadeia else-if do upstream despacha no maximo um evento por <update>,
// mesmo quando o JSON traz mais de um campo. Join tem prioridade sobre leave.
func TestHandleMexDispatchesAtMostOnePerUpdate(t *testing.T) {
	tr := &fakeTransport{}
	HandleMex(tr, &waBinary.Node{Content: []waBinary.Node{{
		Tag: "update",
		Content: []byte(`{"data":{` +
			`"xwa2_notify_newsletter_on_join":{},` +
			`"xwa2_notify_newsletter_on_leave":{},` +
			`"xwa2_notify_newsletter_on_mute_change":{}}}`),
	}}})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	if _, ok := tr.events[0].(*events.NewsletterJoin); !ok {
		t.Errorf("evento = %T, esperado *events.NewsletterJoin (join tem prioridade)", tr.events[0])
	}
}

func TestHandleMexIgnoresBadUpdates(t *testing.T) {
	tr := &fakeTransport{}
	HandleMex(tr, &waBinary.Node{Content: []waBinary.Node{
		// Tag errada.
		{Tag: "outro", Content: []byte(`{"data":{"xwa2_notify_newsletter_on_join":{}}}`)},
		// Conteudo nao-binario.
		{Tag: "update", Content: "texto"},
		// JSON invalido: loga e segue.
		{Tag: "update", Content: []byte(`{`)},
		// JSON valido mas sem nenhum dos tres eventos conhecidos.
		{Tag: "update", Content: []byte(`{"data":{"xwa2_notify_newsletter_on_state_change":{}}}`)},
	}})
	if len(tr.events) != 0 {
		t.Errorf("eventos = %d, esperado 0", len(tr.events))
	}
}
