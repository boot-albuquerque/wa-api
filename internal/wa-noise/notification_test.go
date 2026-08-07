// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// --- parseNewsletterMessages ---

func TestParseNewsletterMessages(t *testing.T) {
	cli := notifTestClient()
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
	msgs := cli.parseNewsletterMessages(&node)
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
	cli := notifTestClient()
	msgs := cli.parseNewsletterMessages(&waBinary.Node{Content: []waBinary.Node{{
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
	cli := notifTestClient()
	msgs := cli.parseNewsletterMessages(&waBinary.Node{})
	if msgs == nil {
		t.Error("lista vazia deveria ser slice nao-nil")
	}
	if len(msgs) != 0 {
		t.Errorf("mensagens = %d, esperado 0", len(msgs))
	}
}

func TestHandleNewsletterNotification(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	newsletterJID := types.NewJID("555", types.NewsletterServer)
	cli.handleNewsletterNotification(context.Background(), &waBinary.Node{
		Tag:   "notification",
		Attrs: waBinary.Attrs{"from": newsletterJID, "t": "1700000000"},
		Content: []waBinary.Node{{Tag: "live_updates", Content: []waBinary.Node{
			{Tag: "message", Attrs: waBinary.Attrs{"server_id": "1", "id": "MSG1", "type": "text", "t": "1"}},
		}}},
	})
	if len(*captured) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(*captured))
	}
	evt, ok := (*captured)[0].(*events.NewsletterLiveUpdate)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.NewsletterLiveUpdate", (*captured)[0])
	}
	if evt.JID != newsletterJID || len(evt.Messages) != 1 {
		t.Errorf("evento mal montado: %+v", evt)
	}
}

// --- handleMexNotification ---

func TestHandleMexNotificationRouting(t *testing.T) {
	for name, tc := range map[string]struct {
		json string
		want any
	}{
		"join":  {`{"data":{"xwa2_notify_newsletter_on_join":{}}}`, &events.NewsletterJoin{}},
		"leave": {`{"data":{"xwa2_notify_newsletter_on_leave":{}}}`, &events.NewsletterLeave{}},
		"mute":  {`{"data":{"xwa2_notify_newsletter_on_mute_change":{}}}`, &events.NewsletterMuteChange{}},
	} {
		t.Run(name, func(t *testing.T) {
			cli := notifTestClient()
			captured := captureEvents(cli)
			cli.handleMexNotification(context.Background(), &waBinary.Node{
				Content: []waBinary.Node{{Tag: "update", Content: []byte(tc.json)}},
			})
			if len(*captured) != 1 {
				t.Fatalf("eventos = %d, esperado 1", len(*captured))
			}
			if got, want := fmt.Sprintf("%T", (*captured)[0]), fmt.Sprintf("%T", tc.want); got != want {
				t.Errorf("evento = %s, esperado %s", got, want)
			}
		})
	}
}

func TestHandleMexNotificationIgnoresBadUpdates(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	cli.handleMexNotification(context.Background(), &waBinary.Node{Content: []waBinary.Node{
		// Tag errada.
		{Tag: "outro", Content: []byte(`{"data":{"xwa2_notify_newsletter_on_join":{}}}`)},
		// Conteudo nao-binario.
		{Tag: "update", Content: "texto"},
		// JSON invalido: loga e segue.
		{Tag: "update", Content: []byte(`{`)},
		// JSON valido mas sem nenhum dos tres eventos conhecidos.
		{Tag: "update", Content: []byte(`{"data":{"xwa2_notify_newsletter_on_state_change":{}}}`)},
	}})
	if len(*captured) != 0 {
		t.Errorf("eventos = %d, esperado 0", len(*captured))
	}
}

// --- handleBlocklist ---

func TestHandleBlocklist(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	cli.handleBlocklist(context.Background(), &waBinary.Node{
		Attrs: waBinary.Attrs{"action": "modify", "dhash": "hash-novo", "prev_dhash": "hash-velho"},
		Content: []waBinary.Node{
			{Tag: "item", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "action": "block"}},
			// Sem `action`: atributo obrigatorio faltando, filho descartado.
			{Tag: "item", Attrs: waBinary.Attrs{"jid": receiptTestOwnJID}},
		},
	})
	if len(*captured) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(*captured))
	}
	evt := (*captured)[0].(*events.Blocklist)
	if evt.DHash != "hash-novo" || evt.PrevDHash != "hash-velho" {
		t.Errorf("hashes = %q/%q", evt.DHash, evt.PrevDHash)
	}
	if len(evt.Changes) != 1 {
		t.Fatalf("mudancas = %d, esperado 1 (a incompleta e' descartada)", len(evt.Changes))
	}
	if evt.Changes[0].JID != receiptTestPeerJID {
		t.Errorf("JID = %v", evt.Changes[0].JID)
	}
}

// --- handlePictureNotification ---

func TestHandlePictureNotification(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	cli.handlePictureNotification(context.Background(), &waBinary.Node{
		Attrs: waBinary.Attrs{"t": "1700000000"},
		Content: []waBinary.Node{
			{Tag: "add", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "id": "PIC1"}},
			{Tag: "set", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "id": "PIC2", "author": receiptTestOwnJID}},
			{Tag: "delete", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID}},
			// Tag desconhecida: nao gera evento.
			{Tag: "rename", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID}},
			// Sem jid: atributo obrigatorio faltando, descartado.
			{Tag: "add", Attrs: waBinary.Attrs{"id": "PIC3"}},
		},
	})
	if len(*captured) != 3 {
		t.Fatalf("eventos = %d, esperado 3", len(*captured))
	}
	add := (*captured)[0].(*events.Picture)
	if add.PictureID != "PIC1" || add.Remove {
		t.Errorf("evento de add = %+v", add)
	}
	if !add.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Timestamp = %v", add.Timestamp)
	}
	set := (*captured)[1].(*events.Picture)
	if set.PictureID != "PIC2" || set.Author != receiptTestOwnJID {
		t.Errorf("evento de set = %+v", set)
	}
	del := (*captured)[2].(*events.Picture)
	if !del.Remove || del.PictureID != "" {
		t.Errorf("evento de delete = %+v", del)
	}
}

// --- handleStatusNotification ---

func TestHandleStatusNotification(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	cli.handleStatusNotification(context.Background(), &waBinary.Node{
		Attrs:   waBinary.Attrs{"from": receiptTestPeerJID, "t": "1700000000"},
		Content: []waBinary.Node{{Tag: "set", Content: []byte("na praia")}},
	})
	if len(*captured) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(*captured))
	}
	evt := (*captured)[0].(*events.UserAbout)
	if evt.Status != "na praia" || evt.JID != receiptTestPeerJID {
		t.Errorf("evento = %+v", evt)
	}
}

func TestHandleStatusNotificationMalformed(t *testing.T) {
	for name, node := range map[string]waBinary.Node{
		"sem filho set": {
			Attrs:   waBinary.Attrs{"from": receiptTestPeerJID, "t": "1"},
			Content: []waBinary.Node{{Tag: "outro", Content: []byte("x")}},
		},
		"conteudo nao binario": {
			Attrs:   waBinary.Attrs{"from": receiptTestPeerJID, "t": "1"},
			Content: []waBinary.Node{{Tag: "set", Content: "texto"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			cli := notifTestClient()
			captured := captureEvents(cli)
			node := node
			cli.handleStatusNotification(context.Background(), &node)
			if len(*captured) != 0 {
				t.Errorf("eventos = %d, esperado 0", len(*captured))
			}
		})
	}
}

// --- handleOwnDevicesNotification ---

func ownDevicesTestClient() *Client {
	cli := notifTestClient()
	cli.userDevicesCache = make(map[types.JID]deviceCache)
	return cli
}

func ownDeviceNode(jid types.JID, dhash string, devices ...types.JID) waBinary.Node {
	children := make([]waBinary.Node, 0, len(devices))
	for _, dev := range devices {
		children = append(children, waBinary.Node{Tag: "device", Attrs: waBinary.Attrs{"jid": dev}})
	}
	return waBinary.Node{
		Tag:     "devices",
		Attrs:   waBinary.Attrs{"from": jid, "dhash": dhash},
		Content: children,
	}
}

func TestHandleOwnDevicesNotificationStoresBothIdentities(t *testing.T) {
	cli := ownDevicesTestClient()
	dev1 := receiptTestOwnJID
	dev1.Device = 1
	devices := []types.JID{receiptTestOwnJID, dev1}
	hash := participantListHashV2(devices)

	node := ownDeviceNode(receiptTestOwnJID, hash, devices...)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestOwnJID)

	cached, ok := cli.userDevicesCache[receiptTestOwnJID]
	if !ok {
		t.Fatal("cache do PN nao foi preenchido")
	}
	if len(cached.devices) != 2 || cached.dhash != hash {
		t.Errorf("cache do PN = %+v", cached)
	}
	// O LID equivalente e' derivado trocando o usuario e mantendo o device.
	altCached, ok := cli.userDevicesCache[receiptTestOwnLID]
	if !ok {
		t.Fatal("cache do LID nao foi preenchido")
	}
	if len(altCached.devices) != 2 {
		t.Fatalf("cache do LID = %+v", altCached)
	}
	if altCached.devices[1].User != receiptTestOwnLID.User || altCached.devices[1].Device != 1 {
		t.Errorf("device alternativo = %v, esperado usuario do LID com device 1", altCached.devices[1])
	}
}

// dhash divergente do que calculamos: os dois caches sao invalidados, para que
// a proxima consulta va' buscar a lista de verdade no servidor.
func TestHandleOwnDevicesNotificationHashMismatchDropsCache(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.userDevicesCache[receiptTestOwnJID] = deviceCache{devices: []types.JID{receiptTestOwnJID}}
	cli.userDevicesCache[receiptTestOwnLID] = deviceCache{devices: []types.JID{receiptTestOwnLID}}

	node := ownDeviceNode(receiptTestOwnJID, "hash-que-nao-bate", receiptTestOwnJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestOwnJID)

	if len(cli.userDevicesCache) != 0 {
		t.Errorf("cache deveria ter sido esvaziado, sobrou %v", cli.userDevicesCache)
	}
}

func TestHandleOwnDevicesNotificationUnexpectedSender(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.userDevicesCache[receiptTestOwnJID] = deviceCache{devices: []types.JID{receiptTestOwnJID}}
	node := ownDeviceNode(receiptTestPeerJID, "x", receiptTestPeerJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestPeerJID)
	if len(cli.userDevicesCache) != 1 {
		t.Errorf("notificacao de outro usuario nao deveria mexer no cache: %v", cli.userDevicesCache)
	}
}

func TestHandleOwnDevicesNotificationWithoutSession(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.Store.ID = nil
	node := ownDeviceNode(receiptTestOwnJID, "x", receiptTestOwnJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestOwnJID)
	if len(cli.userDevicesCache) != 0 {
		t.Errorf("sem sessao nada deveria ser cacheado: %v", cli.userDevicesCache)
	}
}

// --- handleDeviceNotification ---

func deviceChangeNode(from types.JID, tag string, deviceHash string, device types.JID) waBinary.Node {
	return waBinary.Node{
		Tag:   "notification",
		Attrs: waBinary.Attrs{"from": from},
		Content: []waBinary.Node{{
			Tag:     tag,
			Attrs:   waBinary.Attrs{"device_hash": deviceHash},
			Content: []waBinary.Node{{Tag: "device", Attrs: waBinary.Attrs{"jid": device}}},
		}},
	}
}

func TestHandleDeviceNotificationAddWithMatchingHash(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.userDevicesCache[receiptTestPeerJID] = deviceCache{devices: []types.JID{receiptTestPeerJID}}
	newDevice := receiptTestPeerJID
	newDevice.Device = 3
	hash := participantListHashV2([]types.JID{receiptTestPeerJID, newDevice})

	node := deviceChangeNode(receiptTestPeerJID, "add", hash, newDevice)
	cli.handleDeviceNotification(context.Background(), &node)

	cached := cli.userDevicesCache[receiptTestPeerJID]
	if len(cached.devices) != 2 || cached.devices[1] != newDevice {
		t.Errorf("cache = %+v, esperado o device novo anexado", cached)
	}
}

func TestHandleDeviceNotificationRemoveWithMatchingHash(t *testing.T) {
	cli := ownDevicesTestClient()
	extra := receiptTestPeerJID
	extra.Device = 3
	cli.userDevicesCache[receiptTestPeerJID] = deviceCache{devices: []types.JID{receiptTestPeerJID, extra}}
	hash := participantListHashV2([]types.JID{receiptTestPeerJID})

	node := deviceChangeNode(receiptTestPeerJID, "remove", hash, extra)
	cli.handleDeviceNotification(context.Background(), &node)

	cached := cli.userDevicesCache[receiptTestPeerJID]
	if len(cached.devices) != 1 || cached.devices[0] != receiptTestPeerJID {
		t.Errorf("cache = %+v, esperado so' o device principal", cached)
	}
}

// Hash divergente significa que nossa reconstrucao da lista errou: o cache tem
// que sumir, e nao ficar com um estado que achamos certo e o servidor nao.
func TestHandleDeviceNotificationHashMismatchDropsCache(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.userDevicesCache[receiptTestPeerJID] = deviceCache{devices: []types.JID{receiptTestPeerJID}}
	newDevice := receiptTestPeerJID
	newDevice.Device = 3

	node := deviceChangeNode(receiptTestPeerJID, "add", "hash-errado", newDevice)
	cli.handleDeviceNotification(context.Background(), &node)

	if _, ok := cli.userDevicesCache[receiptTestPeerJID]; ok {
		t.Error("cache deveria ter sido descartado")
	}
}

func TestHandleDeviceNotificationUpdateAndUnknownTags(t *testing.T) {
	for name, tag := range map[string]string{
		"update":       "update",
		"desconhecida": "renomeia",
	} {
		t.Run(name, func(t *testing.T) {
			cli := ownDevicesTestClient()
			cli.userDevicesCache[receiptTestPeerJID] = deviceCache{devices: []types.JID{receiptTestPeerJID}}
			node := deviceChangeNode(receiptTestPeerJID, tag, "qualquer", receiptTestPeerJID)
			cli.handleDeviceNotification(context.Background(), &node)
			_, stillCached := cli.userDevicesCache[receiptTestPeerJID]
			// "update" derruba o cache por precaucao; tag desconhecida e' ignorada.
			if tag == "update" && stillCached {
				t.Error("update deveria derrubar o cache")
			}
			if tag != "update" && !stillCached {
				t.Error("tag desconhecida nao deveria mexer no cache")
			}
		})
	}
}

// Sem nada em cache nao ha' o que reconciliar: a notificacao e' descartada sem
// criar entrada nova (senao teriamos uma lista de devices parcial).
func TestHandleDeviceNotificationWithoutCachedListIsIgnored(t *testing.T) {
	cli := ownDevicesTestClient()
	node := deviceChangeNode(receiptTestPeerJID, "add", "x", receiptTestPeerJID)
	cli.handleDeviceNotification(context.Background(), &node)
	if len(cli.userDevicesCache) != 0 {
		t.Errorf("cache = %v, esperado vazio", cli.userDevicesCache)
	}
}

// --- handleNotification (dispatcher) ---

// O dispatcher e' o unico ponto que le' o atributo `type`: sem ele, nada roda
// (nem o ack). Com tipo desconhecido, so' loga.
func TestHandleNotificationRouting(t *testing.T) {
	for name, tc := range map[string]struct {
		node       waBinary.Node
		wantEvents int
	}{
		"sem type": {
			node: waBinary.Node{Tag: "notification", Attrs: waBinary.Attrs{"from": receiptTestPeerJID}},
		},
		"tipo desconhecido": {
			node: waBinary.Node{Tag: "notification", Attrs: waBinary.Attrs{
				"from": receiptTestPeerJID, "type": "psa", "id": "N1",
			}},
		},
		"status": {
			node: waBinary.Node{
				Tag: "notification",
				Attrs: waBinary.Attrs{
					"from": receiptTestPeerJID, "type": notificationTypeStatus, "id": "N1", "t": "1700000000",
				},
				Content: []waBinary.Node{{Tag: "set", Content: []byte("oi")}},
			},
			wantEvents: 1,
		},
		"picture": {
			node: waBinary.Node{
				Tag: "notification",
				Attrs: waBinary.Attrs{
					"from": receiptTestPeerJID, "type": notificationTypePicture, "id": "N1", "t": "1700000000",
				},
				Content: []waBinary.Node{{Tag: "add", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "id": "PIC"}}},
			},
			wantEvents: 1,
		},
		"mex": {
			node: waBinary.Node{
				Tag: "notification",
				Attrs: waBinary.Attrs{
					"from": receiptTestPeerJID, "type": notificationTypeMex, "id": "N1",
				},
				Content: []waBinary.Node{{
					Tag:     "update",
					Content: []byte(`{"data":{"xwa2_notify_newsletter_on_join":{}}}`),
				}},
			},
			wantEvents: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cli := notifTestClient()
			// SynchronousAck evita a goroutine de ack: sem socket, sendAck so'
			// devolve ErrNotConnected e loga, mas de forma deterministica.
			cli.SynchronousAck = true
			captured := captureEvents(cli)
			node := tc.node
			cli.handleNotification(context.Background(), &node)
			if len(*captured) != tc.wantEvents {
				t.Errorf("eventos = %d, esperado %d", len(*captured), tc.wantEvents)
			}
		})
	}
}
