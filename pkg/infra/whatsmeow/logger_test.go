package whatsmeow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// newTestZerologLogger devolve um zerolog.Logger silencioso para os testes
// do ZerologAdapter — o logger é a entrada do adapter, e os testes
// inspecionam o que o adapter emite via ctx (não a saída do base logger).
func newTestZerologLogger() zerolog.Logger {
	return zerolog.Nop()
}

// captureLogger devolve um *zerolog.Logger apontando para um buffer, com
// o nível configurado para Debug (cobre Info/Warn/Error). Usado para
// inspecionar a saída real do adapter.
func captureLogger() (*zerolog.Logger, *strings.Builder) {
	buf := &strings.Builder{}
	l := zerolog.New(buf).Level(zerolog.DebugLevel)
	return &l, buf
}

// TestZerologAdapter_NewZerologAdapter devolve um adapter não-nil.
func TestZerologAdapter_NewZerologAdapter(t *testing.T) {
	a := NewZerologAdapter(newTestZerologLogger())
	if a == nil {
		t.Fatal("NewZerologAdapter returned nil")
	}
}

// TestZerologAdapter_From_NilCtx devolve o logger base quando ctx é nil.
func TestZerologAdapter_From_NilCtx(t *testing.T) {
	a := NewZerologAdapter(newTestZerologLogger())
	got := a.from(nil)
	if got == nil {
		t.Fatal("from(nil) returned nil")
	}
}

// TestZerologAdapter_From_NoLoggerInCtx cai no logger base.
func TestZerologAdapter_From_NoLoggerInCtx(t *testing.T) {
	a := NewZerologAdapter(newTestZerologLogger())
	got := a.from(context.Background())
	if got == nil {
		t.Fatal("from(ctx sem logger) returned nil")
	}
}

// TestZerologAdapter_From_LoggerInCtx devolve o logger gravado no ctx.
func TestZerologAdapter_From_LoggerInCtx(t *testing.T) {
	a := NewZerologAdapter(newTestZerologLogger())
	l := zerolog.New(zerolog.Nop()).Level(zerolog.DebugLevel)
	ctx := l.WithContext(context.Background())
	got := a.from(ctx)
	if got == nil {
		t.Fatal("from(ctx com logger) returned nil")
	}
	// O logger no ctx tem o mesmo output writer que injetamos.
	if got != zerolog.Ctx(ctx) {
		t.Errorf("from(ctx) devolve logger diferente do gravado no ctx")
	}
}

// TestZerologAdapter_From_DisabledLoggerInCtx cai no logger base.
func TestZerologAdapter_From_DisabledLoggerInCtx(t *testing.T) {
	a := NewZerologAdapter(newTestZerologLogger())
	// Logger desabilitado: GetLevel() == Disabled
	disabled := zerolog.Nop().Level(zerolog.Disabled)
	ctx := disabled.WithContext(context.Background())
	got := a.from(ctx)
	if got == nil {
		t.Fatal("from(ctx com logger disabled) returned nil")
	}
}

// TestZerologAdapter_Info_NoKeyvals emite uma linha Info sem keyvals.
func TestZerologAdapter_Info_NoKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Info(context.Background(), "hello world")

	out := buf.String()
	if out == "" {
		t.Fatal("Info did not emit anything")
	}
	if !strings.Contains(out, `"message":"hello world"`) {
		t.Errorf("output missing message: %s", out)
	}
	if !strings.Contains(out, `"level":"info"`) {
		t.Errorf("output missing level: %s", out)
	}
}

// TestZerologAdapter_Info_WithKeyvals emite keyvals pares.
func TestZerologAdapter_Info_WithKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Info(context.Background(), "test", "key1", "value1", "key2", 42)

	out := buf.String()
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rec); err != nil {
		t.Fatalf("output is not JSON: %v / %s", err, out)
	}
	if rec["key1"] != "value1" {
		t.Errorf("key1 = %v, want value1", rec["key1"])
	}
	// JSON unmarshals numbers como float64
	if rec["key2"] != float64(42) {
		t.Errorf("key2 = %v, want 42", rec["key2"])
	}
}

// TestZerologAdapter_Warn_WithKeyvals emite linha Warn.
func TestZerologAdapter_Warn_WithKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Warn(context.Background(), "warn message", "k", "v")

	out := buf.String()
	if !strings.Contains(out, `"level":"warn"`) {
		t.Errorf("output missing level=warn: %s", out)
	}
	if !strings.Contains(out, `"message":"warn message"`) {
		t.Errorf("output missing message: %s", out)
	}
}

// TestZerologAdapter_Error_WithKeyvals emite linha Error.
func TestZerologAdapter_Error_WithKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Error(context.Background(), "boom", "k", "v")

	out := buf.String()
	if !strings.Contains(out, `"level":"error"`) {
		t.Errorf("output missing level=error: %s", out)
	}
	if !strings.Contains(out, `"message":"boom"`) {
		t.Errorf("output missing message: %s", out)
	}
}

// TestZerologAdapter_Info_NilCtx usa logger base (não panic).
func TestZerologAdapter_Info_NilCtx(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	//nolint:staticcheck // estamos testando explicitamente o caminho ctx=nil
	a.Info(nil, "nil ctx")

	if buf.Len() == 0 {
		t.Fatal("Info with nil ctx emitted nothing")
	}
}

// TestZerologAdapter_ImplementsLoggerPort garante o vínculo estático
// entre o adapter e a porta da aplicação.
func TestZerologAdapter_ImplementsLoggerPort(t *testing.T) {
	var _ appPortLogger = (*ZerologAdapter)(nil)
}

// appPortLogger é um alias local da porta, evita import cycle em teste
// quando a structura real do pacote contracts fosse reexportada.
type appPortLogger = interface {
	Info(ctx context.Context, msg string, keyvals ...any)
	Warn(ctx context.Context, msg string, keyvals ...any)
	Error(ctx context.Context, msg string, keyvals ...any)
}

// TestZerologAdapter_Info_OddKeyvals: keyvals ímpares passam por Fields(),
// zerolog lida — apenas verificamos que não panic.
func TestZerologAdapter_Info_OddKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Info(context.Background(), "odd", "only-key")

	if buf.Len() == 0 {
		t.Fatal("Info with odd keyvals emitted nothing")
	}
}

// TestZerologAdapter_Info_EmptyKeyvals emite Info sem campos.
func TestZerologAdapter_Info_EmptyKeyvals(t *testing.T) {
	capture, buf := captureLogger()
	a := NewZerologAdapter(*capture)
	a.Info(context.Background(), "empty")

	out := buf.String()
	if !strings.Contains(out, `"message":"empty"`) {
		t.Errorf("output missing message: %s", out)
	}
}
