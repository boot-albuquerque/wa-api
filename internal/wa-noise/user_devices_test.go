// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// --- infraestrutura compartilhada dos testes do lote 7 ---

var (
	userTestPNJID  = types.NewJID("5511999", types.DefaultUserServer)
	userTestLIDJID = types.NewJID("8877", types.HiddenUserServer)
	userTestFBJID  = types.NewJID("12345", types.MessengerServer)
)

// userTestClient monta o minimo de Client que o parsing de usuario precisa.
func userTestClient() *Client {
	return &Client{
		Log:              waLog.Noop,
		userDevicesCache: make(map[types.JID]deviceCache),
	}
}

// deviceNode monta um `<device id=... [is_hosted=...]>`.
func deviceNode(id string, hosted bool) waBinary.Node {
	attrs := waBinary.Attrs{"id": id}
	if hosted {
		attrs["is_hosted"] = "true"
	}
	return waBinary.Node{Tag: deviceNodeTag, Attrs: attrs}
}

// devicesNode monta o `<devices><device-list>...</device-list></devices>` que o
// servidor devolve dentro de cada `<user>` da resposta usync.
func devicesNode(children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag: devicesNodeTag,
		Content: []waBinary.Node{{
			Tag:     deviceListNodeTag,
			Content: children,
		}},
	}
}

// --- parseDeviceList ---

func TestParseDeviceListReadsAllDevices(t *testing.T) {
	got := parseDeviceList(userTestPNJID, devicesNode(
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

// parseDeviceList recebe o JID do usuario por valor e escreve em `user.Device`
// dentro do laco. Se algum dia isso virar ponteiro, cada dispositivo passaria a
// sobrescrever o anterior — este teste trava a independencia das entradas.
func TestParseDeviceListDoesNotAliasUserJID(t *testing.T) {
	got := parseDeviceList(userTestPNJID, devicesNode(deviceNode("1", false), deviceNode("2", false)))
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
			got := parseDeviceList(tc.user, devicesNode(deviceNode("7", true)))
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
	got := parseDeviceList(userTestPNJID, devicesNode(
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
			if got := parseDeviceList(userTestPNJID, node); got != nil {
				t.Errorf("expected nil, got %v", got)
			}
		})
	}
}

func TestParseDeviceListEmptyDeviceList(t *testing.T) {
	got := parseDeviceList(userTestPNJID, devicesNode())
	if len(got) != 0 {
		t.Errorf("expected no devices, got %v", got)
	}
}

// --- parseFBDeviceList ---

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
	got := parseFBDeviceList(userTestFBJID, node)
	if got.dhash != "abc123" {
		t.Errorf("got dhash %q, want %q", got.dhash, "abc123")
	}
	if len(got.devices) != 2 {
		t.Fatalf("expected 2 devices, got %v", got.devices)
	}
	if got.devices[0].Device != 0 || got.devices[1].Device != 3 {
		t.Errorf("unexpected device ids: %v", got.devices)
	}
	if got.devices[0].Server != types.MessengerServer {
		t.Errorf("server should be preserved, got %q", got.devices[0].Server)
	}
}

// parseFBDeviceList, ao contrario de parseDeviceList, NAO tem o conceito de
// `is_hosted` — o atributo e' ignorado. Travado para que a diferenca entre os
// dois parsers seja deliberada e nao acidente.
func TestParseFBDeviceListIgnoresIsHosted(t *testing.T) {
	got := parseFBDeviceList(userTestFBJID, waBinary.Node{
		Tag:     devicesNodeTag,
		Content: []waBinary.Node{deviceNode("1", true)},
	})
	if len(got.devices) != 1 {
		t.Fatalf("expected 1 device, got %v", got.devices)
	}
	if got.devices[0].Server != types.MessengerServer {
		t.Errorf("got server %q, want %q", got.devices[0].Server, types.MessengerServer)
	}
}

func TestParseFBDeviceListEmpty(t *testing.T) {
	got := parseFBDeviceList(userTestFBJID, waBinary.Node{Tag: devicesNodeTag})
	if len(got.devices) != 0 || got.dhash != "" {
		t.Errorf("expected empty cache entry, got %+v", got)
	}
}

// --- GetUserDevices: o roteamento por servidor antes de qualquer I/O ---

// Bot JID nao gera consulta: entra direto no resultado. Como o Client de teste
// nao tem socket, qualquer caminho que fosse para a rede devolveria erro — o
// sucesso aqui prova que nenhuma consulta foi disparada.
func TestGetUserDevicesBotJIDSkipsQuery(t *testing.T) {
	cli := userTestClient()
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	if !botJID.IsBot() {
		t.Skipf("%s is not recognized as a bot JID", botJID)
	}
	got, err := cli.GetUserDevices(t.Context(), []types.JID{botJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != botJID {
		t.Errorf("got %v, want [%s]", got, botJID)
	}
}

func TestGetUserDevicesUsesCacheWithoutQuery(t *testing.T) {
	cli := userTestClient()
	cached := []types.JID{
		{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 0},
		{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 2},
	}
	cli.userDevicesCache[userTestPNJID] = deviceCache{devices: cached}

	got, err := cli.GetUserDevices(t.Context(), []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != cached[0] || got[1] != cached[1] {
		t.Errorf("got %v, want %v", got, cached)
	}
}

// Nao ha teste para "entrada de cache existente mas vazia nao conta como hit"
// (`len(cached.devices) > 0`): esse caminho cai na consulta usync, e sem socket
// `waitResponse` estoura em mapa nil antes de devolver `ErrNotConnected` — ou
// seja, nao da para observar a decisao sem montar um `Client` conectado. Fica
// para o lote do nucleo do `Client`.

func TestGetUserDevicesNilClient(t *testing.T) {
	var cli *Client
	if _, err := cli.GetUserDevices(t.Context(), nil); err != ErrClientIsNil {
		t.Errorf("got %v, want ErrClientIsNil", err)
	}
}

func TestGetUserDevicesEmptyInput(t *testing.T) {
	got, err := userTestClient().GetUserDevices(t.Context(), nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got (%v, %v), want (empty, nil)", got, err)
	}
}
