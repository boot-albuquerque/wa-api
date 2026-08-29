package headless

import (
	"strings"
	"testing"

	"wa-api/internal/headless"
	"wa-api/pkg/domain"
)

// TestVocabularioCompartilhadoBateComAsConstantesReaisDaPagina é o par do teste
// de mesmo nome em pkg/infra/wa-noise/mapping/jid: cada metade vive no pacote
// que legalmente enxerga as SUAS constantes, e as duas apontam para o mesmo
// vocabulário em domain.
//
// É também o primeiro consumidor da fachada, que é o que a decisão 71 pede: os
// sufixos vêm de wa-api/internal/headless e NÃO de spa/, porque alcançar o
// subpacote seria passar por trás da fronteira que a fachada existe para
// definir.
func TestVocabularioCompartilhadoBateComAsConstantesReaisDaPagina(t *testing.T) {
	casos := []struct {
		sufixo string
		want   domain.Namespace
	}{
		// c.us aqui é o namespace CORRENTE, não legado — o socket é que o
		// chama de LegacyUserServer. Esta linha e a irmã do lado do socket são
		// a evidência viva da decisão 74.
		{headless.ServerPhone, domain.NamespacePhone},
		{headless.ServerLID, domain.NamespaceLID},
		{headless.ServerGroup, domain.NamespaceGroup},
	}
	for _, c := range casos {
		if !strings.HasPrefix(c.sufixo, "@") {
			t.Fatalf("a fachada expôs %q sem o \"@\"; o teste assume a forma com arroba", c.sufixo)
		}
		got := domain.JID("5511999999999" + c.sufixo).Namespace()
		if got != c.want {
			t.Errorf("sufixo %q: got %q, want %q", c.sufixo, got, c.want)
		}
	}
}

// TestOsDoisTransportesConcordamNoSignificadoENaoNaGrafia é a asserção que a
// decisão 74 pediu, dita numa linha: o telefone da página e o telefone do
// socket são grafias DIFERENTES do MESMO namespace.
func TestOsDoisTransportesConcordamNoSignificadoENaoNaGrafia(t *testing.T) {
	pagina := domain.JID("5511999999999" + headless.ServerPhone)
	socket := domain.JID("5511999999999@s.whatsapp.net")

	if pagina == socket {
		t.Fatal("as duas grafias ficaram iguais; o teste deixou de medir a diferença que motivou a 74")
	}
	if pagina.Namespace() != socket.Namespace() {
		t.Fatalf("mesmo namespace esperado: página %q, socket %q", pagina.Namespace(), socket.Namespace())
	}
}

// TestFachadaExpoeARegraDeIdentidadeNaoResolvida prova que a decisão 66 chega
// ao adaptador pela fronteira certa, e não recopiada.
func TestFachadaExpoeARegraDeIdentidadeNaoResolvida(t *testing.T) {
	if !headless.IsUnresolvedIdentity("5511999999999" + headless.ServerPhone) {
		t.Error("identidade de telefone devia ser não-resolvida neste build")
	}
	if headless.IsUnresolvedIdentity("120363000000000" + headless.ServerGroup) {
		t.Error("grupo não é pessoa e não pode ser tratado como identidade não-resolvida")
	}
	if headless.IsUnresolvedIdentity("5511999999999" + headless.ServerLID) {
		t.Error("LID é o namespace que este build indexa; não é não-resolvido")
	}
}
