// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// --- ParseDeviceList (relocado de user_devices_test.go, Fase E lote 7) ---

func TestParseDeviceListReadsAllDevices(t *testing.T) {
	got := ParseDeviceList(userTestPNJID, devicesNode(
		deviceNode("0", false),
		deviceNode("1", false),
		deviceNode("42", false),
	))
	if len(got) != 3 {
		t.Fatalf("expected 3 devices, got %d (%v)", len(got), got)
	}
	for i, wantDevice := range []uint16{0, 1, 42} {
		if got[i].Device != wantDevice {
			t.Errorf("device %d: got id %d, want %d", i, got[i].Device, wantDevice)
		}
		if got[i].User != userTestPNJID.User || got[i].Server != types.DefaultUserServer {
			t.Errorf("device %d: got %s, want user/server of %s", i, got[i], userTestPNJID)
		}
	}
}

// ParseDeviceList recebe o JID do usuario por valor e escreve em `user.Device`
// dentro do laco. Se algum dia isso virar ponteiro, cada dispositivo passaria a
// sobrescrever o anterior — este teste trava a independencia das entradas.
func TestParseDeviceListDoesNotAliasUserJID(t *testing.T) {
	got := ParseDeviceList(userTestPNJID, devicesNode(deviceNode("1", false), deviceNode("2", false)))
	if len(got) != 2 || got[0].Device == got[1].Device {
		t.Fatalf("devices aliased or missing: %v", got)
	}
	if userTestPNJID.Device != 0 {
		t.Errorf("input JID was mutated: %s", userTestPNJID)
	}
}

func TestParseDeviceListHostedServers(t *testing.T) {
	cases := []struct {
		name       string
		user       types.JID
		wantServer string
	}{
		{"PN vira hosted", userTestPNJID, types.HostedServer},
		{"LID vira hosted.lid", userTestLIDJID, types.HostedLIDServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDeviceList(tc.user, devicesNode(deviceNode("7", true)))
			if len(got) != 1 {
				t.Fatalf("expected 1 device, got %v", got)
			}
			if got[0].Server != tc.wantServer {
				t.Errorf("got server %q, want %q", got[0].Server, tc.wantServer)
			}
			if got[0].Device != 7 {
				t.Errorf("got device %d, want 7", got[0].Device)
			}
		})
	}
}

func TestParseDeviceListSkipsInvalidChildren(t *testing.T) {
	got := ParseDeviceList(userTestPNJID, devicesNode(
		waBinary.Node{Tag: "not-a-device", Attrs: waBinary.Attrs{"id": "1"}},
		waBinary.Node{Tag: deviceNodeTag},                                         // sem id
		waBinary.Node{Tag: deviceNodeTag, Attrs: waBinary.Attrs{"id": "not-int"}}, // id nao numerico
		deviceNode("5", false),
	))
	if len(got) != 1 || got[0].Device != 5 {
		t.Fatalf("expected only the valid device 5, got %v", got)
	}
}

func TestParseDeviceListRejectsWrongEnvelope(t *testing.T) {
	cases := map[string]waBinary.Node{
		"tag errada no no externo": {
			Tag:     "not-devices",
			Content: []waBinary.Node{{Tag: deviceListNodeTag, Content: []waBinary.Node{deviceNode("1", false)}}},
		},
		"sem device-list": {
			Tag:     devicesNodeTag,
			Content: []waBinary.Node{{Tag: "other", Content: []waBinary.Node{deviceNode("1", false)}}},
		},
		"no vazio": {},
	}
	for name, node := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ParseDeviceList(userTestPNJID, node); got != nil {
				t.Errorf("expected nil, got %v", got)
			}
		})
	}
}

func TestParseDeviceListEmptyDeviceList(t *testing.T) {
	got := ParseDeviceList(userTestPNJID, devicesNode())
	if len(got) != 0 {
		t.Errorf("expected no devices, got %v", got)
	}
}

// --- ParseFBDeviceList ---

func TestParseFBDeviceListReadsDevicesAndDhash(t *testing.T) {
	node := waBinary.Node{
		Tag:   devicesNodeTag,
		Attrs: waBinary.Attrs{"dhash": "abc123"},
		Content: []waBinary.Node{
			deviceNode("0", false),
			{Tag: "icdc"}, // filho de outra tag e' ignorado
			deviceNode("3", false),
		},
	}
	got := ParseFBDeviceList(userTestFBJID, node)
	if got.DHash != "abc123" {
		t.Errorf("got dhash %q, want %q", got.DHash, "abc123")
	}
	if len(got.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %v", got.Devices)
	}
	if got.Devices[0].Device != 0 || got.Devices[1].Device != 3 {
		t.Errorf("unexpected device ids: %v", got.Devices)
	}
	if got.Devices[0].Server != types.MessengerServer {
		t.Errorf("server should be preserved, got %q", got.Devices[0].Server)
	}
}

// ParseFBDeviceList, ao contrario de ParseDeviceList, NAO tem o conceito de
// `is_hosted` — o atributo e' ignorado. Travado para que a diferenca entre os
// dois parsers seja deliberada e nao acidente.
func TestParseFBDeviceListIgnoresIsHosted(t *testing.T) {
	got := ParseFBDeviceList(userTestFBJID, waBinary.Node{
		Tag:     devicesNodeTag,
		Content: []waBinary.Node{deviceNode("1", true)},
	})
	if len(got.Devices) != 1 {
		t.Fatalf("expected 1 device, got %v", got.Devices)
	}
	if got.Devices[0].Server != types.MessengerServer {
		t.Errorf("got server %q, want %q", got.Devices[0].Server, types.MessengerServer)
	}
}

func TestParseFBDeviceListEmpty(t *testing.T) {
	got := ParseFBDeviceList(userTestFBJID, waBinary.Node{Tag: devicesNodeTag})
	if len(got.Devices) != 0 || got.DHash != "" {
		t.Errorf("expected empty cache entry, got %+v", got)
	}
}

// --- GetDevices: o roteamento por servidor antes de qualquer I/O ---

// Bot JID nao gera consulta: entra direto no resultado.
func TestGetDevicesBotJIDSkipsQuery(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	if !botJID.IsBot() {
		t.Skipf("%s is not recognized as a bot JID", botJID)
	}
	f := newFakeTransport()
	got, err := GetDevices(t.Context(), f, []types.JID{botJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != botJID {
		t.Errorf("got %v, want [%s]", got, botJID)
	}
	if len(f.sent) != 0 {
		t.Error("nao deveria ter disparado consulta")
	}
}

func TestGetDevicesUsesCacheWithoutQuery(t *testing.T) {
	f := newFakeTransport()
	cached := []types.JID{
		{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 0},
		{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 2},
	}
	f.putCached(userTestPNJID, DeviceEntry{Devices: cached})

	got, err := GetDevices(t.Context(), f, []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != cached[0] || got[1] != cached[1] {
		t.Errorf("got %v, want %v", got, cached)
	}
	if len(f.sent) != 0 {
		t.Error("cache quente nao deveria consultar o servidor")
	}
}

// Entrada existente porem VAZIA nao conta como hit: cai na consulta. Este era o
// caminho que a Fase E nao conseguia observar sem um Client conectado — com o
// duble de transporte ele fica visivel.
func TestGetDevicesEmptyCacheEntryStillQueries(t *testing.T) {
	f := newFakeTransport()
	f.putCached(userTestPNJID, DeviceEntry{})
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, devicesNode(deviceNode("0", false))),
	)}

	got, err := GetDevices(t.Context(), f, []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("mandou %d <iq>, queria 1", len(f.sent))
	}
	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestGetDevicesEmptyInput(t *testing.T) {
	f := newFakeTransport()
	got, err := GetDevices(t.Context(), f, nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got (%v, %v), want (empty, nil)", got, err)
	}
}

// A consulta usync grava cada lista no cache com o dhash calculado, e o
// resultado agrega as listas de todos os usuarios da resposta.
func TestGetDevicesCachesAndAggregatesUsyncResult(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, devicesNode(deviceNode("0", false), deviceNode("1", false))),
		usyncUser(userTestPN2JID, devicesNode(deviceNode("0", false))),
		// Filho que nao e' <user> e <user> sem jid sao ignorados.
		waBinary.Node{Tag: "outra-tag", Attrs: waBinary.Attrs{"jid": userTestLIDJID}},
		waBinary.Node{Tag: usyncUserTag},
	)}

	got, err := GetDevices(t.Context(), f, []types.JID{userTestPNJID, userTestPN2JID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %v, want 3 devices", got)
	}
	entry, ok := f.cached(userTestPNJID)
	if !ok || len(entry.Devices) != 2 || entry.DHash != "h:2" {
		t.Errorf("cache do PN = %+v (ok=%v)", entry, ok)
	}
	if entry, ok := f.cached(userTestPN2JID); !ok || entry.DHash != "h:1" {
		t.Errorf("cache do PN2 = %+v (ok=%v)", entry, ok)
	}
	if _, ok := f.cached(userTestLIDJID); ok {
		t.Error("filho de outra tag nao deveria virar entrada de cache")
	}
}

// A consulta usync e' feita com mode=query/context=message. Trava o par, que e'
// o que o servidor usa para priorizar a resposta no caminho de envio.
func TestGetDevicesUsesMessageContext(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}
	if _, err := GetDevices(t.Context(), f, []types.JID{userTestPNJID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	if node.Attrs["mode"] != ModeQuery || node.Attrs["context"] != ContextMessage {
		t.Errorf("attrs = %v", node.Attrs)
	}
	query := node.Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	if len(query) != 1 || query[0].Tag != devicesNodeTag || query[0].Attrs["version"] != deviceListVersion {
		t.Errorf("query = %v", query)
	}
}

func TestGetDevicesPropagatesUsyncError(t *testing.T) {
	boom := errors.New("sem rede")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetDevices(t.Context(), f, []types.JID{userTestPNJID}); !errors.Is(err, boom) {
		t.Errorf("got %v, want %v", err, boom)
	}
}

// JID do Messenger nao passa pelo usync: sai pelo IQ fbid:devices.
func TestGetDevicesRoutesMessengerJIDsToFBID(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{fbidResponse(
		waBinary.Node{
			Tag:   usyncUserTag,
			Attrs: waBinary.Attrs{"jid": userTestFBJID},
			Content: []waBinary.Node{{
				Tag:     devicesNodeTag,
				Attrs:   waBinary.Attrs{"dhash": "fb-hash"},
				Content: []waBinary.Node{deviceNode("0", false)},
			}},
		},
	)}

	got, err := GetDevices(t.Context(), f, []types.JID{userTestFBJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Server != types.MessengerServer {
		t.Errorf("got %v", got)
	}
	if f.sent[0].Namespace != fbidDevicesIQNamespace {
		t.Errorf("namespace = %q", f.sent[0].Namespace)
	}
	if entry, ok := f.cached(userTestFBJID); !ok || entry.DHash != "fb-hash" {
		t.Errorf("cache = %+v (ok=%v)", entry, ok)
	}
}

func TestGetDevicesPropagatesFBIDError(t *testing.T) {
	boom := errors.New("fbid fora do ar")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetDevices(t.Context(), f, []types.JID{userTestFBJID}); !errors.Is(err, boom) {
		t.Errorf("got %v, want %v", err, boom)
	}
}

// --- GetFBIDDevicesInternal / GetFBIDDevices ---

// fbidResponse monta a resposta de um <iq> fbid:devices.
func fbidResponse(users ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     usersNodeTag,
			Content: users,
		}},
	}
}

func TestGetFBIDDevicesInternalBuildsTheQuery(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{fbidResponse()}
	if _, err := GetFBIDDevicesInternal(t.Context(), f, []types.JID{userTestFBJID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	iq := f.sent[0]
	if iq.Namespace != fbidDevicesIQNamespace || iq.Type != IQGet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	users := iq.Content.([]waBinary.Node)[0]
	if users.Tag != usersNodeTag {
		t.Fatalf("tag = %q", users.Tag)
	}
	list := users.Content.([]waBinary.Node)
	if len(list) != 1 || list[0].Tag != usyncUserTag || list[0].Attrs["jid"] != userTestFBJID {
		t.Errorf("lista = %v", list)
	}
}

func TestGetFBIDDevicesInternalWrapsTransportError(t *testing.T) {
	boom := errors.New("caiu")
	f := newFakeTransport()
	f.err = []error{boom}
	_, err := GetFBIDDevicesInternal(t.Context(), f, nil)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "failed to send usync query") {
		t.Errorf("got %v", err)
	}
}

func TestGetFBIDDevicesInternalMissingUsersIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetFBIDDevicesInternal(t.Context(), f, nil)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != usersNodeTag {
		t.Errorf("got %v", err)
	}
}

// Mais de fbIDDeviceChunkSize usuarios viram varios IQ. Trava o batching, que e'
// o que evita um <iq> gigante que o servidor recusaria.
func TestGetFBIDDevicesChunksTheInput(t *testing.T) {
	const total = fbIDDeviceChunkSize + 3
	jids := make([]types.JID, total)
	for i := range jids {
		jids[i] = types.NewJID(fmt.Sprintf("1000%d", i), types.MessengerServer)
	}
	f := newFakeTransport()
	f.resp = []*waBinary.Node{fbidResponse(), fbidResponse()}

	if _, err := GetFBIDDevices(t.Context(), f, jids); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.sent) != 2 {
		t.Fatalf("mandou %d <iq>, queria 2", len(f.sent))
	}
	first := f.sent[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	second := f.sent[1].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	if len(first) != fbIDDeviceChunkSize || len(second) != 3 {
		t.Errorf("tamanhos dos lotes = %d e %d", len(first), len(second))
	}
}

func TestGetFBIDDevicesSkipsInvalidUsers(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{fbidResponse(
		waBinary.Node{Tag: "outra-tag", Attrs: waBinary.Attrs{"jid": userTestFBJID}},
		waBinary.Node{Tag: usyncUserTag}, // sem jid
	)}
	got, err := GetFBIDDevices(t.Context(), f, []types.JID{userTestFBJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestGetFBIDDevicesPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetFBIDDevices(t.Context(), f, []types.JID{userTestFBJID}); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

// --- DeviceCache ---

// O zero value e' usavel: ler antes de qualquer gravacao nao entra em panic e
// devolve o zero; deletar em mapa nil e' no-op.
func TestDeviceCacheZeroValueIsUsable(t *testing.T) {
	var c DeviceCache
	if entry, ok := c.GetLocked(userTestPNJID); ok || len(entry.Devices) != 0 {
		t.Errorf("leitura em cache vazio = %+v (ok=%v)", entry, ok)
	}
	c.Delete(userTestPNJID)
	c.DeleteLocked(userTestPNJID)
	if c.Len() != 0 {
		t.Errorf("Len = %d", c.Len())
	}
}

func TestDeviceCacheSetGetDeleteRoundTrip(t *testing.T) {
	var c DeviceCache
	entry := DeviceEntry{Devices: []types.JID{userTestPNJID}, DHash: "h"}
	c.Lock()
	c.SetLocked(userTestPNJID, entry)
	c.Unlock()

	if c.Len() != 1 {
		t.Fatalf("Len = %d", c.Len())
	}
	c.Lock()
	got, ok := c.GetLocked(userTestPNJID)
	c.Unlock()
	if !ok || got.DHash != "h" || len(got.Devices) != 1 {
		t.Errorf("got %+v (ok=%v)", got, ok)
	}

	c.Delete(userTestPNJID)
	if c.Len() != 0 {
		t.Errorf("apos Delete, Len = %d", c.Len())
	}

	c.Lock()
	c.SetLocked(userTestPNJID, entry)
	c.DeleteLocked(userTestPNJID)
	_, ok = c.GetLocked(userTestPNJID)
	c.Unlock()
	if ok {
		t.Error("DeleteLocked nao removeu")
	}
}
