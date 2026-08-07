// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/proto/waWeb"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

type stubPrivacyTokenStore struct {
	tokens []store.PrivacyToken
}

func (s *stubPrivacyTokenStore) PutPrivacyTokens(_ context.Context, tokens ...store.PrivacyToken) error {
	s.tokens = append(s.tokens, tokens...)
	return nil
}

func (s *stubPrivacyTokenStore) GetPrivacyToken(context.Context, types.JID) (*store.PrivacyToken, error) {
	return nil, nil
}

func (s *stubPrivacyTokenStore) DeleteExpiredPrivacyTokens(context.Context, time.Time) (int64, error) {
	return 0, nil
}

// --- storeMessageSecret ---

// So' grava quando ha' segredo; um MessageContextInfo sem MessageSecret nao
// pode virar uma linha vazia no banco.
func TestStoreMessageSecretOnlyWhenPresent(t *testing.T) {
	stub := &stubMsgSecretStore{}
	cli := recvSecretClient(t, stub)
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: recvTestGroupJID, Sender: recvTestOtherJID},
		ID:            "MSG1",
	}

	cli.storeMessageSecret(context.Background(), info, &waE2E.Message{})
	if stub.putSecret != nil {
		t.Fatalf("gravou %X sem segredo na mensagem", stub.putSecret)
	}

	cli.storeMessageSecret(context.Background(), info, &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: recvTestSecret},
	})
	if !bytes.Equal(stub.putSecret, recvTestSecret) {
		t.Fatalf("segredo = %X", stub.putSecret)
	}
	if stub.putChat != recvTestGroupJID || stub.putSender != recvTestOtherJID || stub.putID != "MSG1" {
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
	cli := &Client{Log: recvTestClient(t).Log, Store: &store.Device{MsgSecrets: stub}}
	cli.storeHistoricalMessageSecrets(context.Background(), []*waHistorySync.Conversation{
		historyConv(recvTestGroupJID.String(), historyMsg("MSG1", true, "", "", recvTestSecret)),
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
	cli := recvSecretClient(t, stub)
	own := cli.getOwnID().ToNonAD()
	participant := recvTestOtherJID

	convs := []*waHistorySync.Conversation{
		// grupo, mensagem propria -> remetente e' o proprio JID
		historyConv(recvTestGroupJID.String(), historyMsg("G_ME", true, "", "", recvTestSecret)),
		// grupo, mensagem alheia com Key.Participant
		historyConv(recvTestGroupJID.String(), historyMsg("G_KEY", false, participant.String(), "", recvTestSecret)),
		// grupo, mensagem alheia so' com Message.Participant
		historyConv(recvTestGroupJID.String(), historyMsg("G_MSG", false, "", participant.String(), recvTestSecret)),
		// DM alheia -> remetente e' o proprio chat
		historyConv(participant.String(), historyMsg("DM", false, "", "", recvTestSecret)),
	}
	cli.storeHistoricalMessageSecrets(context.Background(), convs)

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
	cli := recvSecretClient(t, stub)

	cli.storeHistoricalMessageSecrets(context.Background(), []*waHistorySync.Conversation{
		// conversa com ID que nao e' JID
		historyConv("", historyMsg("X", true, "", "", recvTestSecret)),
		// sem segredo
		historyConv(recvTestGroupJID.String(), historyMsg("SEM_SEGREDO", true, "", "", nil)),
		// grupo, alheia, sem participant em lugar nenhum
		historyConv(recvTestGroupJID.String(), historyMsg("SEM_SENDER", false, "", "", recvTestSecret)),
		// sem ID de mensagem
		historyConv(recvTestGroupJID.String(), historyMsg("", true, "", "", recvTestSecret)),
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
	cli := recvSecretClient(t, stub)
	cli.Store.PrivacyTokens = tokens

	dm := historyConv(recvTestOtherJID.String())
	dm.TcToken = []byte("token")
	dm.TcTokenTimestamp = proto.Uint64(1700000000)
	dm.TcTokenSenderTimestamp = proto.Uint64(1700000001)
	group := historyConv(recvTestGroupJID.String())
	group.TcToken = []byte("token")

	cli.storeHistoricalMessageSecrets(context.Background(), []*waHistorySync.Conversation{dm, group})

	if len(tokens.tokens) != 1 {
		t.Fatalf("%d tokens, queria 1 (so' o de DM)", len(tokens.tokens))
	}
	tok := tokens.tokens[0]
	if tok.User != recvTestOtherJID {
		t.Errorf("user = %s", tok.User)
	}
	if tok.Timestamp.Unix() != 1700000000 || tok.SenderTimestamp.Unix() != 1700000001 {
		t.Errorf("timestamps = %v / %v", tok.Timestamp, tok.SenderTimestamp)
	}
}
