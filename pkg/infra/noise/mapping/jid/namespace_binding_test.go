package jid

import (
	"testing"

	"wa-api/pkg/domain"

	"wa-api/internal/noise/protocol/types"
)

// TestVocabularioCompartilhadoBateComAsConstantesReaisDoSocket amarra o
// vocabulário de domain às constantes que o SOCKET realmente usa.
//
// O ponto é a armadilha nº 1 do ARMADILHAS.md invertida: em vez de escrever
// "s.whatsapp.net" à mão no teste — o que só provaria que eu sei copiar uma
// string — ele lê a constante da produção. Se o servidor mudar lá, ou se
// alguém acrescentar um espaço de identidade novo que o mapa de domain não
// conheça, este teste acusa em vez de deixar o JID cair em Unknown em silêncio.
func TestVocabularioCompartilhadoBateComAsConstantesReaisDoSocket(t *testing.T) {
	casos := []struct {
		servidor string
		want     domain.Namespace
	}{
		{types.DefaultUserServer, domain.NamespacePhone},
		// c.us é LegacyUserServer AQUI e corrente na página. Ele não está
		// morto no socket: é o sufixo de consulta do USync
		// (internal/wa-noise/capabilities/user/info.go:46).
		{types.LegacyUserServer, domain.NamespacePhone},
		{types.HiddenUserServer, domain.NamespaceLID},
	}
	for _, c := range casos {
		got := domain.JID("5511999999999@" + c.servidor).Namespace()
		if got != c.want {
			t.Errorf("servidor %q: got %q, want %q", c.servidor, got, c.want)
		}
	}
}

// TestOResolvedorDoSocketProduzONamespaceDeTelefone fecha o círculo pela PORTA,
// e não pela constante: o que o adaptador devolve tem de ser classificável.
func TestOResolvedorDoSocketProduzONamespaceDeTelefone(t *testing.T) {
	var r JIDResolverAdapter
	got, err := r.ResolveJID(t.Context(), "+5511999999999")
	if err != nil {
		t.Fatalf("recusou o número com \"+\": %v", err)
	}
	if ns := got.Namespace(); ns != domain.NamespacePhone {
		t.Fatalf("ResolveJID devolveu %q, namespace %q, quero %q", string(got), ns, domain.NamespacePhone)
	}
}
