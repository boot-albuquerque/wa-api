package pairing

import (
	"context"
	"testing"
	"time"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/internal/wa-headless/capabilities/qr"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

// compressStaleRefreshAfter shrinks qr.StaleRefreshAfter for the duration of
// a test — same convention as internal/wa-headless/capabilities/qr's own
// compressBudgets, so a staleness test runs in milliseconds instead of
// waiting out the real 90s.
func compressStaleRefreshAfter(t *testing.T) {
	t.Helper()
	old := qr.StaleRefreshAfter
	qr.StaleRefreshAfter = 50 * time.Millisecond
	t.Cleanup(func() { qr.StaleRefreshAfter = old })
}

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

// TestStaleHint_SemHistoricoDevolveFalso: um txtID nunca visto por
// trackCode não tem como estar "stale" — não há desde-quando para medir.
func TestStaleHint_SemHistoricoDevolveFalso(t *testing.T) {
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	if r.staleHint("nunca-visto") {
		t.Fatal("staleHint = true sem histórico nenhum, want false")
	}
}

// TestStaleHint_TornaVerdadeiroAposOLimiar é o achado que motivou F374:
// MEDIDO (TestProbeQRRetryPattern, 2026-08-29) que a rotação do QR do
// wa_headless tem gaps reais de até 60s, e sem este relógio o adapter
// devolveria o MESMO código indefinidamente durante um gap desses. Aqui,
// com qr.StaleRefreshAfter comprimido, o mesmo código rastreado por
// trackCode deve virar "stale" assim que o limiar passa — e NÃO antes.
func TestStaleHint_TornaVerdadeiroAposOLimiar(t *testing.T) {
	compressStaleRefreshAfter(t)
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.trackCode("sessao-x", "codigo-abc")

	if r.staleHint("sessao-x") {
		t.Fatal("staleHint = true logo após trackCode, want false — o limiar ainda não passou")
	}
	time.Sleep(qr.StaleRefreshAfter + 10*time.Millisecond)
	if !r.staleHint("sessao-x") {
		t.Fatal("staleHint = false depois do limiar, want true — é exatamente o gap medido de 60s " +
			"que este mecanismo existe para cobrir")
	}
}

// TestTrackCode_MesmoCodigoNaoReiniciaORelogio: se cada chamada de
// trackCode com o MESMO código reiniciasse o relógio, um poller de 1s em 1s
// (como o do devui) manteria "stale" para sempre falso — o poll em si já
// re-trackeia o código a cada segundo, e um bug aqui faria a suíte inteira
// de negativo de F374 passar por acidente, mascarando exatamente o defeito
// que staleHint corrige.
//
// As duas janelas de sleep têm de ser desenhadas para DISTINGUIR "reiniciou"
// de "não reiniciou" — não bastam margens simétricas. Medido: com
// StaleRefreshAfter=50ms, sleep(40ms) + retrack + sleep(20ms) dá 60ms desde
// o track ORIGINAL (>=50ms, stale=true se o relógio não reiniciou) mas só
// 20ms desde um retrack que reiniciasse por engano (<50ms, stale=false se
// reiniciou) — a primeira versão deste teste usava janelas simétricas
// (metade + metade) e passava com OU sem o defeito, um controle negativo
// que não mordia.
func TestTrackCode_MesmoCodigoNaoReiniciaORelogio(t *testing.T) {
	compressStaleRefreshAfter(t)
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.trackCode("sessao-x", "codigo-abc")
	time.Sleep(40 * time.Millisecond)
	r.trackCode("sessao-x", "codigo-abc") // mesmo código: não deve adiar a marca
	time.Sleep(20 * time.Millisecond)     // 60ms desde o track original; 20ms desde o retrack
	if !r.staleHint("sessao-x") {
		t.Fatal("staleHint = false, want true — retrackear o MESMO código não pode adiar o relógio")
	}
}

// TestTrackCode_CodigoDiferenteReiniciaORelogio: um código NOVO significa
// que a página já rotacionou sozinha — o relógio de staleness tem de
// reiniciar, senão um segundo gap logo depois do primeiro dispararia
// staleHint cedo demais, empurrando refreshQR contra uma sessão que acabou
// de se atualizar sozinha.
func TestTrackCode_CodigoDiferenteReiniciaORelogio(t *testing.T) {
	compressStaleRefreshAfter(t)
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.trackCode("sessao-x", "codigo-abc")
	time.Sleep(qr.StaleRefreshAfter - 5*time.Millisecond)
	r.trackCode("sessao-x", "codigo-DIFERENTE")
	if r.staleHint("sessao-x") {
		t.Fatal("staleHint = true logo após um código NOVO, want false — o relógio devia ter reiniciado")
	}
}

// TestForgetCode_ZeraOHistorico: depois de forgetCode, staleHint volta a
// false mesmo tendo passado o limiar antes — usado quando a sessão deixa de
// estar detida ou acabou de parear, onde "stale" não tem mais sentido.
func TestForgetCode_ZeraOHistorico(t *testing.T) {
	compressStaleRefreshAfter(t)
	r := NewQRReader(adapter.NewSessions(registry.New(1), cfgFor))
	r.trackCode("sessao-x", "codigo-abc")
	time.Sleep(qr.StaleRefreshAfter + 10*time.Millisecond)
	if !r.staleHint("sessao-x") {
		t.Fatal("pré-condição do teste falhou: devia estar stale antes do forgetCode")
	}
	r.forgetCode("sessao-x")
	if r.staleHint("sessao-x") {
		t.Fatal("staleHint = true depois de forgetCode, want false")
	}
}
