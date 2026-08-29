package phonepair

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates the page-global protocol kickScript/parked rely on —
// NOT a JS interpreter, same convention as capabilities/qr's own double.
type pageDouble struct {
	mu sync.Mutex

	ok      bool
	code    string
	why     string
	errType string

	kicks     int
	answer    string
	answerKey string
}

var keyPattern = regexp.MustCompile(`window\.(__waHeadlessPhonePair_\d+)`)

func (d *pageDouble) eval(_ context.Context, expr string, out *string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	m := keyPattern.FindStringSubmatch(expr)
	if m == nil {
		return fmt.Errorf("pageDouble: no state key found in script: %s", expr)
	}
	key := m[1]

	switch {
	case strings.Contains(expr, "async () => {"):
		d.kicks++
		*out = "kicked"
		d.answer = fmt.Sprintf(`{"ok":%v,"code":%q,"why":%q,"errType":%q}`, d.ok, d.code, d.why, d.errType)
		d.answerKey = key
	case strings.Contains(expr, "delete window."):
		if key == d.answerKey {
			d.answer = ""
		}
		*out = "ok"
	default: // poll: window.KEY || ""
		if key == d.answerKey {
			*out = d.answer
		} else {
			*out = ""
		}
	}
	return nil
}

func compressBudget(t *testing.T) {
	t.Helper()
	old, oldTick := Budget, Tick
	Budget, Tick = 2*time.Second, 5*time.Millisecond
	t.Cleanup(func() { Budget, Tick = old, oldTick })
}

// TestRequest_SucessoDevolveOCodigo: a página respondeu ok=true — Request
// devolve o código, sem erro.
func TestRequest_SucessoDevolveOCodigo(t *testing.T) {
	compressBudget(t)
	d := &pageDouble{ok: true, code: "ABCD1234"}
	code, err := New(engine.NewRunner(), d.eval).Request(context.Background(), "5511999999999", "test")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if code != "ABCD1234" {
		t.Fatalf("code = %q, want ABCD1234", code)
	}
	if d.kicks != 1 {
		t.Fatalf("kicks=%d, want 1", d.kicks)
	}
}

// TestRequest_RecusaDoWhatsappNomeiaOTipo: a página respondeu com uma
// recusa estruturada do WhatsApp (o achado da F380, medido ao vivo:
// IQErrorBadRequest) — o erro devolvido tem de citar o tipo, não engolir a
// informação.
func TestRequest_RecusaDoWhatsappNomeiaOTipo(t *testing.T) {
	compressBudget(t)
	d := &pageDouble{ok: false, why: "CompanionHelloError", errType: "IQErrorBadRequest"}
	_, err := New(engine.NewRunner(), d.eval).Request(context.Background(), "5511999999999", "test")
	if err == nil {
		t.Fatal("Request devolveu nil, want erro — a página recusou")
	}
	if !errors.Is(err, ErrRequest) {
		t.Fatalf("err = %v, want wrap de ErrRequest", err)
	}
	if !strings.Contains(err.Error(), "IQErrorBadRequest") {
		t.Fatalf("err = %v, quer citar o tipo da recusa (IQErrorBadRequest) — "+
			"medido ao vivo (F380) contra um número de teste falso", err)
	}
}

// TestRequest_CodigoVazioComOkENAOAceitoComoSucesso: ok=true com code=""
// não é sucesso — RequestPairingCode's contrato (appport.PhonePairer) diz
// "never empty on success".
func TestRequest_CodigoVazioComOkENAOAceitoComoSucesso(t *testing.T) {
	compressBudget(t)
	d := &pageDouble{ok: true, code: ""}
	code, err := New(engine.NewRunner(), d.eval).Request(context.Background(), "5511999999999", "test")
	if err == nil {
		t.Fatalf("Request devolveu nil e code=%q, want erro — code vazio nunca é sucesso", code)
	}
}
