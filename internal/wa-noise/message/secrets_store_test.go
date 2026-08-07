// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/types"
)

// --- storeMessageSecret ---

// So' grava quando ha' segredo; um MessageContextInfo sem MessageSecret nao
// pode virar uma linha vazia no banco.
func TestStoreMessageSecretOnlyWhenPresent(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID},
		ID:            "MSG1",
	}

	StoreSecret(context.Background(), f, info, &waE2E.Message{})
	if stub.putSecret != nil {
		t.Fatalf("gravou %X sem segredo na mensagem", stub.putSecret)
	}

	StoreSecret(context.Background(), f, info, &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: testSecret},
	})
	if !bytes.Equal(stub.putSecret, testSecret) {
		t.Fatalf("segredo = %X", stub.putSecret)
	}
	if stub.putChat != testGroupJID || stub.putSender != testOtherJID || stub.putID != "MSG1" {
		t.Errorf("chave gravada = %s/%s/%s", stub.putChat, stub.putSender, stub.putID)
	}
}

// --- storeHistoricalMessageSecrets ---

func historyConv(id string, msgs ...*waWeb.WebMessageInfo) *waHistorySync.Conversation {
	conv := &waHistorySync.Conversation{ID: proto.String(id)}
	for _, m := range msgs {
		conv.Messages = append(conv.Messages, &waHistorySync.HistorySyncMsg{Message: m})
	}
	return conv
}

func historyMsg(id string, fromMe bool, participant, msgParticipant string, secret []byte) *waWeb.WebMessageInfo {
	msg := &waWeb.WebMessageInfo{
		Key:           &waCommon.MessageKey{ID: proto.String(id), FromMe: proto.Bool(fromMe)},
		MessageSecret: secret,
	}
	if participant != "" {
		msg.Key.Participant = proto.String(participant)
	}
	if msgParticipant != "" {
		msg.Participant = proto.String(msgParticipant)
	}
	return msg
}

// Sem JID proprio nao da' para atribuir as mensagens fromMe, entao a funcao
// desiste inteira em vez de gravar segredos com remetente errado.
func TestStoreHistoricalMessageSecretsRequiresOwnJID(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)
	// Sem JID proprio: e' o estado de antes do pareamento.
	f.ownID = types.EmptyJID
	StoreHistoricalSecrets(context.Background(), f, []*waHistorySync.Conversation{
		historyConv(testGroupJID.String(), historyMsg("MSG1", true, "", "", testSecret)),
	})
	if len(stub.putMany) != 0 {
		t.Fatalf("gravou %d segredos sem JID proprio", len(stub.putMany))
	}
}

// A resolucao do remetente tem quatro ramos, e errar qualquer um grava o
// segredo sob uma chave que nunca sera' consultada — o voto/reacao
// correspondente fica permanentemente ilegivel.
func TestStoreHistoricalMessageSecretsSenderResolution(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)
	own := testOwnJID.ToNonAD()
	participant := testOtherJID

	convs := []*waHistorySync.Conversation{
		// grupo, mensagem propria -> remetente e' o proprio JID
		historyConv(testGroupJID.String(), historyMsg("G_ME", true, "", "", testSecret)),
		// grupo, mensagem alheia com Key.Participant
		historyConv(testGroupJID.String(), historyMsg("G_KEY", false, participant.String(), "", testSecret)),
		// grupo, mensagem alheia so' com Message.Participant
		historyConv(testGroupJID.String(), historyMsg("G_MSG", false, "", participant.String(), testSecret)),
		// DM alheia -> remetente e' o proprio chat
		historyConv(participant.String(), historyMsg("DM", false, "", "", testSecret)),
	}
	StoreHistoricalSecrets(context.Background(), f, convs)

	got := map[string]types.JID{}
	for _, ins := range stub.putMany {
		got[ins.ID] = ins.Sender
	}
	want := map[string]types.JID{
		"G_ME":  own,
		"G_KEY": participant,
		"G_MSG": participant,
		"DM":    participant,
	}
	if len(got) != len(want) {
		t.Fatalf("gravou %d segredos, queria %d: %v", len(got), len(want), got)
	}
	for id, wantSender := range want {
		if got[id] != wantSender {
			t.Errorf("%s: remetente = %s, queria %s", id, got[id], wantSender)
		}
	}
}

// Mensagem sem segredo, sem remetente resolvivel ou sem ID nao gera insercao.
func TestStoreHistoricalMessageSecretsSkipsIncomplete(t *testing.T) {
	stub := &stubMsgSecretStore{}
	f := newFakeTransport().withSecrets(stub)

	StoreHistoricalSecrets(context.Background(), f, []*waHistorySync.Conversation{
		// conversa com ID que nao e' JID
		historyConv("", historyMsg("X", true, "", "", testSecret)),
		// sem segredo
		historyConv(testGroupJID.String(), historyMsg("SEM_SEGREDO", true, "", "", nil)),
		// grupo, alheia, sem participant em lugar nenhum
		historyConv(testGroupJID.String(), historyMsg("SEM_SENDER", false, "", "", testSecret)),
		// sem ID de mensagem
		historyConv(testGroupJID.String(), historyMsg("", true, "", "", testSecret)),
	})
	if len(stub.putMany) != 0 {
		t.Fatalf("gravou %d segredos incompletos: %+v", len(stub.putMany), stub.putMany)
	}
}

// O tc token so' vale para conversa de telefone; em grupo/LID nao ha' token de
// privacidade a guardar.
func TestStoreHistoricalMessageSecretsPrivacyTokens(t *testing.T) {
	stub := &stubMsgSecretStore{}
	tokens := &stubPrivacyTokenStore{}
	f := newFakeTransport().withSecrets(stub)
	f.dev.PrivacyTokens = tokens

	dm := historyConv(testOtherJID.String())
	dm.TcToken = []byte("token")
	dm.TcTokenTimestamp = proto.Uint64(1700000000)
	dm.TcTokenSenderTimestamp = proto.Uint64(1700000001)
	group := historyConv(testGroupJID.String())
	group.TcToken = []byte("token")

	StoreHistoricalSecrets(context.Background(), f, []*waHistorySync.Conversation{dm, group})

	if len(tokens.tokens) != 1 {
		t.Fatalf("%d tokens, queria 1 (so' o de DM)", len(tokens.tokens))
	}
	tok := tokens.tokens[0]
	if tok.User != testOtherJID {
		t.Errorf("user = %s", tok.User)
	}
	if tok.Timestamp.Unix() != 1700000000 || tok.SenderTimestamp.Unix() != 1700000001 {
		t.Errorf("timestamps = %v / %v", tok.Timestamp, tok.SenderTimestamp)
	}
}
