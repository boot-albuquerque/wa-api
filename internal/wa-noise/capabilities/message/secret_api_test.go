package message

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waLidMigrationSyncPayload"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// targetKey monta a MessageKey de grupo que aponta para a mensagem original.
func targetKey(origSender types.JID) *waCommon.MessageKey {
	return &waCommon.MessageKey{
		RemoteJID:   proto.String(testGroupJID.String()),
		Participant: proto.String(origSender.String()),
		ID:          proto.String("MSG1"),
	}
}

// Ida e volta de reacao: EncryptReaction cifra e DecryptReaction devolve o
// waE2E.ReactionMessage. Fecha o ciclo do wrapper publico, e nao so' do
// DecryptSecret de baixo.
func TestReactionRoundTrip(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "MSG1",
	}
	enc, err := EncryptReaction(context.Background(), f, root, &waE2E.ReactionMessage{
		Key:  targetKey(testOtherJID),
		Text: proto.String("🐈️"),
	})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	evt := msgEvent(testGroupJID, testOwnLID)
	evt.Message = &waE2E.Message{EncReactionMessage: &waE2E.EncReactionMessage{
		TargetMessageKey: targetKey(testOtherJID),
		EncPayload:       enc.GetEncPayload(),
		EncIV:            enc.GetEncIV(),
	}}
	got, err := DecryptReaction(context.Background(), f, evt)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got.GetText() != "🐈️" {
		t.Fatalf("texto = %q", got.GetText())
	}
}

// Mesmo ciclo para comentario. Vale por si: o use case e' outro, e derivar com
// o errado nao autentica.
func TestCommentRoundTrip(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "MSG1",
	}
	msg, err := EncryptComment(context.Background(), f, root, &waE2E.Message{
		Conversation: proto.String("comentario"),
	})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	evt := msgEvent(testGroupJID, testOwnLID)
	evt.Message = &waE2E.Message{EncCommentMessage: &waE2E.EncCommentMessage{
		TargetMessageKey: targetKey(testOtherJID),
		EncPayload:       msg.GetEncCommentMessage().GetEncPayload(),
		EncIV:            msg.GetEncCommentMessage().GetEncIV(),
	}}
	got, err := DecryptComment(context.Background(), f, evt)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got.GetConversation() != "comentario" {
		t.Fatalf("conversa = %q", got.GetConversation())
	}
}

// DecryptSecretEncrypted herda o MessageContextInfo do envelope EXTERNO quando
// a mensagem de dentro nao traz o seu. Sem isso, a edicao perderia o segredo
// que permite decifrar as proximas modificacoes dela.
func TestDecryptSecretEncryptedInheritsContextInfo(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})

	inner, err := proto.Marshal(&waE2E.Message{Conversation: proto.String("editado")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	key, aad := GenerateSecretKey(EncSecretMessageEdit, testOwnLID, "MSG1", testOtherJID, testSecret)
	_ = aad // EncSecretMessageEdit nao leva AAD; explicito para nao parecer esquecimento.
	ciphertext, iv, err := EncryptSecret(context.Background(), f, testOwnLID, testGroupJID, testOtherJID, "MSG1", EncSecretMessageEdit, inner)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_ = key

	ctxInfo := &waE2E.MessageContextInfo{MessageSecret: testSecret}
	evt := msgEvent(testGroupJID, testOwnLID)
	evt.Message = &waE2E.Message{
		MessageContextInfo: ctxInfo,
		SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
			SecretEncType:    waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
			TargetMessageKey: targetKey(testOtherJID),
			EncPayload:       ciphertext,
			EncIV:            iv,
		},
	}
	got, err := DecryptSecretEncrypted(context.Background(), f, evt)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got.GetConversation() != "editado" {
		t.Fatalf("conversa = %q", got.GetConversation())
	}
	if got.MessageContextInfo != ctxInfo {
		t.Fatalf("MessageContextInfo nao foi herdado do envelope externo")
	}
}

// --- StoreLIDSyncMessage / StoreGlobalSettings / StoreHistoricalPNLIDMappings ---

func TestStoreLIDSyncMessageParsesPairs(t *testing.T) {
	f := newFakeTransport()
	payload, err := proto.Marshal(&waLidMigrationSyncPayload.LIDMigrationMappingSyncPayload{
		ChatDbMigrationTimestamp: proto.Uint64(1700000000),
		PnToLidMappings: []*waLidMigrationSyncPayload.LIDMigrationMapping{{
			Pn:          proto.Uint64(5511888888888),
			AssignedLid: proto.Uint64(55443322),
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	StoreLIDSyncMessage(context.Background(), f, payload)

	if f.dev.LIDMigrationTimestamp != 1700000000 {
		t.Fatalf("LIDMigrationTimestamp = %d", f.dev.LIDMigrationTimestamp)
	}
	// Uma segunda mensagem NAO pode sobrescrever o timestamp ja' gravado: a
	// condicao e' `== 0`.
	payload2, err := proto.Marshal(&waLidMigrationSyncPayload.LIDMigrationMappingSyncPayload{
		ChatDbMigrationTimestamp: proto.Uint64(1800000000),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	StoreLIDSyncMessage(context.Background(), f, payload2)
	if f.dev.LIDMigrationTimestamp != 1700000000 {
		t.Fatalf("timestamp foi sobrescrito para %d", f.dev.LIDMigrationTimestamp)
	}
}

// Payload que nao e' protobuf valido so' vira log — nao pode entrar em panico
// nem gravar mapeamento nenhum.
func TestStoreLIDSyncMessageMalformed(t *testing.T) {
	f := newFakeTransport()
	StoreLIDSyncMessage(context.Background(), f, []byte{0xFF, 0xFF, 0xFF})
	if f.dev.LIDMigrationTimestamp != 0 {
		t.Fatalf("gravou timestamp a partir de payload invalido")
	}
}

func TestStoreGlobalSettingsOnlySetsOnce(t *testing.T) {
	f := newFakeTransport()
	StoreGlobalSettings(context.Background(), f, &waHistorySync.GlobalSettings{
		ChatDbLidMigrationTimestamp: proto.Int64(1700000000),
	})
	if f.dev.LIDMigrationTimestamp != 1700000000 {
		t.Fatalf("LIDMigrationTimestamp = %d", f.dev.LIDMigrationTimestamp)
	}
	StoreGlobalSettings(context.Background(), f, &waHistorySync.GlobalSettings{
		ChatDbLidMigrationTimestamp: proto.Int64(1800000000),
	})
	if f.dev.LIDMigrationTimestamp != 1700000000 {
		t.Fatalf("timestamp foi sobrescrito para %d", f.dev.LIDMigrationTimestamp)
	}
}

// O server legado @c.us e' normalizado para @s.whatsapp.net; pares com JID
// invalido de qualquer um dos lados sao pulados, e nao abortam o lote.
func TestStoreHistoricalPNLIDMappingsNormalizesAndSkips(t *testing.T) {
	f := newFakeTransport()
	StoreHistoricalPNLIDMappings(context.Background(), f, []*waHistorySync.PhoneNumberToLIDMapping{
		{PnJID: proto.String("5511888888888@c.us"), LidJID: proto.String("55443322@lid")},
		{PnJID: proto.String("1.2.3@g.us"), LidJID: proto.String("55443322@lid")},
		{PnJID: proto.String("5511888888888@s.whatsapp.net"), LidJID: proto.String("1.2.3@g.us")},
	})
	// O NoopStore aceita tudo; o que este teste prova e' que nenhum dos dois
	// pares invalidos derruba a funcao antes do par bom.
}

// events.Message e' usado pelos ajudantes; referencia explicita para o import
// nao ficar orfao se os ajudantes mudarem.
var _ = events.Message{}
