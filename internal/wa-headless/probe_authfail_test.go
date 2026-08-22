package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// TestProbeAuthenticationFailure produces the ONE class that makes the
// AUTHENTICATION_FAILURE row mean what it says.
//
// H168 made the page class travel on session.boot_failed, and proved it with a
// real failed boot — but the fixture there was a blank page, which classifies as
// REDIRECT. The upstream's AUTHENTICATION_FAILURE is specifically the
// unauthenticated case, and claiming the row on REDIRECT would be proving the
// mechanism and calling it the meaning.
//
// AN EMPTY PROFILE IS THE WHOLE FIXTURE, and it needs no human: pointed at the
// real SPA, a profile that has never been paired lands on the QR screen, which
// is what LOGIN_REQUIRED names. No pairing is attempted, no QR is read, and the
// profile is a temp directory that goes away with the test.
func TestProbeAuthenticationFailure(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_AUTHFAIL") == "" {
		t.Skip("set WA_PROBE_AUTHFAIL=1 (boots an unpaired profile against the real SPA)")
	}
	hub := events.NewHub()
	var got []events.Event
	unsub := hub.Subscribe(func(e events.Event) { got = append(got, e) }, events.SessionBootFailed)
	defer unsub()

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: t.TempDir(), DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		SettleBudget: 90 * time.Second,
	})
	defer h.Stop(context.Background())
	if !h.AttachHub(hub) {
		t.Fatal("AttachHub refused a fresh Holder")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if _, err := h.Session(ctx); err == nil {
		t.Fatal("an unpaired profile must not boot to ready")
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one boot_failed, got %d", len(got))
	}
	t.Logf("unpaired boot: stage=%q class=%q", got[0].Reason, got[0].PageClass)
	// A FAMILIA, NAO UM VALOR UNICO — e a correcao e' minha, nao do codigo.
	//
	// A primeira asserção exigiu LOGIN_REQUIRED e o perfil vazio deu
	// PAIRING_LOADING, em 2,9s de um orcamento de 90s: o classificador reconhece
	// a tela de pareamento e NAO espera o QR renderizar, o que e' o comportamento
	// certo. Os dois valores dizem "esta pagina quer autenticacao", que e' o que
	// o AUTHENTICATION_FAILURE do upstream nomeia; exigir um deles seria medir o
	// tempo de renderizacao do QR e chamar isso de significado.
	unauthenticated := map[string]bool{
		string(spa.ClassLoginRequired):  true,
		string(spa.ClassPairingLoading): true,
	}
	if !unauthenticated[got[0].PageClass] {
		t.Fatalf("an unpaired profile classified as %q, which is not an "+
			"authentication class; a subscriber cannot match the upstream's "+
			"AUTHENTICATION_FAILURE on it", got[0].PageClass)
	}
	// E A DISTINCAO E' O PONTO: a pagina em branco do teste de runtime classifica
	// REDIRECT, esta classifica pareamento. Se as duas dessem o mesmo, o campo
	// nao separaria nada.
	if got[0].PageClass == string(spa.ClassRedirect) {
		t.Fatal("the unpaired boot classified the same as a blank page")
	}
	t.Log("PROVEN: an unauthenticated boot is distinguishable on the bus")
}
