package domain_test

import (
	"strings"
	"testing"

	"wa-api/pkg/domain"
)

// TestHashTokenKnownVector fixa o algoritmo, não só o comportamento. Três
// camadas independentes (migração, autenticação, use cases de usuário) têm
// de produzir exatamente este valor para o mesmo token; se alguém trocar
// SHA-256 por outra coisa, ou passar a salgar, os tokens já gravados param de
// autenticar e o único aviso seria produção. O vetor é o SHA-256 de "abc".
func TestHashTokenKnownVector(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := domain.HashToken("abc"); got != want {
		t.Errorf("HashToken(%q) = %q, want %q", "abc", got, want)
	}
}

func TestHashTokenIsDeterministicAndInjective(t *testing.T) {
	first := domain.HashToken("some-token")
	if second := domain.HashToken("some-token"); first != second {
		t.Errorf("HashToken is not deterministic: %q then %q", first, second)
	}
	if other := domain.HashToken("some-token "); other == first {
		t.Error("HashToken collided on inputs differing by a trailing space")
	}
}

// TestHashTokenShapeAndNonLeak: o resultado é chave de lookup e coluna
// UNIQUE, então precisa ser hex de largura fixa; e não pode conter o token,
// porque a coluna existe justamente para o banco não guardar o segredo.
func TestHashTokenShapeAndNonLeak(t *testing.T) {
	const token = "hunter2-secret-token"
	got := domain.HashToken(token)

	if len(got) != 64 {
		t.Errorf("len(HashToken(...)) = %d, want 64 hex chars for SHA-256", len(got))
	}
	if strings.ContainsAny(got, "GHIJKLMNOPQRSTUVWXYZ") || strings.ToLower(got) != got {
		t.Errorf("HashToken returned non-lowercase-hex output: %q", got)
	}
	if strings.Contains(got, token) {
		t.Errorf("HashToken output contains the raw token: %q", got)
	}
	if empty := domain.HashToken(""); empty == got || len(empty) != 64 {
		t.Errorf("HashToken(\"\") = %q, want a distinct 64-char digest", empty)
	}
}
