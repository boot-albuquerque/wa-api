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
)

func blocklistNode(dhash string, children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag:     "list",
		Attrs:   waBinary.Attrs{"dhash": dhash},
		Content: children,
	}
}

func TestParseBlocklistReadsDHashAndJIDs(t *testing.T) {
	other := types.NewJID("5511888", types.DefaultUserServer)
	node := blocklistNode("2:abc",
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": other}},
	)
	got := userTestClient().parseBlocklist(&node)
	if got.DHash != "2:abc" {
		t.Errorf("got dhash %q, want %q", got.DHash, "2:abc")
	}
	if len(got.JIDs) != 2 || got.JIDs[0] != userTestPNJID || got.JIDs[1] != other {
		t.Errorf("got JIDs %v", got.JIDs)
	}
}

// A tag do filho nao e' conferida — o que filtra e' o `jid` ser lido com
// sucesso. Travado para que o criterio real fique explicito.
func TestParseBlocklistSkipsChildrenWithoutValidJID(t *testing.T) {
	node := blocklistNode("",
		waBinary.Node{Tag: "item"}, // sem jid
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": "not-a-jid"}}, // jid de tipo errado
		waBinary.Node{Tag: "qualquer-tag", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)
	got := userTestClient().parseBlocklist(&node)
	if len(got.JIDs) != 1 || got.JIDs[0] != userTestPNJID {
		t.Errorf("got %v, want only %s", got.JIDs, userTestPNJID)
	}
}

func TestParseBlocklistEmptyNode(t *testing.T) {
	node := waBinary.Node{Tag: "list"}
	got := userTestClient().parseBlocklist(&node)
	if got == nil {
		t.Fatal("expected a non-nil blocklist")
	}
	if got.DHash != "" || len(got.JIDs) != 0 {
		t.Errorf("got %+v, want zero-valued blocklist", got)
	}
}

// O AttrGetter e' recriado por filho, entao um filho invalido nao contamina os
// seguintes. Sem isso, um unico item ruim mataria o resto da lista.
func TestParseBlocklistInvalidChildDoesNotPoisonLaterOnes(t *testing.T) {
	node := blocklistNode("",
		waBinary.Node{Tag: "item"},
		waBinary.Node{Tag: "item", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
	)
	got := userTestClient().parseBlocklist(&node)
	if len(got.JIDs) != 1 {
		t.Fatalf("got %v, want 1 JID", got.JIDs)
	}
}
