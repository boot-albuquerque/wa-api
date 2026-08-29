package core

import (
	"context"
	"testing"

	"wa-api/internal/noise/capabilities/notification"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// --- fachadas para internal/wa-noise/notification ---

// A logica de blocklist/picture/status/newsletter/mex vive no subpacote
// (Fase F/G, lote 5) e e' testada la'. O que a raiz precisa travar e' a
// delegacao: que o adaptador entregue o evento ao dispatchEvent do cliente e
// que um receptor nil nao estoure.
func TestFachadaDeNotificacaoDelegaAoSubpacote(t *testing.T) {
	cli := notifTestClient()
	captured := captureEvents(cli)
	ctx := context.Background()
	cli.handleBlocklist(ctx, &waBinary.Node{Attrs: waBinary.Attrs{"dhash": "h"}})
	cli.handlePictureNotification(ctx, &waBinary.Node{
		Attrs:   waBinary.Attrs{"t": "1"},
		Content: []waBinary.Node{{Tag: "add", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "id": "P"}}},
	})
	cli.handleStatusNotification(ctx, &waBinary.Node{
		Attrs:   waBinary.Attrs{"from": receiptTestPeerJID, "t": "1"},
		Content: []waBinary.Node{{Tag: "set", Content: []byte("oi")}},
	})
	cli.handleNewsletterNotification(ctx, &waBinary.Node{
		Attrs: waBinary.Attrs{"from": receiptTestPeerJID, "t": "1"},
	})
	cli.handleMexNotification(ctx, &waBinary.Node{Content: []waBinary.Node{
		{Tag: "update", Content: []byte(`{"data":{"xwa2_notify_newsletter_on_join":{}}}`)},
	}})
	if len(*captured) != 5 {
		t.Fatalf("eventos = %d, esperado 5 (um por fachada)", len(*captured))
	}
	if msgs := cli.parseNewsletterMessages(&waBinary.Node{}); msgs == nil || len(msgs) != 0 {
		t.Errorf("parseNewsletterMessages = %v, esperado slice vazio nao-nil", msgs)
	}
}

func TestFachadaDeNotificacaoRecusaClientNil(t *testing.T) {
	var cli *Client
	ctx := context.Background()
	cli.handleBlocklist(ctx, &waBinary.Node{})
	cli.handlePictureNotification(ctx, &waBinary.Node{})
	cli.handleStatusNotification(ctx, &waBinary.Node{})
	cli.handleNewsletterNotification(ctx, &waBinary.Node{})
	cli.handleMexNotification(ctx, &waBinary.Node{})
	if msgs := cli.parseNewsletterMessages(&waBinary.Node{}); msgs != nil {
		t.Errorf("parseNewsletterMessages = %v, esperado nil", msgs)
	}
}

// --- handleOwnDevicesNotification ---

func ownDevicesTestClient() *Client {
	return notifTestClient()
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

	cached, ok := getDeviceCache(cli, receiptTestOwnJID)
	if !ok {
		t.Fatal("cache do PN nao foi preenchido")
	}
	if len(cached.Devices) != 2 || cached.DHash != hash {
		t.Errorf("cache do PN = %+v", cached)
	}
	// O LID equivalente e' derivado trocando o usuario e mantendo o device.
	altCached, ok := getDeviceCache(cli, receiptTestOwnLID)
	if !ok {
		t.Fatal("cache do LID nao foi preenchido")
	}
	if len(altCached.Devices) != 2 {
		t.Fatalf("cache do LID = %+v", altCached)
	}
	if altCached.Devices[1].User != receiptTestOwnLID.User || altCached.Devices[1].Device != 1 {
		t.Errorf("device alternativo = %v, esperado usuario do LID com device 1", altCached.Devices[1])
	}
}

// dhash divergente do que calculamos: os dois caches sao invalidados, para que
// a proxima consulta va' buscar a lista de verdade no servidor.
func TestHandleOwnDevicesNotificationHashMismatchDropsCache(t *testing.T) {
	cli := ownDevicesTestClient()
	putDeviceCache(cli, receiptTestOwnJID, deviceCache{Devices: []types.JID{receiptTestOwnJID}})
	putDeviceCache(cli, receiptTestOwnLID, deviceCache{Devices: []types.JID{receiptTestOwnLID}})

	node := ownDeviceNode(receiptTestOwnJID, "hash-que-nao-bate", receiptTestOwnJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestOwnJID)

	if cli.userDevicesCache.Len() != 0 {
		t.Errorf("cache deveria ter sido esvaziado, sobrou %v", cli.userDevicesCache.Len())
	}
}

func TestHandleOwnDevicesNotificationUnexpectedSender(t *testing.T) {
	cli := ownDevicesTestClient()
	putDeviceCache(cli, receiptTestOwnJID, deviceCache{Devices: []types.JID{receiptTestOwnJID}})
	node := ownDeviceNode(receiptTestPeerJID, "x", receiptTestPeerJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestPeerJID)
	if cli.userDevicesCache.Len() != 1 {
		t.Errorf("notificacao de outro usuario nao deveria mexer no cache: %v", cli.userDevicesCache.Len())
	}
}

func TestHandleOwnDevicesNotificationWithoutSession(t *testing.T) {
	cli := ownDevicesTestClient()
	cli.Store.ID = nil
	node := ownDeviceNode(receiptTestOwnJID, "x", receiptTestOwnJID)
	cli.handleOwnDevicesNotification(context.Background(), &node, receiptTestOwnJID)
	if cli.userDevicesCache.Len() != 0 {
		t.Errorf("sem sessao nada deveria ser cacheado: %v", cli.userDevicesCache.Len())
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
	putDeviceCache(cli, receiptTestPeerJID, deviceCache{Devices: []types.JID{receiptTestPeerJID}})
	newDevice := receiptTestPeerJID
	newDevice.Device = 3
	hash := participantListHashV2([]types.JID{receiptTestPeerJID, newDevice})

	node := deviceChangeNode(receiptTestPeerJID, "add", hash, newDevice)
	cli.handleDeviceNotification(context.Background(), &node)

	cached, _ := getDeviceCache(cli, receiptTestPeerJID)
	if len(cached.Devices) != 2 || cached.Devices[1] != newDevice {
		t.Errorf("cache = %+v, esperado o device novo anexado", cached)
	}
}

func TestHandleDeviceNotificationRemoveWithMatchingHash(t *testing.T) {
	cli := ownDevicesTestClient()
	extra := receiptTestPeerJID
	extra.Device = 3
	putDeviceCache(cli, receiptTestPeerJID, deviceCache{Devices: []types.JID{receiptTestPeerJID, extra}})
	hash := participantListHashV2([]types.JID{receiptTestPeerJID})

	node := deviceChangeNode(receiptTestPeerJID, "remove", hash, extra)
	cli.handleDeviceNotification(context.Background(), &node)

	cached, _ := getDeviceCache(cli, receiptTestPeerJID)
	if len(cached.Devices) != 1 || cached.Devices[0] != receiptTestPeerJID {
		t.Errorf("cache = %+v, esperado so' o device principal", cached)
	}
}

// Hash divergente significa que nossa reconstrucao da lista errou: o cache tem
// que sumir, e nao ficar com um estado que achamos certo e o servidor nao.
func TestHandleDeviceNotificationHashMismatchDropsCache(t *testing.T) {
	cli := ownDevicesTestClient()
	putDeviceCache(cli, receiptTestPeerJID, deviceCache{Devices: []types.JID{receiptTestPeerJID}})
	newDevice := receiptTestPeerJID
	newDevice.Device = 3

	node := deviceChangeNode(receiptTestPeerJID, "add", "hash-errado", newDevice)
	cli.handleDeviceNotification(context.Background(), &node)

	if _, ok := getDeviceCache(cli, receiptTestPeerJID); ok {
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
			putDeviceCache(cli, receiptTestPeerJID, deviceCache{Devices: []types.JID{receiptTestPeerJID}})
			node := deviceChangeNode(receiptTestPeerJID, tag, "qualquer", receiptTestPeerJID)
			cli.handleDeviceNotification(context.Background(), &node)
			_, stillCached := getDeviceCache(cli, receiptTestPeerJID)
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
	if cli.userDevicesCache.Len() != 0 {
		t.Errorf("cache = %v, esperado vazio", cli.userDevicesCache.Len())
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
					"from": receiptTestPeerJID, "type": notification.TypeStatus, "id": "N1", "t": "1700000000",
				},
				Content: []waBinary.Node{{Tag: "set", Content: []byte("oi")}},
			},
			wantEvents: 1,
		},
		"picture": {
			node: waBinary.Node{
				Tag: "notification",
				Attrs: waBinary.Attrs{
					"from": receiptTestPeerJID, "type": notification.TypePicture, "id": "N1", "t": "1700000000",
				},
				Content: []waBinary.Node{{Tag: "add", Attrs: waBinary.Attrs{"jid": receiptTestPeerJID, "id": "PIC"}}},
			},
			wantEvents: 1,
		},
		"mex": {
			node: waBinary.Node{
				Tag: "notification",
				Attrs: waBinary.Attrs{
					"from": receiptTestPeerJID, "type": notification.TypeMex, "id": "N1",
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
