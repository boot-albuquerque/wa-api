package pairing

import (
	"context"
	"testing"

	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

// TestPhonePairerSatisfazAPorta garante que o adapter satisfaz a porta que
// PairPhoneUseCase vai consumir.
func TestPhonePairerSatisfazAPorta(t *testing.T) {
	var p any = NewPhonePairer(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := p.(appport.PhonePairer); !ok {
		t.Fatal("não satisfaz appport.PhonePairer")
	}
}

// TestPhonePairer_EnsureSession_SemSessaoDevolveErro: mesma garantia do
// resto do pacote de sessões headless — EnsureSession recusa uma sessão
// desconhecida.
func TestPhonePairer_EnsureSession_SemSessaoDevolveErro(t *testing.T) {
	p := NewPhonePairer(adapter.NewSessions(registry.New(1), cfgFor))
	if err := p.EnsureSession(context.Background(), "sessao-inexistente"); err == nil {
		t.Fatal("esperava erro sem sessão registrada")
	}
}

// TestPhonePairer_IsPaired_NaoDetidaDevolveFalsoSemErro: sem sessão detida,
// IsPaired devolve (false, nil) sem tentar subir um browser — a sessão de
// posse é a mesma checagem barata que EnsureSession/QRReader já fazem.
func TestPhonePairer_IsPaired_NaoDetidaDevolveFalsoSemErro(t *testing.T) {
	p := NewPhonePairer(adapter.NewSessions(registry.New(1), cfgFor))
	paired, err := p.IsPaired(context.Background(), "sessao-nao-iniciada")
	if err != nil {
		t.Fatalf("IsPaired sem sessão detida = %v, want nil", err)
	}
	if paired {
		t.Fatal("IsPaired sem sessão detida = true, want false")
	}
}
