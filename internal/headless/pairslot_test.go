package headless

import (
	"path/filepath"
	"strings"
	"testing"
)

// A guarda continua a recusar caminho livre; o que mudou e' que ha DOIS nomes
// descartaveis em vez de um. Medir se uma mensagem CHEGA exige os dois lados.
func TestOSlotDePareamentoESOMESSESDOIS(t *testing.T) {
	for _, bom := range []struct{ in, quer string }{
		{"", labProfileSlotA}, {"a", labProfileSlotA}, {"A", labProfileSlotA},
		{"b", labProfileSlotB}, {" B ", labProfileSlotB},
	} {
		got, err := pairingSlot(bom.in)
		if err != nil || got != bom.quer {
			t.Errorf("pairingSlot(%q) = %q,%v; queria %q", bom.in, got, err, bom.quer)
		}
	}
	// Um nome livre reabriria o risco que a guarda existe para impedir.
	for _, mau := range []string{"c", "../real", "/Users/x/perfil", "test-account-profile", "conta-A"} {
		if _, err := pairingSlot(mau); err == nil {
			t.Errorf("pairingSlot(%q) foi aceito", mau)
		}
	}
}

// O override de caminho continua RECUSADO, e a recusa e' erro e nao skip: um
// skip seria silencioso sobre um humano a espera de um QR que nunca vem.
func TestOverrideDeCaminhoContinuaRecusado(t *testing.T) {
	t.Setenv(profileDirOverride, "/tmp/qualquer")
	if _, err := pairingProfileDir(); err == nil {
		t.Fatal("um caminho livre foi aceito para PAREAR")
	}
}

// Os dois slots sao DIFERENTES e ambos ficam sob a raiz descartavel.
func TestOsDoisSlotsSaoDistintosESobARaizDescartavel(t *testing.T) {
	t.Setenv(profileDirOverride, "")
	t.Setenv(pairSlotEnv, "a")
	a, err := pairingProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(pairSlotEnv, "b")
	b, err := pairingProfileDir()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("os dois slots resolvem para o MESMO perfil: parear o segundo apagaria o primeiro")
	}
	raiz, _ := filepath.Abs(labProfileRoot)
	for _, p := range []string{a, b} {
		if !strings.HasPrefix(p, raiz+string(filepath.Separator)) {
			t.Errorf("%q esta fora da raiz descartavel %q", p, raiz)
		}
	}
}
