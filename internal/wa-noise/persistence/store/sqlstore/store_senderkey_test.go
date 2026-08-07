package sqlstore

import (
	"bytes"
	"context"
	"testing"
)

func TestGetSenderKeyMissingReturnsNilWithoutError(t *testing.T) {
	key, err := newTestStore(t).GetSenderKey(context.Background(), "grupo@g.us", "alice:0")
	if err != nil {
		t.Fatalf("GetSenderKey: %v", err)
	}
	if key != nil {
		t.Fatalf("sender key inexistente deveria devolver nil, veio %q", key)
	}
}

func TestPutGetSenderKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutSenderKey(ctx, "grupo@g.us", "alice:0", []byte("sk-1")); err != nil {
		t.Fatalf("PutSenderKey: %v", err)
	}
	key, err := s.GetSenderKey(ctx, "grupo@g.us", "alice:0")
	if err != nil {
		t.Fatalf("GetSenderKey: %v", err)
	}
	if !bytes.Equal(key, []byte("sk-1")) {
		t.Fatalf("GetSenderKey = %q", key)
	}
}

func TestPutSenderKeyOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutSenderKey(ctx, "grupo@g.us", "alice:0", []byte("v1")); err != nil {
		t.Fatalf("PutSenderKey v1: %v", err)
	}
	if err := s.PutSenderKey(ctx, "grupo@g.us", "alice:0", []byte("v2")); err != nil {
		t.Fatalf("PutSenderKey v2: %v", err)
	}
	key, err := s.GetSenderKey(ctx, "grupo@g.us", "alice:0")
	if err != nil {
		t.Fatalf("GetSenderKey: %v", err)
	}
	if !bytes.Equal(key, []byte("v2")) {
		t.Fatalf("ON CONFLICT DO UPDATE nao substituiu: %q", key)
	}
}

// A chave da tabela e' (our_jid, chat_id, sender_id): o mesmo remetente em dois
// grupos tem duas sender keys distintas, e dois remetentes no mesmo grupo
// tambem.
func TestSenderKeyIsScopedPerChatAndSender(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutSenderKey(ctx, "g1@g.us", "alice:0", []byte("g1-alice")); err != nil {
		t.Fatalf("PutSenderKey g1/alice: %v", err)
	}
	if err := s.PutSenderKey(ctx, "g2@g.us", "alice:0", []byte("g2-alice")); err != nil {
		t.Fatalf("PutSenderKey g2/alice: %v", err)
	}
	if err := s.PutSenderKey(ctx, "g1@g.us", "bob:0", []byte("g1-bob")); err != nil {
		t.Fatalf("PutSenderKey g1/bob: %v", err)
	}

	for _, tc := range []struct{ chat, sender, want string }{
		{"g1@g.us", "alice:0", "g1-alice"},
		{"g2@g.us", "alice:0", "g2-alice"},
		{"g1@g.us", "bob:0", "g1-bob"},
	} {
		got, err := s.GetSenderKey(ctx, tc.chat, tc.sender)
		if err != nil {
			t.Fatalf("GetSenderKey %s/%s: %v", tc.chat, tc.sender, err)
		}
		if string(got) != tc.want {
			t.Fatalf("%s/%s = %q, esperava %q", tc.chat, tc.sender, got, tc.want)
		}
	}
}

func TestSenderKeyIsScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutSenderKey(ctx, "g1@g.us", "alice:0", []byte("segredo")); err != nil {
		t.Fatalf("PutSenderKey: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	key, err := other.GetSenderKey(ctx, "g1@g.us", "alice:0")
	if err != nil {
		t.Fatalf("GetSenderKey outro our_jid: %v", err)
	}
	if key != nil {
		t.Fatalf("a sender key de um our_jid vazou para outro: %q", key)
	}
}
