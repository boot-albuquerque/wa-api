// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

var (
	recvTestOtherJID = types.NewJID("5511888888888", types.DefaultUserServer)
	recvTestGroupJID = types.NewJID("123456789-987654321", types.GroupServer)
)

// --- BuildMessageKey ---

// A chave e' o que liga revoke/reaction/edit a' mensagem original. Errar
// FromMe ou Participant faz o servidor rejeitar ou aplicar no alvo errado.
func TestBuildMessageKeyFromMe(t *testing.T) {
	cli := recvTestClient(t)

	for _, sender := range []types.JID{
		types.EmptyJID,
		cli.getOwnID(),
		cli.getOwnLID(),
	} {
		key := cli.BuildMessageKey(recvTestGroupJID, sender, "MSG1")
		if !key.GetFromMe() {
			t.Errorf("sender %s: FromMe = false, queria true", sender)
		}
		if key.Participant != nil {
			t.Errorf("sender %s: Participant = %q, queria ausente", sender, key.GetParticipant())
		}
		if key.GetRemoteJID() != recvTestGroupJID.String() {
			t.Errorf("RemoteJID = %q", key.GetRemoteJID())
		}
		if key.GetID() != "MSG1" {
			t.Errorf("ID = %q", key.GetID())
		}
	}
}

// Em grupo, revogar mensagem alheia precisa do Participant; em DM ele nao vai,
// porque o RemoteJID ja' identifica o remetente.
func TestBuildMessageKeyOtherSenderParticipantOnlyInGroup(t *testing.T) {
	cli := recvTestClient(t)

	tests := []struct {
		name            string
		chat            types.JID
		wantParticipant bool
	}{
		{"grupo", recvTestGroupJID, true},
		{"broadcast", types.NewJID("status", types.BroadcastServer), true},
		{"dm pn", recvTestOtherJID, false},
		{"dm lid", types.NewJID("99887766", types.HiddenUserServer), false},
		{"messenger", types.NewJID("1234", types.MessengerServer), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := cli.BuildMessageKey(tc.chat, recvTestOtherJID, "MSG1")
			if key.GetFromMe() {
				t.Errorf("FromMe = true, queria false")
			}
			if got := key.Participant != nil; got != tc.wantParticipant {
				t.Fatalf("Participant presente = %v, queria %v", got, tc.wantParticipant)
			}
			if tc.wantParticipant && key.GetParticipant() != recvTestOtherJID.ToNonAD().String() {
				t.Errorf("Participant = %q, queria %q", key.GetParticipant(), recvTestOtherJID.ToNonAD().String())
			}
		})
	}
}

// O Participant vai sempre sem device: e' o usuario que mandou, nao o aparelho.
func TestBuildMessageKeyParticipantIsNonAD(t *testing.T) {
	cli := recvTestClient(t)
	sender := recvTestOtherJID
	sender.Device = 7

	key := cli.BuildMessageKey(recvTestGroupJID, sender, "MSG1")
	if key.GetParticipant() != recvTestOtherJID.ToNonAD().String() {
		t.Fatalf("Participant = %q, queria sem device", key.GetParticipant())
	}
}

// --- BuildRevoke / BuildReaction / BuildEdit ---

func TestBuildRevoke(t *testing.T) {
	cli := recvTestClient(t)
	msg := cli.BuildRevoke(recvTestGroupJID, types.EmptyJID, "MSG1")

	if msg.GetProtocolMessage().GetType() != waE2E.ProtocolMessage_REVOKE {
		t.Fatalf("tipo = %s", msg.GetProtocolMessage().GetType())
	}
	if msg.GetProtocolMessage().GetKey().GetID() != "MSG1" {
		t.Errorf("id da chave = %q", msg.GetProtocolMessage().GetKey().GetID())
	}
}

func TestBuildReaction(t *testing.T) {
	cli := recvTestClient(t)
	before := time.Now().UnixMilli()
	msg := cli.BuildReaction(recvTestGroupJID, recvTestOtherJID, "MSG1", "🐈️")
	after := time.Now().UnixMilli()

	reaction := msg.GetReactionMessage()
	if reaction.GetText() != "🐈️" {
		t.Errorf("texto = %q", reaction.GetText())
	}
	if ts := reaction.GetSenderTimestampMS(); ts < before || ts > after {
		t.Errorf("timestamp %d fora de [%d, %d]", ts, before, after)
	}
	if reaction.GetKey().GetFromMe() {
		t.Errorf("FromMe = true para reacao a mensagem alheia")
	}
}

// Reacao vazia e' o jeito de *remover* a reacao — nao pode virar nil.
func TestBuildReactionEmptyRemoves(t *testing.T) {
	cli := recvTestClient(t)
	msg := cli.BuildReaction(recvTestGroupJID, recvTestOtherJID, "MSG1", "")
	if msg.GetReactionMessage().Text == nil {
		t.Fatal("Text = nil, queria string vazia explicita")
	}
}

// A edicao e' um ProtocolMessage dentro de um EditedMessage, e a chave e'
// sempre FromMe: so' se edita mensagem propria.
func TestBuildEditShape(t *testing.T) {
	cli := recvTestClient(t)
	content := &waE2E.Message{Conversation: proto.String("editado")}
	before := time.Now().UnixMilli()
	msg := cli.BuildEdit(recvTestGroupJID, "MSG1", content)
	after := time.Now().UnixMilli()

	inner := msg.GetEditedMessage().GetMessage().GetProtocolMessage()
	if inner.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		t.Fatalf("tipo = %s", inner.GetType())
	}
	if !inner.GetKey().GetFromMe() {
		t.Errorf("FromMe = false")
	}
	if inner.GetKey().Participant != nil {
		t.Errorf("Participant presente na edicao: %q", inner.GetKey().GetParticipant())
	}
	if inner.GetEditedMessage() != content {
		t.Errorf("conteudo editado nao e' o passado")
	}
	if ts := inner.GetTimestampMS(); ts < before || ts > after {
		t.Errorf("timestamp %d fora de [%d, %d]", ts, before, after)
	}
}

func TestEditWindowValue(t *testing.T) {
	if EditWindow != 20*time.Minute {
		t.Fatalf("EditWindow = %s, queria 20m", EditWindow)
	}
}

// --- BuildUnavailableMessageRequest / BuildHistorySyncRequest ---

func TestBuildUnavailableMessageRequest(t *testing.T) {
	cli := recvTestClient(t)
	msg := cli.BuildUnavailableMessageRequest(recvTestGroupJID, recvTestOtherJID, "MSG1")

	pdo := msg.GetProtocolMessage().GetPeerDataOperationRequestMessage()
	if pdo.GetPeerDataOperationRequestType() != waE2E.PeerDataOperationRequestType_PLACEHOLDER_MESSAGE_RESEND {
		t.Fatalf("tipo = %s", pdo.GetPeerDataOperationRequestType())
	}
	if len(pdo.GetPlaceholderMessageResendRequest()) != 1 {
		t.Fatalf("%d pedidos, queria 1", len(pdo.GetPlaceholderMessageResendRequest()))
	}
	if got := pdo.GetPlaceholderMessageResendRequest()[0].GetMessageKey().GetID(); got != "MSG1" {
		t.Errorf("id = %q", got)
	}
}

// O campo se chama OldestMsgTimestampMS mas o servidor espera *segundos*. O
// comentario no codigo diz isso; este teste trava o comportamento.
func TestBuildHistorySyncRequestTimestampIsSeconds(t *testing.T) {
	cli := recvTestClient(t)
	ts := time.Unix(1700000000, 0)
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: recvTestGroupJID, IsFromMe: true},
		ID:            "MSG1",
		Timestamp:     ts,
	}

	req := cli.BuildHistorySyncRequest(info, 50).GetProtocolMessage().GetPeerDataOperationRequestMessage()
	od := req.GetHistorySyncOnDemandRequest()
	if got := od.GetOldestMsgTimestampMS(); got != ts.Unix() {
		t.Fatalf("timestamp = %d, queria %d (segundos, nao ms)", got, ts.Unix())
	}
	if od.GetOnDemandMsgCount() != 50 {
		t.Errorf("count = %d", od.GetOnDemandMsgCount())
	}
	if od.GetChatJID() != recvTestGroupJID.String() {
		t.Errorf("chat = %q", od.GetChatJID())
	}
	if !od.GetOldestMsgFromMe() {
		t.Errorf("OldestMsgFromMe = false")
	}
	if req.GetPeerDataOperationRequestType() != waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND {
		t.Errorf("tipo = %s", req.GetPeerDataOperationRequestType())
	}
}
