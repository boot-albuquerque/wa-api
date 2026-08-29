package logout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// TestDo_SucessoNaoDevolveErro: a página aceitou a chamada (measured shape:
// {"ok":true}) — Do não erra.
func TestDo_SucessoNaoDevolveErro(t *testing.T) {
	eval := func(_ context.Context, _ string, out *string) error {
		*out = `{"ok":true}`
		return nil
	}
	if err := Do(context.Background(), engine.NewRunner(), eval, "test"); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

// TestDo_RecusaDaPaginaCitaOMotivo: a página lançou (ex.: Socket.logout não
// é função neste build hipotético) — o erro devolvido tem de nomear o
// motivo, não engolir a informação.
func TestDo_RecusaDaPaginaCitaOMotivo(t *testing.T) {
	eval := func(_ context.Context, _ string, out *string) error {
		*out = `{"ok":false,"why":"Socket.logout is not a function"}`
		return nil
	}
	err := Do(context.Background(), engine.NewRunner(), eval, "test")
	if err == nil {
		t.Fatal("Do devolveu nil, want erro — a página recusou")
	}
	if !errors.Is(err, ErrLogout) {
		t.Fatalf("err = %v, want wrap de ErrLogout", err)
	}
	if !strings.Contains(err.Error(), "Socket.logout is not a function") {
		t.Fatalf("err = %v, quer citar o motivo da página", err)
	}
}
