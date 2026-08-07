package message

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	gcmutil "wa-api/internal/wa-noise/security/gcm"
)

// --- guardas de nil / pre-condicoes ---

func TestEncryptMsgSecretNotLoggedIn(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret})
	_, _, err := EncryptSecret(context.Background(), f, types.EmptyJID, testGroupJID, testOtherJID, "MSG1", EncSecretReaction, []byte("oi"))
	if !errors.Is(err, errNotLoggedIn) {
		t.Fatalf("err = %v, want errNotLoggedIn", err)
	}
}

func TestMsgSecretNotFound(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: nil})
	if _, _, err := EncryptSecret(context.Background(), f, testOwnLID, testGroupJID, testOtherJID, "MSG1", EncSecretReaction, []byte("oi")); !errors.Is(err, ErrOriginalMessageSecretNotFound) {
		t.Errorf("encryptMsgSecret: err = %v", err)
	}

	evt := msgEvent(testGroupJID, testOwnLID)
	_, err := DecryptSecret(context.Background(), f, evt, EncSecretReaction, &waE2E.EncReactionMessage{}, &waCommon.MessageKey{FromMe: proto.Bool(true)})
	if !errors.Is(err, ErrOriginalMessageSecretNotFound) {
		t.Errorf("decryptMsgSecret: err = %v", err)
	}
}

func TestMsgSecretStoreError(t *testing.T) {
	sentinel := errors.New("boom")
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{err: sentinel})
	if _, _, err := EncryptSecret(context.Background(), f, testOwnLID, testGroupJID, testOtherJID, "MSG1", EncSecretReaction, []byte("oi")); !errors.Is(err, sentinel) {
		t.Errorf("encryptMsgSecret: err = %v", err)
	}
}

// --- ida e volta real de criptografia ---

// encryptMsgSecret e decryptMsgSecret tem que fechar o ciclo com AES-GCM de
// verdade: chave derivada, IV de msgSecretIVSize bytes e o mesmo AAD dos dois
// lados.
func TestMsgSecretRoundTrip(t *testing.T) {
	origSender := testOtherJID
	stub := &stubMsgSecretStore{secret: testSecret, origSender: origSender}
	f := newFakeTransport().withSecrets(stub)
	ownID := testOwnLID
	plaintext := []byte("mensagem secreta")

	for _, useCase := range []SecretType{EncSecretReaction, EncSecretPollVote, EncSecretComment} {
		t.Run(string(useCase), func(t *testing.T) {
			ciphertext, iv, err := EncryptSecret(context.Background(), f, ownID, testGroupJID, origSender, "MSG1", useCase, plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if len(iv) != msgSecretIVSize {
				t.Fatalf("len(iv) = %d, queria %d", len(iv), msgSecretIVSize)
			}
			if bytes.Contains(ciphertext, plaintext) {
				t.Fatal("o texto claro aparece no ciphertext")
			}

			evt := msgEvent(testGroupJID, ownID)
			got, err := DecryptSecret(context.Background(), f, evt, useCase, &waE2E.EncReactionMessage{
				EncPayload: ciphertext,
				EncIV:      iv,
			}, &waCommon.MessageKey{
				RemoteJID:   proto.String(testGroupJID.String()),
				Participant: proto.String(origSender.String()),
				ID:          proto.String("MSG1"),
			})
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Fatalf("got %q, want %q", got, plaintext)
			}
		})
	}
}

// Decriptar com o use case errado tem que falhar: os tipos sao separados por
// dominio justamente para que uma reacao nao possa ser lida como voto.
func TestMsgSecretWrongUseCaseFails(t *testing.T) {
	origSender := testOtherJID
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: origSender})
	ownID := testOwnLID

	ciphertext, iv, err := EncryptSecret(context.Background(), f, ownID, testGroupJID, origSender, "MSG1", EncSecretReaction, []byte("oi"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	evt := msgEvent(testGroupJID, ownID)
	if _, err = DecryptSecret(context.Background(), f, evt, EncSecretComment, &waE2E.EncReactionMessage{
		EncPayload: ciphertext, EncIV: iv,
	}, &waCommon.MessageKey{
		RemoteJID:   proto.String(testGroupJID.String()),
		Participant: proto.String(origSender.String()),
		ID:          proto.String("MSG1"),
	}); err == nil {
		t.Fatal("decriptou com o use case errado")
	}
}

// Hack de migracao LID: quando o remetente original deduzido da chave difere do
// que estava gravado junto do segredo, o decrypt refaz a derivacao com o
// gravado antes de desistir. Sem isso, mensagens da transicao PN->LID ficam
// permanentemente ilegiveis.
func TestMsgSecretFallsBackToStoredOrigSender(t *testing.T) {
	storedSender := types.NewJID("55443322", types.HiddenUserServer)
	keySender := testOtherJID
	stub := &stubMsgSecretStore{secret: testSecret, origSender: storedSender}
	f := newFakeTransport().withSecrets(stub)
	ownID := testOwnLID

	// Cifra usando o remetente *gravado*...
	ciphertext, iv, err := EncryptSecret(context.Background(), f, ownID, testGroupJID, storedSender, "MSG1", EncSecretReaction, []byte("oi"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// ...e decripta com uma chave que aponta para o *outro* JID do mesmo usuario.
	evt := msgEvent(testGroupJID, ownID)
	got, err := DecryptSecret(context.Background(), f, evt, EncSecretReaction, &waE2E.EncReactionMessage{
		EncPayload: ciphertext, EncIV: iv,
	}, &waCommon.MessageKey{
		RemoteJID:   proto.String(testGroupJID.String()),
		Participant: proto.String(keySender.String()),
		ID:          proto.String("MSG1"),
	})
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != "oi" {
		t.Fatalf("got %q", got)
	}
}

// --- decryptBotMessage ---

// A mensagem de bot usa o segredo passado por HKDF adicional e use case vazio
// (que leva AAD). Cifrar a mao com a mesma derivacao tem que fechar.
func TestDecryptBotMessageRoundTrip(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	target := testOwnLID
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Sender: types.NewJID("1234", types.BotServer)},
		ID:            "MSG1",
	}
	plaintext := []byte("resposta do bot")

	key, aad := GenerateSecretKey("", info.Sender, "MSG1", target, ApplyBotMessageHKDF(testSecret))
	iv := bytes.Repeat([]byte{7}, msgSecretIVSize)
	ciphertext, err := gcmutil.Encrypt(key, iv, plaintext, aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := DecryptBotMessage(f, testSecret, &waE2E.MessageSecretMessage{
		EncPayload: ciphertext, EncIV: iv,
	}, "MSG1", target, info)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestDecryptBotMessageWrongSecret(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{})
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Sender: types.NewJID("1234", types.BotServer)},
		ID:            "MSG1",
	}
	_, err := DecryptBotMessage(f, testSecret, &waE2E.MessageSecretMessage{
		EncPayload: bytes.Repeat([]byte{0}, 32),
		EncIV:      bytes.Repeat([]byte{0}, msgSecretIVSize),
	}, "MSG1", testOwnLID, info)
	if err == nil {
		t.Fatal("esperava falha de autenticacao")
	}
}

// --- guardas de tipo dos wrappers publicos ---

func TestDecryptWrappersRejectWrongMessageType(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret})
	empty := &events.Message{Message: &waE2E.Message{}}

	if _, err := DecryptReaction(context.Background(), f, empty); !errors.Is(err, ErrNotEncryptedReactionMessage) {
		t.Errorf("DecryptReaction: err = %v", err)
	}
	if _, err := DecryptComment(context.Background(), f, empty); !errors.Is(err, ErrNotEncryptedCommentMessage) {
		t.Errorf("DecryptComment: err = %v", err)
	}
	if _, err := DecryptSecretEncrypted(context.Background(), f, empty); !errors.Is(err, ErrNotSecretEncryptedMessage) {
		t.Errorf("DecryptSecretEncryptedMessage: err = %v", err)
	}
	if _, err := DecryptPollVote(context.Background(), f, empty); !errors.Is(err, ErrNotPollUpdateMessage) {
		t.Errorf("DecryptPollVote: err = %v", err)
	}
}

// O mapa de SecretEncType -> MsgSecretType e' o que decide qual chave derivar;
// um tipo desconhecido tem que virar erro em vez de derivar com use case vazio
// (que e' o do bot, e traz AAD).
func TestDecryptSecretEncryptedMessageTypeMapping(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: nil})

	tests := []struct {
		encType waE2E.SecretEncryptedMessage_SecretEncType
		want    error
	}{
		{waE2E.SecretEncryptedMessage_EVENT_EDIT, ErrOriginalMessageSecretNotFound},
		{waE2E.SecretEncryptedMessage_POLL_EDIT, ErrOriginalMessageSecretNotFound},
		{waE2E.SecretEncryptedMessage_POLL_ADD_OPTION, ErrOriginalMessageSecretNotFound},
		{waE2E.SecretEncryptedMessage_MESSAGE_EDIT, ErrOriginalMessageSecretNotFound},
		{waE2E.SecretEncryptedMessage_UNKNOWN, nil},
		// MESSAGE_SCHEDULE existe no enum mas nao esta' mapeado no switch;
		// travar isso aqui deixa explicito que e' uma lacuna conhecida e nao
		// um caso esquecido que "funciona por acaso".
		{waE2E.SecretEncryptedMessage_MESSAGE_SCHEDULE, nil},
	}
	for _, tc := range tests {
		evt := &events.Message{
			Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID}},
			Message: &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
				SecretEncType:    tc.encType.Enum(),
				TargetMessageKey: &waCommon.MessageKey{FromMe: proto.Bool(true)},
			}},
		}
		_, err := DecryptSecretEncrypted(context.Background(), f, evt)
		if tc.want == nil {
			// tipo desconhecido: erro proprio, antes de tocar no store
			if err == nil || errors.Is(err, ErrOriginalMessageSecretNotFound) {
				t.Errorf("%s: err = %v, queria \"unsupported secret enc type\"", tc.encType, err)
			}
		} else if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.encType, err, tc.want)
		}
	}
}

// --- EncryptComment / EncryptReaction ---

func TestEncryptCommentShape(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})
	sender := testOtherJID
	sender.Device = 4
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: sender, IsGroup: true},
		ID:            "MSG1",
	}

	msg, err := EncryptComment(context.Background(), f, root, &waE2E.Message{Conversation: proto.String("comentario")})
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	enc := msg.GetEncCommentMessage()
	if len(enc.GetEncPayload()) == 0 || len(enc.GetEncIV()) != msgSecretIVSize {
		t.Fatalf("payload/iv = %d/%d", len(enc.GetEncPayload()), len(enc.GetEncIV()))
	}
	// O participante da chave alvo vai sem device.
	if got := enc.GetTargetMessageKey().GetParticipant(); got != testOtherJID.ToNonAD().String() {
		t.Errorf("Participant = %q, queria %q", got, testOtherJID.ToNonAD().String())
	}
	if enc.GetTargetMessageKey().GetID() != "MSG1" {
		t.Errorf("ID = %q", enc.GetTargetMessageKey().GetID())
	}
}

// Correcao do lote 9: EncryptReaction zera reaction.Key para tirar a chave do
// payload cifrado, mas a struct e' do chamador. Sem restaurar, reusar a mesma
// reacao produzia uma reacao sem alvo.
func TestEncryptReactionRestoresCallerKey(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: testSecret, origSender: testOtherJID})
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "MSG1",
	}
	key := &waCommon.MessageKey{ID: proto.String("MSG1"), FromMe: proto.Bool(false)}
	reaction := &waE2E.ReactionMessage{Key: key, Text: proto.String("🐈️")}

	enc, err := EncryptReaction(context.Background(), f, root, reaction)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if reaction.Key != key {
		t.Fatalf("reaction.Key = %v, queria a chave original de volta", reaction.Key)
	}
	if enc.GetTargetMessageKey() != key {
		t.Errorf("TargetMessageKey nao e' a chave original")
	}
	if len(enc.GetEncIV()) != msgSecretIVSize {
		t.Errorf("len(iv) = %d", len(enc.GetEncIV()))
	}
}

// A chave tem que voltar tambem quando a criptografia falha — o erro nao pode
// deixar a struct do chamador mutilada.
func TestEncryptReactionRestoresCallerKeyOnError(t *testing.T) {
	f := newFakeTransport().withSecrets(&stubMsgSecretStore{secret: nil})
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "MSG1",
	}
	key := &waCommon.MessageKey{ID: proto.String("MSG1")}
	reaction := &waE2E.ReactionMessage{Key: key, Text: proto.String("🐈️")}

	if _, err := EncryptReaction(context.Background(), f, root, reaction); err == nil {
		t.Fatal("esperava erro")
	}
	if reaction.Key != key {
		t.Fatalf("reaction.Key = %v apos erro, queria a chave original", reaction.Key)
	}
}

// A chave nao pode entrar no payload cifrado: ela viaja em claro no
// TargetMessageKey, e duplica-la vazaria o alvo dentro do texto autenticado.
func TestEncryptReactionExcludesKeyFromPayload(t *testing.T) {
	stub := &stubMsgSecretStore{secret: testSecret, origSender: testOtherJID}
	f := newFakeTransport().withSecrets(stub)
	root := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: testGroupJID, Sender: testOtherJID, IsGroup: true},
		ID:            "MSG1",
	}
	reaction := &waE2E.ReactionMessage{
		Key:  &waCommon.MessageKey{ID: proto.String("MSG1")},
		Text: proto.String("🐈️"),
	}
	enc, err := EncryptReaction(context.Background(), f, root, reaction)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}

	evt := msgEvent(testGroupJID, testOwnLID)
	plaintext, err := DecryptSecret(context.Background(), f, evt, EncSecretReaction, &waE2E.EncReactionMessage{
		EncPayload: enc.GetEncPayload(), EncIV: enc.GetEncIV(),
	}, &waCommon.MessageKey{
		RemoteJID:   proto.String(testGroupJID.String()),
		Participant: proto.String(testOtherJID.String()),
		ID:          proto.String("MSG1"),
	})
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var decoded waE2E.ReactionMessage
	if err = proto.Unmarshal(plaintext, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Key != nil {
		t.Errorf("Key presente no payload cifrado: %v", decoded.Key)
	}
	if decoded.GetText() != "🐈️" {
		t.Errorf("texto = %q", decoded.GetText())
	}
}
