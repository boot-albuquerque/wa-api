// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// --- HandleBlocklist ---

func TestHandleBlocklist(t *testing.T) {
	tr := &fakeTransport{}
	HandleBlocklist(tr, &waBinary.Node{
		Attrs: waBinary.Attrs{"action": "modify", "dhash": "hash-novo", "prev_dhash": "hash-velho"},
		Content: []waBinary.Node{
			{Tag: "item", Attrs: waBinary.Attrs{"jid": testPeerJID, "action": "block"}},
			// Sem `action`: atributo obrigatorio faltando, filho descartado.
			{Tag: "item", Attrs: waBinary.Attrs{"jid": testOwnJID}},
		},
	})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	evt := tr.events[0].(*events.Blocklist)
	if evt.DHash != "hash-novo" || evt.PrevDHash != "hash-velho" {
		t.Errorf("hashes = %q/%q", evt.DHash, evt.PrevDHash)
	}
	if evt.Action != events.BlocklistAction("modify") {
		t.Errorf("Action = %q", evt.Action)
	}
	if len(evt.Changes) != 1 {
		t.Fatalf("mudancas = %d, esperado 1 (a incompleta e' descartada)", len(evt.Changes))
	}
	if evt.Changes[0].JID != testPeerJID {
		t.Errorf("JID = %v", evt.Changes[0].JID)
	}
}

// Um <blocklist> sem filho nenhum ainda despacha o evento, com Changes nil.
// E' como o snapshot inicial chega.
func TestHandleBlocklistWithoutChanges(t *testing.T) {
	tr := &fakeTransport{}
	HandleBlocklist(tr, &waBinary.Node{Attrs: waBinary.Attrs{"dhash": "h"}})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	if changes := tr.events[0].(*events.Blocklist).Changes; changes != nil {
		t.Errorf("Changes = %v, esperado nil", changes)
	}
}

// --- HandlePicture ---

func TestHandlePicture(t *testing.T) {
	tr := &fakeTransport{}
	HandlePicture(tr, &waBinary.Node{
		Attrs: waBinary.Attrs{"t": "1700000000"},
		Content: []waBinary.Node{
			{Tag: "add", Attrs: waBinary.Attrs{"jid": testPeerJID, "id": "PIC1"}},
			{Tag: "set", Attrs: waBinary.Attrs{"jid": testPeerJID, "id": "PIC2", "author": testOwnJID}},
			{Tag: "delete", Attrs: waBinary.Attrs{"jid": testPeerJID}},
			// Tag desconhecida: nao gera evento.
			{Tag: "rename", Attrs: waBinary.Attrs{"jid": testPeerJID}},
			// Sem jid: atributo obrigatorio faltando, descartado.
			{Tag: "add", Attrs: waBinary.Attrs{"id": "PIC3"}},
		},
	})
	if len(tr.events) != 3 {
		t.Fatalf("eventos = %d, esperado 3", len(tr.events))
	}
	add := tr.events[0].(*events.Picture)
	if add.PictureID != "PIC1" || add.Remove {
		t.Errorf("evento de add = %+v", add)
	}
	if !add.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Timestamp = %v", add.Timestamp)
	}
	set := tr.events[1].(*events.Picture)
	if set.PictureID != "PIC2" || set.Author != testOwnJID {
		t.Errorf("evento de set = %+v", set)
	}
	del := tr.events[2].(*events.Picture)
	if !del.Remove || del.PictureID != "" {
		t.Errorf("evento de delete = %+v", del)
	}
}

// Sem filho nenhum, nada e' despachado — o evento e' por filho, nao por no.
func TestHandlePictureWithoutChildren(t *testing.T) {
	tr := &fakeTransport{}
	HandlePicture(tr, &waBinary.Node{Attrs: waBinary.Attrs{"t": "1"}})
	if len(tr.events) != 0 {
		t.Errorf("eventos = %d, esperado 0", len(tr.events))
	}
}

// --- HandleStatus ---

func TestHandleStatus(t *testing.T) {
	tr := &fakeTransport{}
	HandleStatus(tr, &waBinary.Node{
		Attrs:   waBinary.Attrs{"from": testPeerJID, "t": "1700000000"},
		Content: []waBinary.Node{{Tag: "set", Content: []byte("na praia")}},
	})
	if len(tr.events) != 1 {
		t.Fatalf("eventos = %d, esperado 1", len(tr.events))
	}
	evt := tr.events[0].(*events.UserAbout)
	if evt.Status != "na praia" || evt.JID != testPeerJID {
		t.Errorf("evento = %+v", evt)
	}
	if !evt.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("Timestamp = %v", evt.Timestamp)
	}
}

func TestHandleStatusMalformed(t *testing.T) {
	for name, node := range map[string]waBinary.Node{
		"sem filho set": {
			Attrs:   waBinary.Attrs{"from": testPeerJID, "t": "1"},
			Content: []waBinary.Node{{Tag: "outro", Content: []byte("x")}},
		},
		"conteudo nao binario": {
			Attrs:   waBinary.Attrs{"from": testPeerJID, "t": "1"},
			Content: []waBinary.Node{{Tag: "set", Content: "texto"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			tr := &fakeTransport{}
			node := node
			HandleStatus(tr, &node)
			if len(tr.events) != 0 {
				t.Errorf("eventos = %d, esperado 0", len(tr.events))
			}
		})
	}
}

// --- constantes ---

// A taxonomia e' contrato de wire com o servidor: os valores nao podem mudar
// por refatoracao, e nenhum pode colidir com outro (duas entradas iguais
// fariam o switch da raiz virar codigo morto silenciosamente).
func TestTypeConstants(t *testing.T) {
	want := map[string]string{
		"encrypt":                 TypeEncrypt,
		"server_sync":             TypeServerSync,
		"account_sync":            TypeAccountSync,
		"devices":                 TypeDevices,
		"fbid:devices":            TypeFBIDDevices,
		"w:gp2":                   TypeGroup,
		"picture":                 TypePicture,
		"mediaretry":              TypeMediaRetry,
		"privacy_token":           TypePrivacyToken,
		"link_code_companion_reg": TypeLinkCodeCompanionReg,
		"newsletter":              TypeNewsletter,
		"mex":                     TypeMex,
		"status":                  TypeStatus,
	}
	seen := make(map[string]string, len(want))
	for literal, constant := range want {
		if literal != constant {
			t.Errorf("constante = %q, esperado %q", constant, literal)
		}
		if prev, dup := seen[constant]; dup {
			t.Errorf("valor %q duplicado (tambem em %q)", constant, prev)
		}
		seen[constant] = literal
	}
}
