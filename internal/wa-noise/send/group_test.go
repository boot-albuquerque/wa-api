// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/protocol/msgattrs"
	"wa-api/internal/wa-noise/protocol/types"
)

// --- Group / GroupV3 (sender key) ---

// A chave de remetente e' criada e gravada no store; com a lista de
// dispositivos vazia, nenhuma cifragem por dispositivo acontece, entao da' para
// exercitar a montagem inteira do <message> de grupo.
func TestGroupBuildsSenderKeyNode(t *testing.T) {
	tr := loggedIn()
	tr.sendData = []byte("frame")
	var timings DebugTimings
	phash, data, err := Group(
		context.Background(), tr, tr.ownID, sendTestGroupJID,
		[]types.JID{sendTestUserJID}, "MSG1", textMessage(), &timings, NodeExtraParams{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "frame" {
		t.Errorf("data = %q", data)
	}
	node := tr.sentNodes[0]
	if node.Attrs[msgAttrPHash] != phash {
		t.Errorf("phash no no = %v, devolvido = %q", node.Attrs[msgAttrPHash], phash)
	}
	// O ultimo filho e' o <enc type=skmsg v=2> com o ciphertext do grupo.
	children := node.GetChildren()
	last := children[len(children)-1]
	if last.Tag != encNodeTag {
		t.Fatalf("ultimo filho = %q, filhos = %v", last.Tag, childTags(children))
	}
	if last.Attrs[encAttrType] != encTypeSenderKey || last.Attrs[encAttrVersion] != encVersionSignal {
		t.Errorf("atributos do skmsg = %v", last.Attrs)
	}
	if timings.GroupEncrypt <= 0 {
		t.Error("GroupEncrypt deveria ter sido cronometrado")
	}
}

// O reporting token de grupo entra DEPOIS do skmsg, e so' com segredo.
func TestGroupReportingTokenIsLast(t *testing.T) {
	tr := loggedIn()
	tr.reportingToken = true
	msg := textMessage()
	msg.MessageContextInfo = messageContextWithSecret()
	var timings DebugTimings
	if _, _, err := Group(
		context.Background(), tr, tr.ownID, sendTestGroupJID,
		[]types.JID{sendTestUserJID}, "MSG1", msg, &timings, NodeExtraParams{},
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	children := tr.sentNodes[0].GetChildren()
	if children[len(children)-1].Tag != "reporting_token" {
		t.Errorf("filhos = %v, reporting_token deveria ser o ultimo", childTags(children))
	}
}

func TestGroupPropagatesSendError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.sendErr = sentinel
	var timings DebugTimings
	if _, _, err := Group(
		context.Background(), tr, tr.ownID, sendTestGroupJID, nil, "MSG1",
		textMessage(), &timings, NodeExtraParams{},
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// A versao v3 do skmsg escreve "3" (string) no `v`, ao contrario do <enc> por
// dispositivo, que escreve o numero. E' assim no fio.
func TestGroupV3SenderKeyNodeUsesStringVersion(t *testing.T) {
	tr := loggedIn()
	tr.groupMeta = &group.Meta{Members: []types.JID{sendTestUserJID}}
	tr.sendData = []byte("frame")
	var timings DebugTimings
	phash, data, err := GroupV3(
		context.Background(), tr, sendTestGroupJID, tr.ownLI, "MSG1", []byte("app"),
		msgattrs.MessageAttrs{Type: msgTypeText, MediaType: "image"}, []byte("tag"), &timings,
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "frame" {
		t.Errorf("data = %q", data)
	}
	node := tr.sentNodes[0]
	if node.Attrs[msgAttrPHash] != phash {
		t.Errorf("phash = %v", node.Attrs[msgAttrPHash])
	}
	children := node.GetChildren()
	last := children[len(children)-1]
	if last.Attrs[encAttrVersion] != encVersionFB {
		t.Errorf("v = %v (%T), queria a string %q", last.Attrs[encAttrVersion], last.Attrs[encAttrVersion], encVersionFB)
	}
	if last.Attrs[encAttrMediaType] != "image" {
		t.Errorf("mediatype = %v", last.Attrs[encAttrMediaType])
	}
}

// --- wrappers exportados para as fachadas da raiz ---

// CopyAttrs e os dois EncryptForDeviceAndWrap* existem so' para internals.go e
// retry_transport.go; sao delegacoes de uma linha, mas se alguem trocar a
// direcao de copia ou o wireIdentity, o fio quebra em silencio.
func TestCopyAttrsExportedWrapper(t *testing.T) {
	dst := waBinary.Attrs{encAttrType: encTypePreKeyMsg}
	CopyAttrs(waBinary.Attrs{encAttrType: encTypeMsg}, dst)
	if dst[encAttrType] != encTypeMsg {
		t.Errorf("type = %v", dst[encAttrType])
	}
}

func TestEncryptForDeviceAndWrapPropagatesError(t *testing.T) {
	tr := loggedIn()
	if _, _, err := EncryptForDeviceAndWrap(
		context.Background(), tr, []byte("oi"), sendTestUserJID, sendTestUserJID, nil, nil, nil,
	); !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
}

func TestEncryptForDeviceAndWrapV3PropagatesError(t *testing.T) {
	tr := loggedIn()
	if _, err := EncryptForDeviceAndWrapV3(
		context.Background(), tr, nil, nil, nil, sendTestUserJID, nil, nil,
	); !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
}
