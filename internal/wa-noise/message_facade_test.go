// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/message"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// recvTestClient e' o *Client minimo do lote 9. Difere de sendTestClient por
// ja' vir com um Store: quase tudo do caminho de recepcao passa por
// getOwnID/getOwnLID.
func recvTestClient(t *testing.T) *Client {
	t.Helper()
	ownID := types.NewJID("5511999999999", types.DefaultUserServer)
	ownID.Device = 0
	return &Client{
		Log: waLog.Noop,
		Store: &store.Device{
			ID:  &ownID,
			LID: types.NewJID("11223344556677", types.HiddenUserServer),
		},
	}
}

// A raiz reexporta os cinco sentinelas de segredo de mensagem por ATRIBUICAO.
// Se alguem trocar por um errors.New proprio, errors.Is deixa de atravessar a
// fronteira e quem compara com o nome historico passa a receber "nao casa" em
// silencio.
func TestMessageErrorsAreTheSameValues(t *testing.T) {
	pairs := []struct {
		name       string
		root, subp error
	}{
		{"ErrOriginalMessageSecretNotFound", ErrOriginalMessageSecretNotFound, message.ErrOriginalMessageSecretNotFound},
		{"ErrNotEncryptedReactionMessage", ErrNotEncryptedReactionMessage, message.ErrNotEncryptedReactionMessage},
		{"ErrNotEncryptedCommentMessage", ErrNotEncryptedCommentMessage, message.ErrNotEncryptedCommentMessage},
		{"ErrNotSecretEncryptedMessage", ErrNotSecretEncryptedMessage, message.ErrNotSecretEncryptedMessage},
		{"ErrNotPollUpdateMessage", ErrNotPollUpdateMessage, message.ErrNotPollUpdateMessage},
		{"EventAlreadyProcessed", EventAlreadyProcessed, message.ErrEventAlreadyProcessed},
	}
	for _, p := range pairs {
		if p.root != p.subp {
			t.Errorf("%s: raiz e subpacote sao valores diferentes", p.name)
		}
	}
}

// As constantes de use case sao entrada de HKDF. Reexportar por atribuicao e'
// o que garante um unico dono do valor; redeclarar as strings dos dois lados e'
// exatamente a divergencia silenciosa que a Fase F/G existe para impedir.
func TestMsgSecretConstantsAreReexported(t *testing.T) {
	pairs := []struct {
		root, subp MsgSecretType
	}{
		{EncSecretPollVote, message.EncSecretPollVote},
		{EncSecretReaction, message.EncSecretReaction},
		{EncSecretComment, message.EncSecretComment},
		{EncSecretReportToken, message.EncSecretReportToken},
		{EncSecretEventResponse, message.EncSecretEventResponse},
		{EncSecretEventEdit, message.EncSecretEventEdit},
		{EncSecretMessageEdit, message.EncSecretMessageEdit},
		{EncSecretPollEdit, message.EncSecretPollEdit},
		{EncSecretPollAddOption, message.EncSecretPollAddOption},
		{EncSecretBotMsg, message.EncSecretBotMsg},
	}
	for _, p := range pairs {
		if p.root != p.subp {
			t.Errorf("%q != %q", p.root, p.subp)
		}
	}
	if WebMessageIDPrefix != message.WebMessageIDPrefix {
		t.Errorf("WebMessageIDPrefix = %q, subpacote = %q", WebMessageIDPrefix, message.WebMessageIDPrefix)
	}
	if EditWindow != message.EditWindow {
		t.Errorf("EditWindow = %s, subpacote = %s", EditWindow, message.EditWindow)
	}
}

// MsgSecretType e messageEncryptedSecret sao APELIDOS, e nao definicoes novas:
// internals.go (gerado) cita os dois, e um tipo novo obrigaria conversao em
// toda travessia da fronteira.
func TestMsgSecretTypesAreAliases(t *testing.T) {
	var rootType MsgSecretType = message.EncSecretPollVote
	var subType message.SecretType = rootType
	_ = subType

	var enc messageEncryptedSecret = &waE2E.EncReactionMessage{}
	var subEnc message.EncryptedSecret = enc
	_ = subEnc
}

// A fachada da raiz tem que produzir a mesma chave que a funcao livre com os
// dois JIDs consultados na mesma ordem. Se a fachada trocasse ownID por ownLID,
// o Participant de mensagem propria apareceria em grupo.
func TestBuildMessageKeyFacadeMatchesSubpackage(t *testing.T) {
	cli := recvTestClient(t)
	chat := types.NewJID("123456789-987654321", types.GroupServer)
	sender := types.NewJID("5511888888888", types.DefaultUserServer)

	got := cli.BuildMessageKey(chat, sender, "MSG1")
	want := message.BuildKey(cli.getOwnID(), cli.getOwnLID(), chat, sender, "MSG1")
	if got.GetFromMe() != want.GetFromMe() || got.GetParticipant() != want.GetParticipant() ||
		got.GetRemoteJID() != want.GetRemoteJID() || got.GetID() != want.GetID() {
		t.Fatalf("fachada = %+v, subpacote = %+v", got, want)
	}
}

// A fachada de geracao de ID tem que continuar aceitando receptor nil: era
// assim antes da extracao (getOwnID trata nil), e chamadores de fora usam isso.
func TestGenerateMessageIDNilClient(t *testing.T) {
	var cli *Client
	id := cli.GenerateMessageID()
	if !strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("id = %q", id)
	}
}

// As duas guardas de receptor nil que ja' existiam ANTES da extracao continuam
// na raiz, porque um *Client nil nao produz Transport.
func TestMsgSecretNilClient(t *testing.T) {
	var cli *Client
	if _, err := cli.decryptMsgSecret(context.Background(), &events.Message{}, EncSecretReaction, &waE2E.EncReactionMessage{}, &waCommon.MessageKey{}); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("decryptMsgSecret: err = %v", err)
	}
	if _, _, err := cli.encryptMsgSecret(context.Background(), types.EmptyJID, types.EmptyJID, types.EmptyJID, "MSG1", EncSecretReaction, nil); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("encryptMsgSecret: err = %v", err)
	}
}

// pbSerializer continua na raiz para retry_transport.go, e tem que ser o MESMO
// serializador que message/ usa direto. Se divergirem, o caminho de retry
// desserializaria com um serializador e o de recepcao com outro.
func TestPBSerializerMatchesStore(t *testing.T) {
	if pbSerializer != store.SignalProtobufSerializer {
		t.Fatal("pbSerializer da raiz divergiu de store.SignalProtobufSerializer")
	}
}
