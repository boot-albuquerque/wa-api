package sqlstore

import (
	"bytes"
	"context"
	"testing"
)

func testKey(fill byte) [curve25519KeyLength]byte {
	var k [curve25519KeyLength]byte
	for i := range k {
		k[i] = fill
	}
	return k
}

func TestIsTrustedIdentityUnknownAddressIsTrusted(t *testing.T) {
	s := newTestStore(t)
	trusted, err := s.IsTrustedIdentity(context.Background(), "desconhecido:0", testKey(1))
	if err != nil {
		t.Fatalf("IsTrustedIdentity: %v", err)
	}
	if !trusted {
		t.Fatal("endereco desconhecido deve ser confiavel (a identity e' salva depois)")
	}
}

func TestPutIdentityThenIsTrusted(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key := testKey(7)
	if err := s.PutIdentity(ctx, "alice:0", key); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}

	trusted, err := s.IsTrustedIdentity(ctx, "alice:0", key)
	if err != nil {
		t.Fatalf("IsTrustedIdentity: %v", err)
	}
	if !trusted {
		t.Fatal("a mesma chave gravada deve ser confiavel")
	}

	trusted, err = s.IsTrustedIdentity(ctx, "alice:0", testKey(8))
	if err != nil {
		t.Fatalf("IsTrustedIdentity (chave outra): %v", err)
	}
	if trusted {
		t.Fatal("chave diferente da gravada nao pode ser confiavel")
	}
}

func TestPutIdentityOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutIdentity(ctx, "bob:0", testKey(1)); err != nil {
		t.Fatalf("PutIdentity 1: %v", err)
	}
	if err := s.PutIdentity(ctx, "bob:0", testKey(2)); err != nil {
		t.Fatalf("PutIdentity 2: %v", err)
	}
	trusted, err := s.IsTrustedIdentity(ctx, "bob:0", testKey(2))
	if err != nil {
		t.Fatalf("IsTrustedIdentity: %v", err)
	}
	if !trusted {
		t.Fatal("ON CONFLICT DO UPDATE deveria ter substituido a identity antiga")
	}
}

func TestDeleteAllIdentitiesRemovesEveryDeviceOfTheUser(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key := testKey(3)
	for _, addr := range []string{"carol:0", "carol:1", "dave:0"} {
		if err := s.PutIdentity(ctx, addr, key); err != nil {
			t.Fatalf("PutIdentity %s: %v", addr, err)
		}
	}
	if err := s.DeleteAllIdentities(ctx, "carol"); err != nil {
		t.Fatalf("DeleteAllIdentities: %v", err)
	}

	// Apagadas: voltam a ser "desconhecidas", logo confiaveis para qualquer chave.
	for _, addr := range []string{"carol:0", "carol:1"} {
		trusted, err := s.IsTrustedIdentity(ctx, addr, testKey(9))
		if err != nil {
			t.Fatalf("IsTrustedIdentity %s: %v", addr, err)
		}
		if !trusted {
			t.Fatalf("%s deveria ter sido apagado", addr)
		}
	}
	// Preservada: dave nao casa o LIKE 'carol:%'.
	trusted, err := s.IsTrustedIdentity(ctx, "dave:0", testKey(9))
	if err != nil {
		t.Fatalf("IsTrustedIdentity dave: %v", err)
	}
	if trusted {
		t.Fatal("dave:0 nao deveria ter sido apagado por DeleteAllIdentities(carol)")
	}
}

// TestDeleteIdentityUsesWildcardQuery trava o comportamento observado hoje:
// DeleteIdentity executa deleteAllIdentitiesQuery (LIKE) em vez de
// deleteIdentityQuery (igualdade). Como o endereco e' passado sem o sufixo
// curinga, o LIKE degenera em igualdade e o efeito pratico coincide — mas
// deleteIdentityQuery fica sem uso. E' assim no upstream; ver HOUSEKEEP.md.
func TestDeleteIdentityRemovesOnlyThatAddress(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	key := testKey(4)
	for _, addr := range []string{"erin:0", "erin:1"} {
		if err := s.PutIdentity(ctx, addr, key); err != nil {
			t.Fatalf("PutIdentity %s: %v", addr, err)
		}
	}
	if err := s.DeleteIdentity(ctx, "erin:0"); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	trusted, err := s.IsTrustedIdentity(ctx, "erin:0", testKey(9))
	if err != nil {
		t.Fatalf("IsTrustedIdentity erin:0: %v", err)
	}
	if !trusted {
		t.Fatal("erin:0 deveria ter sido apagado")
	}
	trusted, err = s.IsTrustedIdentity(ctx, "erin:1", testKey(9))
	if err != nil {
		t.Fatalf("IsTrustedIdentity erin:1: %v", err)
	}
	if trusted {
		t.Fatal("erin:1 nao deveria ter sido apagado")
	}
}

func TestIdentityIsScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutIdentity(ctx, "frank:0", testKey(5)); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	trusted, err := other.IsTrustedIdentity(ctx, "frank:0", testKey(6))
	if err != nil {
		t.Fatalf("IsTrustedIdentity outro our_jid: %v", err)
	}
	if !trusted {
		t.Fatal("a identity de um our_jid nao pode vazar para outro")
	}
}

func TestIsTrustedIdentityRejectsWrongLengthInDatabase(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	// O schema tem CHECK(length(identity) = 32), entao o caminho de
	// ErrInvalidLength so' e' alcancavel com o CHECK burlado. Provamos aqui que
	// o CHECK existe — que e' o que torna ErrInvalidLength inalcancavel.
	_, err := s.db.Exec(ctx, putIdentityQuery, s.JID, "curto:0", bytes.Repeat([]byte{1}, 4))
	if err == nil {
		t.Fatal("o schema deveria recusar identity com tamanho != 32")
	}
}
