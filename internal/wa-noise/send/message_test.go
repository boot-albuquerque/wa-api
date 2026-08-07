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
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// loggedIn prepara o duble com uma identidade valida, que e' o minimo para
// Message passar das guardas de entrada.
func loggedIn() *fakeTransport {
	tr := newFakeTransport()
	tr.ownID = types.JID{User: "5511000000000", Server: types.DefaultUserServer, Device: 1}
	tr.ownLI = types.JID{User: "11111111", Server: types.HiddenUserServer, Device: 1}
	return tr
}

func textMessage() *waE2E.Message {
	return &waE2E.Message{Conversation: proto.String("oi")}
}

// --- guardas de entrada, na ordem original ---

// to.Device > 0 e' recusado ANTES da checagem de login. A ordem importa: trocar
// as duas mudaria qual erro o chamador recebe quando as duas condicoes valem.
func TestMessageRejectsADJIDBeforeCheckingLogin(t *testing.T) {
	tr := newFakeTransport() // deslogado de proposito
	to := types.JID{User: "1", Server: types.DefaultUserServer, Device: 2}
	_, err := Message(context.Background(), tr, to, textMessage(), RequestExtra{})
	if !errors.Is(err, ErrRecipientADJID) {
		t.Fatalf("err = %v, want ErrRecipientADJID", err)
	}
}

// Com Peer o device part e' permitido — e ai' a proxima guarda (login) dispara.
func TestMessagePeerAllowsADJIDAndThenChecksLogin(t *testing.T) {
	tr := newFakeTransport()
	to := types.JID{User: "1", Server: types.DefaultUserServer, Device: 2}
	_, err := Message(context.Background(), tr, to, textMessage(), RequestExtra{Peer: true})
	if !errors.Is(err, errNotLoggedIn) {
		t.Fatalf("err = %v, want NotLoggedIn", err)
	}
}

func TestMessageRejectsUnknownServer(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: "example.com"}
	resp, err := Message(context.Background(), tr, to, textMessage(), RequestExtra{})
	if !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("err = %v, want ErrUnknownServer", err)
	}
	// O waiter tem que ter sido cancelado no caminho de erro, ou o ID vaza.
	if _, ok := tr.waiter(resp.ID); ok {
		t.Error("o waiter deveria ser cancelado quando o envio falha")
	}
}

// --- preenchimento de req/resp ---

func TestMessageGeneratesIDAndTimeoutWhenAbsent(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: "example.com"} // falha depois, ja' com req montado
	resp, _ := Message(context.Background(), tr, to, textMessage(), RequestExtra{})
	if resp.ID != tr.generatedID {
		t.Errorf("resp.ID = %q, queria o ID gerado %q", resp.ID, tr.generatedID)
	}
	if resp.Sender != tr.ownID {
		t.Errorf("resp.Sender = %v, want %v", resp.Sender, tr.ownID)
	}
}

func TestMessageKeepsCallerProvidedID(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: "example.com"}
	resp, _ := Message(context.Background(), tr, to, textMessage(), RequestExtra{ID: "MEU"})
	if resp.ID != "MEU" {
		t.Errorf("resp.ID = %q, want MEU", resp.ID)
	}
}

// Em newsletter o ID nao e' escolhido por nos: edicao e revoke reaproveitam o
// ID da mensagem-alvo, senao o servidor nao acha o que editar/apagar.
func TestMessageNewsletterEditAndRevokeReuseTargetID(t *testing.T) {
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	cases := map[string]struct {
		message *waE2E.Message
		wantID  types.MessageID
	}{
		"edicao": {&waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				Key: &waCommon.MessageKey{ID: proto.String("ALVO_EDIT")},
			}},
		}}, "ALVO_EDIT"},
		"revoke": {&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String("ALVO_REVOKE")},
		}}, "ALVO_REVOKE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := loggedIn()
			done := feedAck(tr, string(tc.wantID), waBinary.Attrs{})
			resp, err := Message(context.Background(), tr, newsletter, tc.message, RequestExtra{ID: "IGNORADO"})
			<-done
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if resp.ID != tc.wantID {
				t.Errorf("resp.ID = %q, want %q", resp.ID, tc.wantID)
			}
		})
	}
}

// --- caminho completo de newsletter (o unico sem Signal) ---

// Newsletter e' texto plano no fio, entao o envio inteiro roda sem sessao
// Signal. E' o unico caminho de Message que da' para exercitar ponta a ponta
// em teste de unidade.
func TestMessageNewsletterEndToEnd(t *testing.T) {
	tr := loggedIn()
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	req := RequestExtra{ID: "MSG1", Timeout: time.Second, MediaHandle: "HANDLE"}

	done := feedAck(tr, "MSG1", waBinary.Attrs{ackAttrTime: "1700000000", ackAttrServerID: "7"})
	resp, err := Message(context.Background(), tr, newsletter, textMessage(), req)
	<-done
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.ServerID != 7 || resp.Timestamp.Unix() != 1700000000 {
		t.Errorf("resposta = %+v", resp)
	}
	if len(tr.sentNodes) != 1 {
		t.Fatalf("nos enviados = %d, queria 1", len(tr.sentNodes))
	}
	node := tr.sentNodes[0]
	if node.Tag != messageNodeTag {
		t.Errorf("tag = %q", node.Tag)
	}
	if node.Attrs[msgAttrMediaID] != "HANDLE" {
		t.Errorf("media_id = %v", node.Attrs[msgAttrMediaID])
	}
	if !hasTag(node.GetChildren(), plaintextNodeTag) {
		t.Errorf("faltou <plaintext>, filhos = %v", childTags(node.GetChildren()))
	}
}

// Falha ao gravar a mensagem no cache de reenvio aborta antes de tocar o
// socket — a mensagem nao pode ir para o fio se nao da' para reenviar depois.
func TestMessageAbortsWhenRecentMessageFails(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.recentErr = sentinel
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}

	_, err := Message(context.Background(), tr, newsletter, textMessage(), RequestExtra{ID: "MSG1"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if len(tr.sentNodes) != 0 {
		t.Error("nada deveria ter ido para o socket")
	}
}

// Mensagem peer nao entra no cache de reenvio (retry de peer nao existe), entao
// um erro do cache nao pode aborta-la.
func TestMessagePeerSkipsRecentMessage(t *testing.T) {
	tr := loggedIn()
	tr.recentErr = errors.New("nao deveria ser consultado")
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	done := feedAck(tr, "MSG1", waBinary.Attrs{})
	defer func() { <-done }()

	if _, err := Message(
		context.Background(), tr, newsletter, textMessage(), RequestExtra{ID: "MSG1", Peer: true},
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// O erro de gravacao do message secret e' apenas logado: perder a chave nao
// pode impedir o envio.
func TestMessageMessageSecretFailureIsNotFatal(t *testing.T) {
	tr := loggedIn()
	tr.store.MsgSecrets = failingMsgSecrets{&store.NoopStore{}}
	newsletter := types.JID{User: "999", Server: types.NewsletterServer}
	msg := &waE2E.Message{
		Conversation:       proto.String("oi"),
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("segredo")},
	}
	done := feedAck(tr, "MSG1", waBinary.Attrs{})
	defer func() { <-done }()
	if _, err := Message(
		context.Background(), tr, newsletter, msg, RequestExtra{ID: "MSG1"},
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(tr.sentNodes) != 1 {
		t.Error("o envio deveria ter acontecido mesmo assim")
	}
}
