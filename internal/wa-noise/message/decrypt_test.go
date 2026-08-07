// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/security/gcm"
)

func encNode(encType string, content any, extra ...string) waBinary.Node {
	attrs := waBinary.Attrs{send.EncAttrType: encType, send.EncAttrVersion: "2"}
	for i := 0; i+1 < len(extra); i += 2 {
		attrs[extra[i]] = extra[i+1]
	}
	return waBinary.Node{Tag: send.EncNodeTag, Attrs: attrs, Content: content}
}

func incomingNode(children ...waBinary.Node) *waBinary.Node {
	return msgNode(waBinary.Attrs{
		"from":        testGroupJID,
		"participant": testOtherJID,
		"id":          "MSG1",
		"t":           "1700000000",
	}, children...)
}

func testInfo(t *testing.T, f *fakeTransport, node *waBinary.Node) *types.MessageInfo {
	t.Helper()
	info, err := ParseInfo(f, node)
	if err != nil {
		t.Fatalf("ParseInfo: %v", err)
	}
	return info
}

// --- caminho "unavailable" ---

// Um <message> so' com <unavailable> e sem nenhum <enc> nao tem o que decifrar:
// pede a mensagem ao telefone, acusa com 0 e despacha UndecryptableMessage. NAO
// pode mandar retry receipt (o telefone ja' sabe que nao temos a mensagem).
func TestDecryptMessagesUnavailable(t *testing.T) {
	f := newFakeTransport()
	node := incomingNode(waBinary.Node{Tag: "unavailable", Attrs: waBinary.Attrs{"type": "view_once"}})
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if f.immediateRequests != 1 {
		t.Errorf("immediateRequests = %d, queria 1", f.immediateRequests)
	}
	if f.retryReceipts != 0 {
		t.Errorf("mandou %d retry receipts no caminho unavailable", f.retryReceipts)
	}
	if len(f.acks) != 1 || f.acks[0] != 0 {
		t.Errorf("acks = %v, queria [0]", f.acks)
	}
	if len(f.events) != 1 {
		t.Fatalf("%d eventos", len(f.events))
	}
	evt, ok := f.events[0].(*events.UndecryptableMessage)
	if !ok {
		t.Fatalf("evento = %T", f.events[0])
	}
	if !evt.IsUnavailable || evt.UnavailableType != events.UnavailableType("view_once") {
		t.Errorf("evento = %+v", evt)
	}
}

// Com um <enc> presente, o ramo de unavailable NAO deve disparar: a mensagem
// veio, so' pode ser que nao consigamos decifra-la.
func TestDecryptMessagesUnavailableIgnoredWhenEncPresent(t *testing.T) {
	f := newFakeTransport()
	f.syncAck = true
	node := incomingNode(
		waBinary.Node{Tag: "unavailable"},
		encNode(send.EncTypeMsg, []byte("lixo")),
	)
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if f.immediateRequests != 0 {
		t.Fatalf("entrou no ramo unavailable mesmo com <enc> presente")
	}
}

// --- stanza nao reconhecido / falha de decifragem ---

// Sem nenhum filho <enc>, e sem <unavailable>, o stanza nao e' reconhecido e
// tem que ser nackeado com UnrecognizedStanza — e nao virar recibo de entrega.
func TestDecryptMessagesUnrecognizedStanza(t *testing.T) {
	f := newFakeTransport()
	node := incomingNode(waBinary.Node{Tag: "outro"})
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if len(f.acks) != 1 || f.acks[0] != testNackUnrecognizedStanza {
		t.Fatalf("acks = %v, queria [%d]", f.acks, testNackUnrecognizedStanza)
	}
	if f.msgReceipts != 0 {
		t.Errorf("mandou recibo de entrega para stanza nao reconhecido")
	}
}

// Um <enc> sem o atributo `type` e' pulado, mas ainda conta como stanza
// reconhecido: o ack final e' o recibo normal, nao um nack.
func TestDecryptMessagesEncWithoutTypeIsSkipped(t *testing.T) {
	f := newFakeTransport()
	node := incomingNode(waBinary.Node{Tag: send.EncNodeTag, Attrs: waBinary.Attrs{}})
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if f.msgReceipts != 1 {
		t.Fatalf("msgReceipts = %d, queria 1", f.msgReceipts)
	}
	if len(f.acks) != 0 {
		t.Errorf("acks = %v, queria nenhum", f.acks)
	}
}

// Tipo de <enc> desconhecido e' apenas logado e pulado.
func TestDecryptMessagesUnhandledEncType(t *testing.T) {
	f := newFakeTransport()
	node := incomingNode(encNode("tipo-inexistente", []byte("x")))
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if f.msgReceipts != 1 {
		t.Fatalf("msgReceipts = %d, queria 1", f.msgReceipts)
	}
}

// Sem sessao Signal, decifrar um <enc type="msg"> falha. O caminho de erro tem
// que: mandar retry receipt, acusar com 0 e despachar UndecryptableMessage
// carregando o DecryptFailMode lido do atributo do <enc>.
func TestDecryptMessagesDecryptFailureSendsRetryReceipt(t *testing.T) {
	f := newFakeTransport()
	f.syncAck = true
	node := incomingNode(encNode(send.EncTypeMsg, []byte("nao e' um SignalMessage"),
		send.EncAttrDecryptFail, "hide"))
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if f.retryReceipts != 1 {
		t.Errorf("retryReceipts = %d, queria 1", f.retryReceipts)
	}
	if len(f.acks) != 1 || f.acks[0] != 0 {
		t.Errorf("acks = %v, queria [0]", f.acks)
	}
	if f.msgReceipts != 0 {
		t.Errorf("mandou recibo de entrega apos falha de decifragem")
	}
	if len(f.events) != 1 {
		t.Fatalf("%d eventos", len(f.events))
	}
	evt := f.events[0].(*events.UndecryptableMessage)
	if evt.DecryptFailMode != events.DecryptFailMode("hide") {
		t.Errorf("DecryptFailMode = %q", evt.DecryptFailMode)
	}
	if evt.IsUnavailable {
		t.Errorf("IsUnavailable = true para falha de <enc type=msg>")
	}
}

// Contexto ja' cancelado: a funcao desiste ANTES de mandar recibo ou despachar
// evento. Mandar retry receipt em cima de um shutdown so' geraria trafego que
// ninguem vai processar.
func TestDecryptMessagesCancelledContextReturnsEarly(t *testing.T) {
	f := newFakeTransport()
	f.syncAck = true
	ctx, cancel := context.WithCancel(context.Background())
	node := incomingNode(encNode(send.EncTypeMsg, []byte("lixo")))
	info := testInfo(t, f, node)
	cancel()
	DecryptMessages(ctx, f, info, node)

	if f.retryReceipts != 0 || len(f.acks) != 0 || len(f.events) != 0 {
		t.Fatalf("retry=%d acks=%v eventos=%d, queria nada", f.retryReceipts, f.acks, len(f.events))
	}
}

// --- regressao do lote 9 (Fase E): panico remoto no <enc type="msmsg"> ---

// Antes da correcao, este ramo fazia `child.Content.([]byte)` sem checar o
// segundo retorno. Um <enc type="msmsg"> com FILHOS (ou sem conteudo) — que o
// servidor, ou um par malicioso, pode mandar — derrubava o cliente INTEIRO por
// type assertion. A correcao virou um erro de decifragem comum.
//
// O teste so' passou a ser possivel apos a extracao: antes exigiria um *Client
// real com socket para chegar em decryptMessages.
func TestDecryptMessagesMsgSecretNonByteContentDoesNotPanic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content any
	}{
		{"filhos em vez de bytes", []waBinary.Node{{Tag: "interno"}}},
		{"sem conteudo", nil},
		{"string", "nao sou []byte"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret})
			botJID := types.NewJID("1234", types.BotServer)
			node := msgNode(waBinary.Attrs{
				"from": botJID,
				"id":   "MSG1",
				"t":    "1700000000",
			},
				waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{"target_id": "TARGET1"}},
				encNode(EncTypeMsgSecret, tc.content),
			)
			info := testInfo(t, f, node)

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panico: %v", r)
				}
			}()
			DecryptMessages(context.Background(), f, info, node)

			// O erro do ramo msmsg e' acusado com o nack proprio, e NAO com
			// retry receipt: o telefone nao tem como reenviar um segredo.
			if len(f.acks) != 1 || f.acks[0] != testNackMissingMessageSecret {
				t.Fatalf("acks = %v, queria [%d]", f.acks, testNackMissingMessageSecret)
			}
			if f.retryReceipts != 0 {
				t.Errorf("mandou retry receipt no ramo msmsg")
			}
		})
	}
}

// Segredo ausente no store tambem vira erro comum com o nack proprio.
func TestDecryptMessagesMsgSecretNotFound(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: nil})
	botJID := types.NewJID("1234", types.BotServer)
	node := msgNode(waBinary.Attrs{"from": botJID, "id": "MSG1", "t": "1700000000"},
		waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{"target_id": "TARGET1"}},
		encNode(EncTypeMsgSecret, []byte("x")),
	)
	DecryptMessages(context.Background(), f, testInfo(t, f, node), node)

	if len(f.acks) != 1 || f.acks[0] != testNackMissingMessageSecret {
		t.Fatalf("acks = %v", f.acks)
	}
}

// Caminho FELIZ do ramo de bot, ponta a ponta: e' o unico <enc> que da' para
// decifrar de verdade sem sessao Signal, porque a chave sai de HKDF sobre um
// segredo que o store devolve.
func TestDecryptMessagesMsgSecretRoundTrip(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret})
	botJID := types.NewJID("1234", types.BotServer)
	node := msgNode(waBinary.Attrs{"from": botJID, "id": "MSG1", "t": "1700000000"},
		waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{"target_id": "TARGET1"}},
	)
	info := testInfo(t, f, node)

	// O alvo e' o proprio LID porque info.Sender.Server e' BotServer.
	inner, err := proto.Marshal(&waE2E.Message{Conversation: proto.String("oi do bot")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	key, aad := GenerateSecretKey("", info.Sender, "MSG1", testOwnLID, ApplyBotMessageHKDF(testSecret))
	iv := make([]byte, msgSecretIVSize)
	ciphertext, err := gcmutil.Encrypt(key, iv, inner, aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	payload, err := proto.Marshal(&waE2E.MessageSecretMessage{EncPayload: ciphertext, EncIV: iv})
	if err != nil {
		t.Fatalf("marshal msmsg: %v", err)
	}
	node.Content = append(node.Content.([]waBinary.Node), encNode(EncTypeMsgSecret, payload))

	DecryptMessages(context.Background(), f, info, node)

	if len(f.acks) != 0 {
		t.Fatalf("acks = %v, queria nenhum", f.acks)
	}
	if f.msgReceipts != 1 {
		t.Fatalf("msgReceipts = %d, queria 1", f.msgReceipts)
	}
	var got *events.Message
	for _, e := range f.events {
		if m, ok := e.(*events.Message); ok {
			got = m
		}
	}
	if got == nil {
		t.Fatalf("nenhum events.Message despachado: %#v", f.events)
	}
	if got.Message.GetConversation() != "oi do bot" {
		t.Fatalf("conversa = %q", got.Message.GetConversation())
	}
	// O ID de mensagem tem que ter sido tirado da fila de pedido ao telefone.
	if len(f.cancelledRequests) != 1 || f.cancelledRequests[0] != "MSG1" {
		t.Errorf("cancelledRequests = %v", f.cancelledRequests)
	}
}

// Versao 3 no <enc> delega ao caminho armadillo, que continua na raiz.
func TestDecryptMessagesVersion3DelegatesToArmadillo(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret})
	f.armadilloProtoFailed = true
	botJID := types.NewJID("1234", types.BotServer)
	node := msgNode(waBinary.Attrs{"from": botJID, "id": "MSG1", "t": "1700000000"},
		waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{"target_id": "TARGET1"}},
	)
	info := testInfo(t, f, node)

	key, aad := GenerateSecretKey("", info.Sender, "MSG1", testOwnLID, ApplyBotMessageHKDF(testSecret))
	iv := make([]byte, msgSecretIVSize)
	ciphertext, err := gcmutil.Encrypt(key, iv, []byte("payload v3"), aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	payload, err := proto.Marshal(&waE2E.MessageSecretMessage{EncPayload: ciphertext, EncIV: iv})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	enc := encNode(EncTypeMsgSecret, payload)
	enc.Attrs[send.EncAttrVersion] = "3"
	node.Content = append(node.Content.([]waBinary.Node), enc)

	DecryptMessages(context.Background(), f, info, node)

	if len(f.acks) != 1 || f.acks[0] != testNackInvalidProtobuf {
		t.Fatalf("acks = %v, queria [%d]", f.acks, testNackInvalidProtobuf)
	}
}

// --- HandlePlaintext (newsletter) ---

func TestHandlePlaintext(t *testing.T) {
	f := newFakeTransport()
	body, err := proto.Marshal(&waE2E.Message{Conversation: proto.String("newsletter")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	node := msgNode(waBinary.Attrs{"from": types.NewJID("123", types.NewsletterServer)},
		waBinary.Node{Tag: "plaintext", Content: body},
		waBinary.Node{Tag: "meta", Attrs: waBinary.Attrs{
			"msg_edit_t": "1700000000000", "original_msg_t": "1700000000",
		}},
	)
	info := &types.MessageInfo{ID: "MSG1"}

	if failed := HandlePlaintext(context.Background(), f, info, node); failed {
		t.Fatalf("handlerFailed = true")
	}
	if len(f.events) != 1 {
		t.Fatalf("%d eventos", len(f.events))
	}
	evt := f.events[0].(*events.Message)
	if evt.Message.GetConversation() != "newsletter" {
		t.Errorf("conversa = %q", evt.Message.GetConversation())
	}
	if evt.NewsletterMeta == nil || evt.NewsletterMeta.EditTS.UnixMilli() != 1700000000000 {
		t.Errorf("NewsletterMeta = %+v", evt.NewsletterMeta)
	}
}

// Sem <plaintext>, sem conteudo em bytes ou com protobuf invalido: nada e'
// despachado, e nao pode entrar em panico.
func TestHandlePlaintextRejectsMalformed(t *testing.T) {
	cases := map[string]*waBinary.Node{
		"sem plaintext":     msgNode(waBinary.Attrs{}),
		"conteudo nao-byte": msgNode(waBinary.Attrs{}, waBinary.Node{Tag: "plaintext", Content: []waBinary.Node{{Tag: "x"}}}),
		"protobuf invalido": msgNode(waBinary.Attrs{}, waBinary.Node{Tag: "plaintext", Content: []byte{0xFF, 0xFF, 0xFF}}),
	}
	for name, node := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeTransport()
			if failed := HandlePlaintext(context.Background(), f, &types.MessageInfo{}, node); failed {
				t.Errorf("handlerFailed = true")
			}
			if len(f.events) != 0 {
				t.Fatalf("despachou %d eventos", len(f.events))
			}
		})
	}
}

// --- HandleEncrypted ---

// Um stanza que nem parseia (sem `id`/`t`) so' vira warning: nada de ack, nada
// de evento. Derrubar aqui perderia o resto da conexao.
func TestHandleEncryptedUnparseableIsIgnored(t *testing.T) {
	f := newFakeTransport()
	HandleEncrypted(context.Background(), f, msgNode(waBinary.Attrs{"from": testGroupJID, "participant": testOtherJID}))
	if len(f.events) != 0 || len(f.acks) != 0 {
		t.Fatalf("eventos=%d acks=%v", len(f.events), f.acks)
	}
}

// Push name e nome business sao atualizados em goroutine; o filtro de
// "username" so' vale para sessao Messenger.
func TestHandleEncryptedPushNameFilter(t *testing.T) {
	tests := []struct {
		name      string
		pushName  string
		messenger bool
		want      bool
	}{
		{"nome normal", "Fulano", false, true},
		{"hifen e' placeholder", "-", false, false},
		{"vazio", "", false, false},
		{"username em whatsapp e' nome", "username", false, true},
		{"username em messenger e' placeholder", "username", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTransport()
			f.msgr = tc.messenger
			node := msgNode(waBinary.Attrs{
				"from": types.NewJID("123", types.NewsletterServer),
				"id":   "MSG1", "t": "1700000000", "notify": tc.pushName,
			})
			HandleEncrypted(context.Background(), f, node)
			// A atualizacao roda em `go`; espera-a de forma limitada.
			deadline := time.Now().Add(250 * time.Millisecond)
			for time.Now().Before(deadline) {
				f.mu.Lock()
				n := len(f.pushNames)
				f.mu.Unlock()
				if n > 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			f.mu.Lock()
			got := len(f.pushNames) > 0
			f.mu.Unlock()
			if got != tc.want {
				t.Fatalf("atualizou push name = %v, queria %v", got, tc.want)
			}
		})
	}
}

// --- MigrateSessionStore / ClearUntrustedIdentity ---

func TestClearUntrustedIdentityDispatchesIdentityChange(t *testing.T) {
	f := newFakeTransport()
	if err := ClearUntrustedIdentity(context.Background(), f, testOtherJID); err != nil {
		t.Fatalf("erro: %v", err)
	}
	// O despacho e' em goroutine.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.events)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) != 1 {
		t.Fatalf("%d eventos", len(f.events))
	}
	evt, ok := f.events[0].(*events.IdentityChange)
	if !ok || evt.JID != testOtherJID || !evt.Implicit {
		t.Fatalf("evento = %#v", f.events[0])
	}
}

// A migracao de sessao PN->LID nunca aborta a decifragem: falha e' so' logada.
func TestMigrateSessionStoreDoesNotPanic(t *testing.T) {
	f := newFakeTransport()
	MigrateSessionStore(context.Background(), f, testOtherJID, testOwnLID)
}
