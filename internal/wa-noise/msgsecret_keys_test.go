// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/util/hkdfutil"
)

var recvTestSecret = bytes.Repeat([]byte{0xAB}, messageSecretSize)

// --- generateMsgSecretKey ---

// A chave e' conferida contra o algoritmo (HKDF-SHA256 sobre a concatenacao
// declarada), nao contra a propria funcao: e' o mesmo material que o outro lado
// deriva, e um deslize na ordem dos campos so' apareceria como "falha de
// autenticacao" em producao.
func TestGenerateMsgSecretKeyMatchesHKDF(t *testing.T) {
	sender := types.NewJID("5511888888888", types.DefaultUserServer)
	sender.Device = 4
	origSender := types.NewJID("5511777777777", types.DefaultUserServer)
	origSender.Device = 9

	key, aad := generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", origSender, recvTestSecret)

	var want []byte
	want = append(want, "MSG1"...)
	want = append(want, origSender.ToNonAD().String()...)
	want = append(want, sender.ToNonAD().String()...)
	want = append(want, EncSecretPollVote...)
	expectedKey := hkdfutil.SHA256(recvTestSecret, nil, want, msgSecretKeyLength)

	if !bytes.Equal(key, expectedKey) {
		t.Fatalf("chave = %X, queria %X", key, expectedKey)
	}
	if len(key) != msgSecretKeyLength {
		t.Errorf("len(chave) = %d", len(key))
	}
	// O AAD do voto e' "<id da mensagem>\x00<remetente da modificacao>", ambos
	// sem device.
	expectedAAD := fmt.Appendf(nil, "%s\x00%s", "MSG1", sender.ToNonAD().String())
	if !bytes.Equal(aad, expectedAAD) {
		t.Fatalf("aad = %q, queria %q", aad, expectedAAD)
	}
}

// Os JIDs entram sempre sem device: dois aparelhos do mesmo usuario tem que
// derivar a mesma chave, senao um voto so' seria legivel pelo aparelho que o
// mandou.
func TestGenerateMsgSecretKeyIgnoresDevice(t *testing.T) {
	sender := types.NewJID("5511888888888", types.DefaultUserServer)
	origSender := types.NewJID("5511777777777", types.DefaultUserServer)

	base, _ := generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", origSender, recvTestSecret)
	sender.Device = 12
	origSender.Device = 3
	withDevice, _ := generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", origSender, recvTestSecret)

	if !bytes.Equal(base, withDevice) {
		t.Fatal("o device do JID mudou a chave derivada")
	}
}

// So' voto de enquete, resposta de evento e o caso do bot (tipo vazio) levam
// additionalData; para os demais o AAD tem que ser nil, senao o GCM do outro
// lado nao autentica.
func TestGenerateMsgSecretKeyAdditionalDataByType(t *testing.T) {
	sender := types.NewJID("5511888888888", types.DefaultUserServer)
	origSender := types.NewJID("5511777777777", types.DefaultUserServer)

	withAAD := []MsgSecretType{EncSecretPollVote, EncSecretEventResponse, ""}
	withoutAAD := []MsgSecretType{
		EncSecretReaction, EncSecretComment, EncSecretReportToken,
		EncSecretEventEdit, EncSecretMessageEdit, EncSecretPollEdit,
		EncSecretPollAddOption, EncSecretBotMsg,
	}

	for _, useCase := range withAAD {
		if _, aad := generateMsgSecretKey(useCase, sender, "MSG1", origSender, recvTestSecret); aad == nil {
			t.Errorf("%q: aad = nil, queria preenchido", useCase)
		}
	}
	for _, useCase := range withoutAAD {
		if _, aad := generateMsgSecretKey(useCase, sender, "MSG1", origSender, recvTestSecret); aad != nil {
			t.Errorf("%q: aad = %q, queria nil", useCase, aad)
		}
	}
}

// Cada campo do material de derivacao tem que influenciar a chave; se algum
// nao influenciasse, dois usos diferentes compartilhariam chave.
func TestGenerateMsgSecretKeyIsDomainSeparated(t *testing.T) {
	sender := types.NewJID("5511888888888", types.DefaultUserServer)
	origSender := types.NewJID("5511777777777", types.DefaultUserServer)
	base, _ := generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", origSender, recvTestSecret)

	variants := map[string][]byte{}
	variants["outro tipo"], _ = generateMsgSecretKey(EncSecretReaction, sender, "MSG1", origSender, recvTestSecret)
	variants["outro id"], _ = generateMsgSecretKey(EncSecretPollVote, sender, "MSG2", origSender, recvTestSecret)
	variants["outro remetente"], _ = generateMsgSecretKey(EncSecretPollVote, origSender, "MSG1", origSender, recvTestSecret)
	variants["outro remetente original"], _ = generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", sender, recvTestSecret)
	variants["outro segredo"], _ = generateMsgSecretKey(EncSecretPollVote, sender, "MSG1", origSender, bytes.Repeat([]byte{0xCD}, messageSecretSize))

	for name, got := range variants {
		if bytes.Equal(base, got) {
			t.Errorf("%s: derivou a mesma chave", name)
		}
	}
}

func TestApplyBotMessageHKDF(t *testing.T) {
	got := applyBotMessageHKDF(recvTestSecret)
	want := hkdfutil.SHA256(recvTestSecret, nil, []byte(EncSecretBotMsg), msgSecretKeyLength)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %X, want %X", got, want)
	}
	if len(got) != msgSecretKeyLength {
		t.Errorf("len = %d", len(got))
	}
}

// --- getOrigSenderFromKey ---

func msgEvent(chat, sender types.JID) *events.Message {
	return &events.Message{Info: types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: sender},
	}}
}

// fromMe significa que quem mandou a enquete e quem mandou o voto sao o mesmo
// usuario, entao o remetente original e' o do proprio evento.
func TestGetOrigSenderFromKeyFromMe(t *testing.T) {
	evt := msgEvent(recvTestGroupJID, recvTestOtherJID)
	got, err := getOrigSenderFromKey(evt, &waCommon.MessageKey{FromMe: proto.Bool(true)})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got != recvTestOtherJID {
		t.Fatalf("got %s, want %s", got, recvTestOtherJID)
	}
}

// Em DM o remetente original sai do RemoteJID; em grupo, do Participant.
func TestGetOrigSenderFromKeyDMUsesRemoteJID(t *testing.T) {
	for _, chat := range []types.JID{
		recvTestOtherJID,
		types.NewJID("55443322", types.HiddenUserServer),
	} {
		evt := msgEvent(chat, recvTestOtherJID)
		got, err := getOrigSenderFromKey(evt, &waCommon.MessageKey{
			RemoteJID:   proto.String(recvTestOtherJID.String()),
			Participant: proto.String(recvTestGroupJID.String()),
		})
		if err != nil {
			t.Fatalf("chat %s: erro: %v", chat, err)
		}
		if got != recvTestOtherJID {
			t.Fatalf("chat %s: got %s", chat, got)
		}
	}
}

func TestGetOrigSenderFromKeyGroupUsesParticipant(t *testing.T) {
	evt := msgEvent(recvTestGroupJID, recvTestOtherJID)
	got, err := getOrigSenderFromKey(evt, &waCommon.MessageKey{
		RemoteJID:   proto.String(recvTestGroupJID.String()),
		Participant: proto.String(recvTestOtherJID.String()),
	})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got != recvTestOtherJID {
		t.Fatalf("got %s", got)
	}
}

// Correcao do lote 9: quando o ParseJID falhava *e* o JID meio-parseado que ele
// devolve tinha um server inesperado, a causa real do parse era sobrescrita
// pelo "unexpected server" e nunca chegava ao log. Este JID cai exatamente
// nesse caso: user com pontos demais e server de grupo.
func TestGetOrigSenderFromKeyGroupInvalidJIDKeepsParseError(t *testing.T) {
	evt := msgEvent(recvTestGroupJID, recvTestOtherJID)
	_, err := getOrigSenderFromKey(evt, &waCommon.MessageKey{
		Participant: proto.String("1.2.3@g.us"),
	})
	if err == nil {
		t.Fatal("esperava erro")
	}
	if errors.Is(err, errUnexpectedOrigSenderServer) {
		t.Fatalf("erro = %v, queria a causa do parse e nao \"unexpected server\"", err)
	}
	if !strings.Contains(err.Error(), "dots") {
		t.Fatalf("erro = %v, queria conter a causa do ParseJID", err)
	}
}

// Servidor que este pacote nao sabe usar como remetente continua sendo erro,
// agora com sentinela propria.
func TestGetOrigSenderFromKeyGroupUnexpectedServer(t *testing.T) {
	evt := msgEvent(recvTestGroupJID, recvTestOtherJID)
	_, err := getOrigSenderFromKey(evt, &waCommon.MessageKey{
		Participant: proto.String(recvTestGroupJID.String()),
	})
	if !errors.Is(err, errUnexpectedOrigSenderServer) {
		t.Fatalf("erro = %v, want errUnexpectedOrigSenderServer", err)
	}
}

// --- getKeyFromInfo ---

// O Participant so' entra em grupo; em DM ele nao deve existir, e (ao contrario
// de BuildMessageKey) aqui ele vai *com* device, porque e' o que a chave de
// criacao da enquete gravou.
func TestGetKeyFromInfoParticipantOnlyInGroup(t *testing.T) {
	sender := recvTestOtherJID
	sender.Device = 5

	group := getKeyFromInfo(&types.MessageInfo{
		MessageSource: types.MessageSource{Chat: recvTestGroupJID, Sender: sender, IsGroup: true, IsFromMe: true},
		ID:            "MSG1",
	})
	if group.GetParticipant() != sender.String() {
		t.Errorf("Participant = %q, queria %q", group.GetParticipant(), sender.String())
	}
	if !group.GetFromMe() || group.GetID() != "MSG1" || group.GetRemoteJID() != recvTestGroupJID.String() {
		t.Errorf("chave = %+v", group)
	}

	dm := getKeyFromInfo(&types.MessageInfo{
		MessageSource: types.MessageSource{Chat: recvTestOtherJID, Sender: sender},
		ID:            "MSG1",
	})
	if dm.Participant != nil {
		t.Errorf("Participant = %q em DM, queria ausente", dm.GetParticipant())
	}
	if dm.GetFromMe() {
		t.Errorf("FromMe = true")
	}
}
