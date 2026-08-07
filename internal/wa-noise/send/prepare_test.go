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

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// --- prepareBotMessage ---

func TestPrepareBotMessageRejectsNonBotInlineJID(t *testing.T) {
	tr := loggedIn()
	var extra NodeExtraParams
	req := &RequestExtra{InlineBotJID: sendTestUserJID}
	_, err := prepareBotMessage(context.Background(), tr, req, sendTestUserJID, textMessage(), "MSG1", &extra)
	if !errors.Is(err, ErrInvalidInlineBotID) {
		t.Fatalf("err = %v, want ErrInvalidInlineBotID", err)
	}
}

// Sem bot e sem reporting token, a mensagem sai intacta: nenhum segredo
// gerado, nenhum envelope trocado.
func TestPrepareBotMessagePlainMessageUntouched(t *testing.T) {
	tr := loggedIn()
	var extra NodeExtraParams
	msg := textMessage()
	got, err := prepareBotMessage(context.Background(), tr, &RequestExtra{}, sendTestUserJID, msg, "MSG1", &extra)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != msg {
		t.Error("a mensagem nao deveria ter sido trocada")
	}
	if msg.MessageContextInfo != nil {
		t.Errorf("MessageContextInfo = %+v, queria nil", msg.MessageContextInfo)
	}
	if extra.botNode != nil {
		t.Error("nao deveria haver <bot>")
	}
}

// Reporting token exige segredo de mensagem, mesmo fora de modo bot.
func TestPrepareBotMessageReportingTokenNeedsSecret(t *testing.T) {
	tr := loggedIn()
	tr.reportingToken = true
	var extra NodeExtraParams
	msg := textMessage()
	if _, err := prepareBotMessage(
		context.Background(), tr, &RequestExtra{}, sendTestUserJID, msg, "MSG1", &extra,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(msg.GetMessageContextInfo().GetMessageSecret()) != messageSecretSize {
		t.Errorf("segredo = %d bytes, want %d", len(msg.GetMessageContextInfo().GetMessageSecret()), messageSecretSize)
	}
	if msg.GetMessageContextInfo().GetBotMetadata() != nil {
		t.Error("fora de modo bot nao deveria haver BotMetadata")
	}
}

// Um segredo ja' fornecido pelo chamador nao pode ser sobrescrito: e' com ele
// que a chave do reporting token e do poll ja' foi derivada em outro lugar.
func TestPrepareBotMessageKeepsExistingSecret(t *testing.T) {
	tr := loggedIn()
	tr.reportingToken = true
	secret := []byte("0123456789abcdef0123456789abcdef")
	msg := &waE2E.Message{
		Conversation:       proto.String("oi"),
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: secret},
	}
	var extra NodeExtraParams
	if _, err := prepareBotMessage(
		context.Background(), tr, &RequestExtra{}, sendTestUserJID, msg, "MSG1", &extra,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(msg.MessageContextInfo.MessageSecret) != string(secret) {
		t.Error("o segredo do chamador foi sobrescrito")
	}
}

// Destino que e' bot (mas sem inline bot) ganha segredo e BotMetadata com a
// persona padrao, e para por ai' — nenhum <bot> cifrado.
func TestPrepareBotMessageBotDestinationGetsDefaultPersona(t *testing.T) {
	tr := loggedIn()
	botJID := types.JID{User: "13135550002", Server: types.BotServer}
	msg := textMessage()
	var extra NodeExtraParams
	got, err := prepareBotMessage(context.Background(), tr, &RequestExtra{}, botJID, msg, "MSG1", &extra)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != msg {
		t.Error("fora do modo inline a mensagem nao e' reembrulhada")
	}
	if got.GetMessageContextInfo().GetBotMetadata().GetPersonaID() != defaultBotPersonaID {
		t.Errorf("persona = %q", got.GetMessageContextInfo().GetBotMetadata().GetPersonaID())
	}
	if extra.botNode != nil {
		t.Error("sem inline bot nao ha <bot>")
	}
}

// --- resolveSendTarget ---

func TestResolveSendTargetHiddenUserUsesOwnLID(t *testing.T) {
	tr := loggedIn()
	to := sendTestLIDJID
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	participants, err := resolveSendTarget(context.Background(), tr, &to, &ownID, &RequestExtra{}, &resp, &extra)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if participants != nil {
		t.Errorf("participantes = %v, queria nil", participants)
	}
	if ownID != tr.ownLI {
		t.Errorf("ownID = %v, queria o LID %v", ownID, tr.ownLI)
	}
}

// Sem timestamp de migracao o destino PN fica como esta'.
func TestResolveSendTargetPNWithoutMigrationIsUntouched(t *testing.T) {
	tr := loggedIn()
	to := sendTestUserJID
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	if _, err := resolveSendTarget(
		context.Background(), tr, &to, &ownID, &RequestExtra{}, &resp, &extra,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if to != sendTestUserJID || ownID != tr.ownID {
		t.Errorf("to = %v, ownID = %v — nada deveria ter mudado", to, ownID)
	}
}

// Mensagem peer nunca migra para LID, mesmo com a migracao ligada: o
// destinatario e' o proprio dispositivo.
func TestResolveSendTargetPeerNeverMigrates(t *testing.T) {
	tr := loggedIn()
	tr.store.LIDMigrationTimestamp = 1
	tr.store.LIDs = stubLIDStore{lid: sendTestLIDJID}
	to := sendTestUserJID
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	if _, err := resolveSendTarget(
		context.Background(), tr, &to, &ownID, &RequestExtra{Peer: true}, &resp, &extra,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if to != sendTestUserJID {
		t.Errorf("to = %v, peer nao deveria migrar", to)
	}
}

// --- migrateSendTargetToLID ---

func TestMigrateSendTargetUsesStoredLID(t *testing.T) {
	tr := loggedIn()
	tr.store.LIDMigrationTimestamp = 1
	tr.store.LIDs = stubLIDStore{lid: sendTestLIDJID}
	to := sendTestUserJID
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	if _, err := resolveSendTarget(
		context.Background(), tr, &to, &ownID, &RequestExtra{}, &resp, &extra,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if to != sendTestLIDJID {
		t.Errorf("to = %v, want %v", to, sendTestLIDJID)
	}
	if ownID != tr.ownLI {
		t.Errorf("ownID = %v, want %v", ownID, tr.ownLI)
	}
	if resp.DebugTimings.LIDFetch <= 0 {
		t.Error("LIDFetch deveria ter sido cronometrado")
	}
}

// Sem LID guardado, consulta o servidor (usync) e usa o que vier.
func TestMigrateSendTargetFallsBackToUserInfo(t *testing.T) {
	tr := loggedIn()
	tr.store.LIDs = stubLIDStore{lid: types.EmptyJID}
	tr.userInfo = map[types.JID]types.UserInfo{sendTestUserJID: {LID: sendTestLIDJID}}
	to := sendTestUserJID
	ownID := tr.ownID
	var resp Response
	if err := migrateSendTargetToLID(context.Background(), tr, &to, &ownID, &resp); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if to != sendTestLIDJID {
		t.Errorf("to = %v, want %v", to, sendTestLIDJID)
	}
}

// Servidor sem LID para o numero e' erro: seguir com o JID zerado cifraria
// para um endereco Signal vazio.
func TestMigrateSendTargetFailsWhenServerHasNoLID(t *testing.T) {
	tr := loggedIn()
	tr.store.LIDs = stubLIDStore{lid: types.EmptyJID}
	tr.userInfo = map[types.JID]types.UserInfo{}
	to := sendTestUserJID
	ownID := tr.ownID
	var resp Response
	err := migrateSendTargetToLID(context.Background(), tr, &to, &ownID, &resp)
	if err == nil {
		t.Fatal("queria erro")
	}
	if to != sendTestUserJID {
		t.Errorf("to = %v, nao deveria ter sido trocado", to)
	}
}

func TestMigrateSendTargetPropagatesErrors(t *testing.T) {
	sentinel := errors.New("boom")
	cases := map[string]func(*fakeTransport){
		"store": func(f *fakeTransport) { f.store.LIDs = stubLIDStore{err: sentinel} },
		"usync": func(f *fakeTransport) {
			f.store.LIDs = stubLIDStore{lid: types.EmptyJID}
			f.userInfoErr = sentinel
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			setup(tr)
			to := sendTestUserJID
			ownID := tr.ownID
			var resp Response
			if err := migrateSendTargetToLID(
				context.Background(), tr, &to, &ownID, &resp,
			); !errors.Is(err, sentinel) {
				t.Fatalf("err = %v, want %v", err, sentinel)
			}
		})
	}
}

// --- resolveGroupSendTarget ---

func TestResolveGroupSendTargetBroadcastUsesBroadcastList(t *testing.T) {
	tr := loggedIn()
	tr.broadcast = []types.JID{sendTestUserJID}
	to := types.JID{User: "status", Server: types.BroadcastServer}
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	got, err := resolveGroupSendTarget(context.Background(), tr, to, &ownID, &RequestExtra{}, &resp, &extra)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || got[0] != sendTestUserJID {
		t.Errorf("participantes = %v", got)
	}
	if resp.DebugTimings.GetParticipants <= 0 {
		t.Error("GetParticipants deveria ter sido cronometrado")
	}
}

func TestResolveGroupSendTargetBroadcastError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.broadcastErr = sentinel
	to := types.JID{User: "status", Server: types.BroadcastServer}
	ownID := tr.ownID
	var resp Response
	var extra NodeExtraParams
	if _, err := resolveGroupSendTarget(
		context.Background(), tr, to, &ownID, &RequestExtra{}, &resp, &extra,
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// O modo de enderecamento do grupo decide com qual identidade a mensagem sai.
func TestResolveGroupSendTargetAddressingMode(t *testing.T) {
	meta := &RequestExtra{Meta: &types.MsgMetaInfo{}}
	cases := map[string]struct {
		meta     *group.Meta
		req      *RequestExtra
		wantMode types.AddressingMode
		wantLID  bool
	}{
		"lid": {
			&group.Meta{AddressingMode: types.AddressingModeLID, Members: []types.JID{sendTestUserJID}},
			&RequestExtra{}, types.AddressingModeLID, true,
		},
		"community com meta": {
			&group.Meta{CommunityAnnouncementGroup: true, Members: []types.JID{sendTestUserJID}},
			meta, types.AddressingModePN, true,
		},
		"community sem meta": {
			&group.Meta{CommunityAnnouncementGroup: true, Members: []types.JID{sendTestUserJID}},
			&RequestExtra{}, "", false,
		},
		"pn comum": {
			&group.Meta{Members: []types.JID{sendTestUserJID}},
			&RequestExtra{}, "", false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			tr.groupMeta = tc.meta
			ownID := tr.ownID
			var resp Response
			var extra NodeExtraParams
			members, err := resolveGroupSendTarget(
				context.Background(), tr, sendTestGroupJID, &ownID, tc.req, &resp, &extra,
			)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(members) != 1 {
				t.Errorf("membros = %v", members)
			}
			if extra.addressingMode != tc.wantMode {
				t.Errorf("addressing_mode = %q, want %q", extra.addressingMode, tc.wantMode)
			}
			if (ownID == tr.ownLI) != tc.wantLID {
				t.Errorf("ownID = %v, queria LID=%v", ownID, tc.wantLID)
			}
		})
	}
}
