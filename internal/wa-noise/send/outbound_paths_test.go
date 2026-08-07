// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

// --- Newsletter ---

func TestNewsletterPlainTextNode(t *testing.T) {
	tr := loggedIn()
	tr.sendData = []byte("frame")
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	var timings DebugTimings
	data, err := Newsletter(context.Background(), tr, newsletter, "MSG1", textMessage(), "", &timings)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "frame" {
		t.Errorf("data = %q", data)
	}
	node := tr.sentNodes[0]
	if node.Attrs[msgAttrType] != msgTypeText {
		t.Errorf("type = %v", node.Attrs[msgAttrType])
	}
	if _, ok := node.Attrs[msgAttrMediaID]; ok {
		t.Error("sem handle nao deve haver media_id")
	}
	if timings.Marshal <= 0 || timings.Send <= 0 {
		t.Errorf("timings = %+v", timings)
	}
}

// Edicao e revoke viram atributos `edit` distintos, e o revoke zera a mensagem
// (o servidor so' precisa saber o que apagar).
func TestNewsletterEditAndRevokeAttrs(t *testing.T) {
	cases := map[string]struct {
		message  *waE2E.Message
		wantEdit string
		wantBody bool
	}{
		"edicao": {&waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				EditedMessage: &waE2E.Message{Conversation: proto.String("novo")},
				Key:           &waCommon.MessageKey{ID: proto.String("ALVO")},
			}},
		}}, string(types.EditAttributeAdminEdit), true},
		"revoke": {&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String("ALVO")},
		}}, string(types.EditAttributeAdminRevoke), false},
	}
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			var timings DebugTimings
			if _, err := Newsletter(
				context.Background(), tr, newsletter, "MSG1", tc.message, "", &timings,
			); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			node := tr.sentNodes[0]
			if node.Attrs[msgAttrEdit] != tc.wantEdit {
				t.Errorf("edit = %v, want %q", node.Attrs[msgAttrEdit], tc.wantEdit)
			}
			plaintext := node.GetChildByTag(plaintextNodeTag)
			body, _ := plaintext.Content.([]byte)
			if (len(body) > 0) != tc.wantBody {
				t.Errorf("corpo presente=%v, queria=%v", len(body) > 0, tc.wantBody)
			}
		})
	}
}

func TestNewsletterPropagatesSendError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.sendErr = sentinel
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	var timings DebugTimings
	if _, err := Newsletter(
		context.Background(), tr, newsletter, "MSG1", textMessage(), "", &timings,
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// --- DM (com lista de dispositivos vazia) ---

func TestDMSendsNodeAndReturnsHash(t *testing.T) {
	tr := loggedIn()
	tr.sendData = []byte("frame")
	var timings DebugTimings
	phash, data, err := DM(
		context.Background(), tr, tr.ownID, sendTestUserJID, "MSG1", textMessage(), &timings, NodeExtraParams{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "frame" || phash != ParticipantListHashV2(nil) {
		t.Errorf("data = %q, phash = %q", data, phash)
	}
}

// O <tctoken> entra quando ha token guardado; o <cstoken> so' entra quando NAO
// ha — sao alternativas, nunca os dois no mesmo stanza.
func TestDMTokenNodesAreExclusive(t *testing.T) {
	cases := map[string]struct {
		tcToken []byte
		csToken []byte
		wantTag string
	}{
		"tctoken":   {[]byte("tc"), []byte("cs"), tcTokenNodeTag},
		"cstoken":   {nil, []byte("cs"), csTokenNodeTag},
		"nenhum":    {nil, nil, ""},
		"tc sem cs": {[]byte("tc"), nil, tcTokenNodeTag},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			tr.tcToken = tc.tcToken
			tr.csToken = tc.csToken
			var timings DebugTimings
			if _, _, err := DM(
				context.Background(), tr, tr.ownID, sendTestUserJID, "MSG1", textMessage(), &timings, NodeExtraParams{},
			); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			children := tr.sentNodes[0].GetChildren()
			for _, tag := range []string{tcTokenNodeTag, csTokenNodeTag} {
				want := tag == tc.wantTag
				if hasTag(children, tag) != want {
					t.Errorf("<%s> presente=%v, queria=%v (filhos=%v)", tag, !want, want, childTags(children))
				}
			}
		})
	}
}

// Falha ao buscar o privacy token e' apenas logada: a mensagem sai sem
// <tctoken>, e cai no <cstoken> se houver.
func TestDMTCTokenErrorIsNotFatal(t *testing.T) {
	tr := loggedIn()
	tr.tcTokenErr = errors.New("boom")
	tr.csToken = []byte("cs")
	var timings DebugTimings
	if _, _, err := DM(
		context.Background(), tr, tr.ownID, sendTestUserJID, "MSG1", textMessage(), &timings, NodeExtraParams{},
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !hasTag(tr.sentNodes[0].GetChildren(), csTokenNodeTag) {
		t.Error("deveria ter caido no <cstoken>")
	}
}

// O reporting token so' entra quando o cliente o pede E ha segredo de
// mensagem — sem o segredo nao ha como derivar a chave do HMAC.
func TestDMReportingTokenRequiresSecret(t *testing.T) {
	cases := map[string]struct {
		enabled bool
		secret  []byte
		want    bool
	}{
		"desligado":        {false, []byte("s"), false},
		"ligado sem chave": {true, nil, false},
		"ligado com chave": {true, []byte("s"), true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			tr.reportingToken = tc.enabled
			msg := textMessage()
			if tc.secret != nil {
				msg.MessageContextInfo = &waE2E.MessageContextInfo{MessageSecret: tc.secret}
			}
			var timings DebugTimings
			if _, _, err := DM(
				context.Background(), tr, tr.ownID, sendTestUserJID, "MSG1", msg, &timings, NodeExtraParams{},
			); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got := hasTag(tr.sentNodes[0].GetChildren(), "reporting_token"); got != tc.want {
				t.Errorf("reporting_token presente=%v, queria=%v", got, tc.want)
			}
		})
	}
}

func TestDMPropagatesSendError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.sendErr = sentinel
	var timings DebugTimings
	if _, _, err := DM(
		context.Background(), tr, tr.ownID, sendTestUserJID, "MSG1", textMessage(), &timings, NodeExtraParams{},
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// --- PeerMessage ---

func TestPeerMessageSendsNode(t *testing.T) {
	tr := loggedIn()
	tr.sendData = []byte("frame")
	var timings DebugTimings
	// Destino LID: nao passa pela resolucao PN->LID, que exigiria sessao.
	_, err := PeerMessage(context.Background(), tr, sendTestLIDJID, "MSG1", textMessage(), &timings)
	// Sem sessao Signal a cifragem falha — o que se trava aqui e' que o erro
	// vem embrulhado com o destino, e nao um panic.
	if err == nil {
		t.Fatal("sem sessao a cifragem tem que falhar")
	}
	if !strings.Contains(err.Error(), "failed to encrypt peer message") {
		t.Errorf("err = %q", err)
	}
}

// --- PrepareMessageNode (waE2E) ---

// Dispositivos hospedados sao removidos APENAS em grupo: em DM eles recebem a
// mensagem normalmente.
func TestPrepareMessageNodeDropsHostedDevicesOnlyInGroups(t *testing.T) {
	hosted := types.JID{User: "1", Server: types.HostedServer, Device: 1}
	cases := map[string]struct {
		to        types.JID
		wantCount int
	}{
		"grupo": {sendTestGroupJID, 0},
		"dm":    {sendTestUserJID, 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			tr.devices = []types.JID{hosted}
			var timings DebugTimings
			_, devices, err := PrepareMessageNode(
				context.Background(), tr, tc.to, "MSG1", textMessage(), nil,
				[]byte("oi"), nil, &timings, NodeExtraParams{},
			)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(devices) != tc.wantCount {
				t.Errorf("dispositivos = %v, queria %d", devices, tc.wantCount)
			}
		})
	}
}

// Reacao e voto de enquete escondem a falha de decifragem do destinatario: sao
// mensagens que nao valem um aviso de "mensagem nao entregue".
func TestPrepareMessageNodeDecryptFailHide(t *testing.T) {
	tr := loggedIn()
	var timings DebugTimings
	node, _, err := PrepareMessageNode(
		context.Background(), tr, sendTestUserJID, "MSG1",
		&waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}, nil,
		[]byte("oi"), nil, &timings, NodeExtraParams{},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if node.Attrs[msgAttrType] != msgTypeReaction {
		t.Errorf("type = %v", node.Attrs[msgAttrType])
	}
}

func TestPrepareMessageNodeAddressingModeAttr(t *testing.T) {
	tr := loggedIn()
	var timings DebugTimings
	node, _, err := PrepareMessageNode(
		context.Background(), tr, sendTestGroupJID, "MSG1", textMessage(), nil,
		[]byte("oi"), nil, &timings,
		NodeExtraParams{addressingMode: types.AddressingModeLID},
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if node.Attrs[msgAttrAddressingMode] != string(types.AddressingModeLID) {
		t.Errorf("addressing_mode = %v", node.Attrs[msgAttrAddressingMode])
	}
}

func TestPrepareMessageNodePropagatesDeviceError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.devicesErr = sentinel
	var timings DebugTimings
	if _, _, err := PrepareMessageNode(
		context.Background(), tr, sendTestUserJID, "MSG1", textMessage(), nil,
		nil, nil, &timings, NodeExtraParams{},
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// --- MakeDeviceIdentityNode ---

func TestMakeDeviceIdentityNode(t *testing.T) {
	tr := loggedIn()
	tr.store.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte("detalhe")}
	node := MakeDeviceIdentityNode(tr)
	if node.Tag != deviceIdentityNodeTag {
		t.Errorf("tag = %q", node.Tag)
	}
	if body, _ := node.Content.([]byte); len(body) == 0 {
		t.Error("conteudo vazio")
	}
}
