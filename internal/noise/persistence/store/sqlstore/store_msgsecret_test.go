package sqlstore

import (
	"bytes"
	"context"
	"testing"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/types"
)

func TestGetMessageSecretMissingReturnsNil(t *testing.T) {
	secret, _, err := newTestStore(t).GetMessageSecret(
		context.Background(), contactJID("1"), contactJID("1"), "MSGID",
	)
	if err != nil {
		t.Fatalf("GetMessageSecret: %v", err)
	}
	if secret != nil {
		t.Fatalf("segredo inexistente deveria devolver nil, veio %q", secret)
	}
}

func TestPutGetMessageSecretRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat, sender := contactJID("1"), contactJID("2")
	if err := s.PutMessageSecret(ctx, chat, sender, "MSGID", []byte("segredo")); err != nil {
		t.Fatalf("PutMessageSecret: %v", err)
	}
	secret, realSender, err := s.GetMessageSecret(ctx, chat, sender, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret: %v", err)
	}
	if !bytes.Equal(secret, []byte("segredo")) {
		t.Fatalf("segredo = %q", secret)
	}
	if realSender != sender {
		t.Fatalf("realSender = %s, esperava %s", realSender, sender)
	}
}

// ON CONFLICT DO NOTHING: o primeiro segredo gravado para uma mensagem manda.
// Reescrever quebraria a decriptacao de reacoes/comentarios ja' recebidos.
func TestPutMessageSecretDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat, sender := contactJID("1"), contactJID("2")
	if err := s.PutMessageSecret(ctx, chat, sender, "MSGID", []byte("primeiro")); err != nil {
		t.Fatalf("PutMessageSecret 1: %v", err)
	}
	if err := s.PutMessageSecret(ctx, chat, sender, "MSGID", []byte("segundo")); err != nil {
		t.Fatalf("PutMessageSecret 2: %v", err)
	}
	secret, _, err := s.GetMessageSecret(ctx, chat, sender, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret: %v", err)
	}
	if !bytes.Equal(secret, []byte("primeiro")) {
		t.Fatalf("DO NOTHING deveria preservar o primeiro segredo, veio %q", secret)
	}
}

// O JID e' normalizado com ToNonAD antes de gravar e de ler: o mesmo usuario em
// dispositivos diferentes compartilha o segredo da mensagem.
func TestMessageSecretIgnoresDeviceInJID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat := contactJID("1")
	senderDev0 := types.JID{User: "2", Device: 0, Server: types.DefaultUserServer}
	senderDev3 := types.JID{User: "2", Device: 3, Server: types.DefaultUserServer}

	if err := s.PutMessageSecret(ctx, chat, senderDev0, "MSGID", []byte("segredo")); err != nil {
		t.Fatalf("PutMessageSecret: %v", err)
	}
	secret, _, err := s.GetMessageSecret(ctx, chat, senderDev3, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret: %v", err)
	}
	if !bytes.Equal(secret, []byte("segredo")) {
		t.Fatalf("o device do JID nao deveria influenciar a busca, veio %q", secret)
	}
}

func TestPutMessageSecretsEmptyIsNoOp(t *testing.T) {
	if err := newTestStore(t).PutMessageSecrets(context.Background(), nil); err != nil {
		t.Fatalf("PutMessageSecrets vazio: %v", err)
	}
}

func TestPutMessageSecretsBatch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	chat, sender := contactJID("1"), contactJID("2")
	inserts := []store.MessageSecretInsert{
		{Chat: chat, Sender: sender, ID: "A", Secret: []byte("sa")},
		{Chat: chat, Sender: sender, ID: "B", Secret: []byte("sb")},
	}
	if err := s.PutMessageSecrets(ctx, inserts); err != nil {
		t.Fatalf("PutMessageSecrets: %v", err)
	}
	for _, ins := range inserts {
		got, _, err := s.GetMessageSecret(ctx, chat, sender, ins.ID)
		if err != nil {
			t.Fatalf("GetMessageSecret %s: %v", ins.ID, err)
		}
		if !bytes.Equal(got, ins.Secret) {
			t.Fatalf("%s = %q, esperava %q", ins.ID, got, ins.Secret)
		}
	}
}

// O SELECT tem um CASE que traduz LID <-> PN via wa-noise_lid_map. Um segredo
// gravado com o JID PN precisa ser encontrado quando o chat ja' migrou para
// LID, senao reacoes a mensagens antigas parariam de decifrar.
func TestGetMessageSecretResolvesLIDToPN(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)

	pn := types.JID{User: "5511555555555", Server: types.DefaultUserServer}
	lid := types.JID{User: "333333333333333", Server: types.HiddenUserServer}
	if err := container.LIDMap.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}

	if err := s.PutMessageSecret(ctx, pn, pn, "MSGID", []byte("segredo")); err != nil {
		t.Fatalf("PutMessageSecret: %v", err)
	}
	secret, _, err := s.GetMessageSecret(ctx, lid, lid, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret por LID: %v", err)
	}
	if !bytes.Equal(secret, []byte("segredo")) {
		t.Fatalf("o CASE de LID->PN nao encontrou o segredo, veio %q", secret)
	}
}

func TestGetMessageSecretResolvesPNToLID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)

	pn := types.JID{User: "5511555555555", Server: types.DefaultUserServer}
	lid := types.JID{User: "333333333333333", Server: types.HiddenUserServer}
	if err := container.LIDMap.PutLIDMapping(ctx, lid, pn); err != nil {
		t.Fatalf("PutLIDMapping: %v", err)
	}

	if err := s.PutMessageSecret(ctx, lid, lid, "MSGID", []byte("segredo")); err != nil {
		t.Fatalf("PutMessageSecret: %v", err)
	}
	secret, _, err := s.GetMessageSecret(ctx, pn, pn, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret por PN: %v", err)
	}
	if !bytes.Equal(secret, []byte("segredo")) {
		t.Fatalf("o CASE de PN->LID nao encontrou o segredo, veio %q", secret)
	}
}

func TestMessageSecretsAreScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	chat, sender := contactJID("1"), contactJID("2")
	if err := s.PutMessageSecret(ctx, chat, sender, "MSGID", []byte("segredo")); err != nil {
		t.Fatalf("PutMessageSecret: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	got, _, err := other.GetMessageSecret(ctx, chat, sender, "MSGID")
	if err != nil {
		t.Fatalf("GetMessageSecret outro our_jid: %v", err)
	}
	if got != nil {
		t.Fatalf("o segredo de um our_jid vazou para outro: %q", got)
	}
}
