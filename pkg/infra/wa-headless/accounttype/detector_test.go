package accounttype

import (
	"context"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// TestSatisfazAccountTypeDetector garante que o adapter satisfaz a porta que
// o usecase de deteccao vai consumir.
func TestSatisfazAccountTypeDetector(t *testing.T) {
	var d any = NewDetector(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := d.(appport.AccountTypeDetector); !ok {
		t.Fatal("não satisfaz appport.AccountTypeDetector")
	}
}

// TestDetect_NoSessionNoBoot: sem sessao registrada, Detect nao tenta ler a
// pagina e devolve erro em vez de uma classificacao inventada.
func TestDetect_NoSessionNoBoot(t *testing.T) {
	d := NewDetector(adapter.NewSessions(registry.New(1), cfgFor))
	_, err := d.Detect(context.Background(), "sessao-inexistente")
	if err == nil {
		t.Fatal("esperava erro sem sessao registrada")
	}
}
