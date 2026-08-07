// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// recvTestClient e' o *Client minimo do lote 9. Difere de sendTestClient por
// ja' vir com um Store: quase tudo do caminho de recepcao passa por
// getOwnID/getOwnLID.
func recvTestClient(t *testing.T) *Client {
	t.Helper()
	ownID := types.NewJID("5511999999999", types.DefaultUserServer)
	ownID.Device = 0
	return &Client{
		Log: waLog.Noop,
		Store: &store.Device{
			ID:  &ownID,
			LID: types.NewJID("11223344556677", types.HiddenUserServer),
		},
	}
}

func TestGenerateMessageIDFormat(t *testing.T) {
	cli := recvTestClient(t)
	id := cli.GenerateMessageID()

	if !strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("id %q nao comeca com %q", id, WebMessageIDPrefix)
	}
	suffix := strings.TrimPrefix(string(id), WebMessageIDPrefix)
	// webMessageIDHashLength bytes viram o dobro de caracteres hex.
	if len(suffix) != webMessageIDHashLength*2 {
		t.Errorf("len(sufixo) = %d, queria %d", len(suffix), webMessageIDHashLength*2)
	}
	if suffix != strings.ToUpper(suffix) {
		t.Errorf("sufixo %q nao esta' em maiusculas", suffix)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Errorf("sufixo %q nao e' hex: %v", suffix, err)
	}
}

// O ID carrega 16 bytes aleatorios; duas chamadas no mesmo segundo, com o mesmo
// JID, ainda tem que divergir.
func TestGenerateMessageIDIsRandom(t *testing.T) {
	cli := recvTestClient(t)
	seen := make(map[types.MessageID]struct{}, 64)
	for i := 0; i < 64; i++ {
		id := cli.GenerateMessageID()
		if _, dup := seen[id]; dup {
			t.Fatalf("id repetido na iteracao %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

// Sem JID (antes do pareamento) o material do hash nao leva o sufixo @c.us, mas
// o ID continua bem formado.
func TestGenerateMessageIDWithoutOwnJID(t *testing.T) {
	cli := &Client{Log: waLog.Noop, Store: &store.Device{}}
	id := cli.GenerateMessageID()
	if len(id) != len(WebMessageIDPrefix)+webMessageIDHashLength*2 {
		t.Fatalf("id = %q (len %d)", id, len(id))
	}
}

// Com MessengerConfig o ID passa a ser o inteiro do Facebook em base 10, sem
// prefixo nenhum.
func TestGenerateMessageIDMessengerConfig(t *testing.T) {
	cli := recvTestClient(t)
	cli.MessengerConfig = &MessengerConfig{}
	id := cli.GenerateMessageID()
	if strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("id do messenger %q nao devia ter prefixo web", id)
	}
	if _, err := strconv.ParseInt(string(id), 10, 64); err != nil {
		t.Fatalf("id do messenger %q nao e' um int64: %v", id, err)
	}
}

// Client nil e' um caminho suportado (getOwnID trata nil); nao pode entrar no
// ramo do Messenger nem entrar em panico.
func TestGenerateMessageIDNilClient(t *testing.T) {
	var cli *Client
	id := cli.GenerateMessageID()
	if !strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("id = %q", id)
	}
}

// O ID do Messenger e' (unix ms << 22) | aleatorio de 22 bits. Confere-se o
// desmembramento contra o relogio, nao contra a propria funcao.
func TestGenerateFacebookMessageIDLayout(t *testing.T) {
	before := time.Now().UnixMilli()
	id := GenerateFacebookMessageID()
	after := time.Now().UnixMilli()

	ts := id >> facebookMessageIDRandomBits
	if ts < before || ts > after {
		t.Errorf("timestamp %d fora de [%d, %d]", ts, before, after)
	}
	if random := id & ((1 << facebookMessageIDRandomBits) - 1); random < 0 {
		t.Errorf("parte aleatoria negativa: %d", random)
	}
	if id <= 0 {
		t.Errorf("id = %d, queria positivo", id)
	}
}

func TestGenerateFacebookMessageIDIsRandom(t *testing.T) {
	seen := make(map[int64]struct{}, 64)
	for i := 0; i < 64; i++ {
		id := GenerateFacebookMessageID()
		if _, dup := seen[id]; dup {
			t.Fatalf("id repetido na iteracao %d", i)
		}
		seen[id] = struct{}{}
	}
}

// A funcao depreciada nao passa por hash: sao legacyMessageIDRandomLength bytes
// crus em hex.
func TestGenerateMessageIDDeprecated(t *testing.T) {
	id := GenerateMessageID()
	suffix := strings.TrimPrefix(string(id), WebMessageIDPrefix)
	if len(suffix) != legacyMessageIDRandomLength*2 {
		t.Fatalf("len(sufixo) = %d, queria %d", len(suffix), legacyMessageIDRandomLength*2)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Errorf("sufixo %q nao e' hex: %v", suffix, err)
	}
}
