package pairing

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

// TestQRReaderSatisfazAPorta garante que o adapter satisfaz a porta que o
// GetQRUseCase vai consumir.
func TestQRReaderSatisfazAPorta(t *testing.T) {
	var r any = NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := r.(appport.PairingQRReader); !ok {
		t.Fatal("não satisfaz appport.PairingQRReader")
	}
}

// TestPairingQR_NaoDetidaDevolveVazioSemErro: sem sessão detida (StartSession
// nunca foi chamado, ou já foi liberada), PairingQR devolve string vazia sem
// erro e SEM tentar subir um browser — a mesma semântica de "QR rotates,
// vazio não é erro" que o adapter wa_noise já documenta
// (pkg/infra/wa-noise/adapters/pairing/qr.go).
func TestPairingQR_NaoDetidaDevolveVazioSemErro(t *testing.T) {
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	code, err := r.PairingQR(context.Background(), "sessao-nao-iniciada")
	if err != nil {
		t.Fatalf("PairingQR sem sessão detida = %v, want nil", err)
	}
	if code != "" {
		t.Fatalf("PairingQR sem sessão detida = %q, want vazio", code)
	}
}

// TestEnsureSession_SemSessaoDevolveErro: EnsureSession recusa uma sessão
// desconhecida, mesma garantia do resto do pacote de sessões headless.
func TestEnsureSession_SemSessaoDevolveErro(t *testing.T) {
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	if err := r.EnsureSession(context.Background(), "sessao-inexistente"); err == nil {
		t.Fatal("esperava erro sem sessão registrada")
	}
}
