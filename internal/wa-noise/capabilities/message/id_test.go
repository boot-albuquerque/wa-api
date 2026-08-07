// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

func TestGenerateIDFormat(t *testing.T) {
	id := GenerateID(testOwnJID, false)

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
func TestGenerateIDIsRandom(t *testing.T) {
	seen := make(map[types.MessageID]struct{}, 64)
	for i := 0; i < 64; i++ {
		id := GenerateID(testOwnJID, false)
		if _, dup := seen[id]; dup {
			t.Fatalf("id repetido na iteracao %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

// Sem JID (antes do pareamento, ou com cliente nil na raiz) o material do hash
// nao leva o sufixo @c.us, mas o ID continua bem formado.
func TestGenerateIDWithoutOwnJID(t *testing.T) {
	id := GenerateID(types.EmptyJID, false)
	if len(id) != len(WebMessageIDPrefix)+webMessageIDHashLength*2 {
		t.Fatalf("id = %q (len %d)", id, len(id))
	}
}

// Com Messenger o ID passa a ser o inteiro do Facebook em base 10, sem prefixo
// nenhum.
func TestGenerateIDMessenger(t *testing.T) {
	id := GenerateID(testOwnJID, true)
	if strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("id do messenger %q nao devia ter prefixo web", id)
	}
	if _, err := strconv.ParseInt(string(id), 10, 64); err != nil {
		t.Fatalf("id do messenger %q nao e' um int64: %v", id, err)
	}
}

// O ID do Messenger e' (unix ms << 22) | aleatorio de 22 bits. Confere-se o
// desmembramento contra o relogio, nao contra a propria funcao.
func TestGenerateFacebookIDLayout(t *testing.T) {
	before := time.Now().UnixMilli()
	id := GenerateFacebookID()
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

func TestGenerateFacebookIDIsRandom(t *testing.T) {
	seen := make(map[int64]struct{}, 64)
	for i := 0; i < 64; i++ {
		id := GenerateFacebookID()
		if _, dup := seen[id]; dup {
			t.Fatalf("id repetido na iteracao %d", i)
		}
		seen[id] = struct{}{}
	}
}

// A funcao depreciada nao passa por hash: sao legacyMessageIDRandomLength bytes
// crus em hex.
func TestGenerateLegacyID(t *testing.T) {
	id := GenerateLegacyID()
	suffix := strings.TrimPrefix(string(id), WebMessageIDPrefix)
	if len(suffix) != legacyMessageIDRandomLength*2 {
		t.Fatalf("len(sufixo) = %d, queria %d", len(suffix), legacyMessageIDRandomLength*2)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Errorf("sufixo %q nao e' hex: %v", suffix, err)
	}
}
