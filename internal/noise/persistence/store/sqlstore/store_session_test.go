package sqlstore

import (
	"bytes"
	"context"
	"testing"
)

func TestGetSessionMissingReturnsNilWithoutError(t *testing.T) {
	sess, err := newTestStore(t).GetSession(context.Background(), "ninguem:0")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess != nil {
		t.Fatalf("sessao inexistente deveria devolver nil, veio %q", sess)
	}
}

func TestPutGetHasSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	has, err := s.HasSession(ctx, "alice:0")
	if err != nil {
		t.Fatalf("HasSession antes: %v", err)
	}
	if has {
		t.Fatal("HasSession deveria ser falso antes do PutSession")
	}

	if err = s.PutSession(ctx, "alice:0", []byte("sessao-1")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}

	sess, err := s.GetSession(ctx, "alice:0")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !bytes.Equal(sess, []byte("sessao-1")) {
		t.Fatalf("GetSession devolveu %q", sess)
	}

	has, err = s.HasSession(ctx, "alice:0")
	if err != nil {
		t.Fatalf("HasSession depois: %v", err)
	}
	if !has {
		t.Fatal("HasSession deveria ser verdadeiro depois do PutSession")
	}
}

func TestPutSessionOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutSession(ctx, "bob:0", []byte("v1")); err != nil {
		t.Fatalf("PutSession v1: %v", err)
	}
	if err := s.PutSession(ctx, "bob:0", []byte("v2")); err != nil {
		t.Fatalf("PutSession v2: %v", err)
	}
	sess, err := s.GetSession(ctx, "bob:0")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !bytes.Equal(sess, []byte("v2")) {
		t.Fatalf("ON CONFLICT DO UPDATE nao substituiu: %q", sess)
	}
}

func TestGetManySessionsEmptyInput(t *testing.T) {
	result, err := newTestStore(t).GetManySessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetManySessions: %v", err)
	}
	if result != nil {
		t.Fatalf("entrada vazia deveria devolver nil, veio %v", result)
	}
}

// GetManySessions promete uma entrada no mapa para CADA endereco pedido,
// inclusive os que nao existem (com valor nil). Quem chama usa isso para
// distinguir "sem sessao" de "nao perguntei".
func TestGetManySessionsIncludesMissingAddressesAsNil(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutSession(ctx, "alice:0", []byte("a")); err != nil {
		t.Fatalf("PutSession alice: %v", err)
	}
	if err := s.PutSession(ctx, "bob:0", []byte("b")); err != nil {
		t.Fatalf("PutSession bob: %v", err)
	}

	result, err := s.GetManySessions(ctx, []string{"alice:0", "bob:0", "carol:0"})
	if err != nil {
		t.Fatalf("GetManySessions: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("esperava 3 entradas, veio %d: %v", len(result), result)
	}
	if !bytes.Equal(result["alice:0"], []byte("a")) {
		t.Fatalf("alice:0 = %q", result["alice:0"])
	}
	if !bytes.Equal(result["bob:0"], []byte("b")) {
		t.Fatalf("bob:0 = %q", result["bob:0"])
	}
	if got, ok := result["carol:0"]; !ok || got != nil {
		t.Fatalf("carol:0 deveria estar presente com nil, veio (%q, ok=%v)", got, ok)
	}
}

func TestPutManySessionsIsAtomicAndReadable(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	err := s.PutManySessions(ctx, map[string][]byte{
		"x:0": []byte("sx"),
		"y:0": []byte("sy"),
	})
	if err != nil {
		t.Fatalf("PutManySessions: %v", err)
	}
	for addr, want := range map[string]string{"x:0": "sx", "y:0": "sy"} {
		got, getErr := s.GetSession(ctx, addr)
		if getErr != nil {
			t.Fatalf("GetSession %s: %v", addr, getErr)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, esperava %q", addr, got, want)
		}
	}
}

func TestDeleteSessionRemovesOnlyThatAddress(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, addr := range []string{"dave:0", "dave:1"} {
		if err := s.PutSession(ctx, addr, []byte("s")); err != nil {
			t.Fatalf("PutSession %s: %v", addr, err)
		}
	}
	if err := s.DeleteSession(ctx, "dave:0"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if sess, err := s.GetSession(ctx, "dave:0"); err != nil || sess != nil {
		t.Fatalf("dave:0 deveria ter sumido (sess=%q err=%v)", sess, err)
	}
	if sess, err := s.GetSession(ctx, "dave:1"); err != nil || sess == nil {
		t.Fatalf("dave:1 deveria continuar (sess=%q err=%v)", sess, err)
	}
}

func TestDeleteAllSessionsRemovesEveryDeviceOfTheUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, addr := range []string{"erin:0", "erin:1", "frank:0"} {
		if err := s.PutSession(ctx, addr, []byte("s")); err != nil {
			t.Fatalf("PutSession %s: %v", addr, err)
		}
	}
	if err := s.DeleteAllSessions(ctx, "erin"); err != nil {
		t.Fatalf("DeleteAllSessions: %v", err)
	}
	for _, addr := range []string{"erin:0", "erin:1"} {
		if sess, err := s.GetSession(ctx, addr); err != nil || sess != nil {
			t.Fatalf("%s deveria ter sumido (sess=%q err=%v)", addr, sess, err)
		}
	}
	if sess, err := s.GetSession(ctx, "frank:0"); err != nil || sess == nil {
		t.Fatalf("frank:0 nao deveria ser afetado (sess=%q err=%v)", sess, err)
	}
}

func TestSessionsAreScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutSession(ctx, "grace:0", []byte("segredo")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	sess, err := other.GetSession(ctx, "grace:0")
	if err != nil {
		t.Fatalf("GetSession outro our_jid: %v", err)
	}
	if sess != nil {
		t.Fatalf("a sessao de um our_jid vazou para outro: %q", sess)
	}
}
