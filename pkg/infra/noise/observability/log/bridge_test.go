package log

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"

	sdklog "wa-api/internal/noise/observability/log"
)

// newSink devolve um bridge escrevendo num buffer, mais um decodificador do
// último registro emitido.
func newSink(t *testing.T, module string, min zerolog.Level) (*Bridge, func() []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	b := New(zerolog.New(&buf), module, min)
	return b, func() []map[string]any {
		var out []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal(line, &rec); err != nil {
				t.Fatalf("registro nao e JSON valido: %v (%s)", err, line)
			}
			out = append(out, rec)
		}
		return out
	}
}

// TestBridge_NiveisRespeitamPiso: com o piso default (Warn), Warn e Error
// saem e Info/Debug são descartados — o comportamento que a ausência de
// --wadebug passa a ter.
func TestBridge_NiveisRespeitamPiso(t *testing.T) {
	cases := []struct {
		name  string
		min   zerolog.Level
		want  []string
		emitc func(*Bridge)
	}{
		{"default emite warn e error", zerolog.WarnLevel, []string{"warn", "error"}, nil},
		{"info baixa o piso", zerolog.InfoLevel, []string{"info", "warn", "error"}, nil},
		{"debug baixa o piso", zerolog.DebugLevel, []string{"debug", "info", "warn", "error"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, read := newSink(t, ModuleClient, tc.min)
			b.Debugf("d")
			b.Infof("i")
			b.Warnf("w")
			b.Errorf("e")

			recs := read()
			if len(recs) != len(tc.want) {
				t.Fatalf("emitiu %d registros, queria %d (%v)", len(recs), len(tc.want), recs)
			}
			for i, level := range tc.want {
				if recs[i]["level"] != level {
					t.Errorf("registro %d level = %v, queria %q", i, recs[i]["level"], level)
				}
				if recs[i][FieldModule] != ModuleClient {
					t.Errorf("registro %d %s = %v", i, FieldModule, recs[i][FieldModule])
				}
			}
		})
	}
}

// TestBridge_FormataArgs: o SDK chama Errorf com verbos de Sprintf.
func TestBridge_FormataArgs(t *testing.T) {
	b, read := newSink(t, ModuleClient, zerolog.WarnLevel)
	b.Errorf("decrypt failed: %s (%d)", "bad mac", 42)

	recs := read()
	if len(recs) != 1 {
		t.Fatalf("emitiu %d registros, queria 1", len(recs))
	}
	if recs[0]["message"] != "decrypt failed: bad mac (42)" {
		t.Errorf("message = %v", recs[0]["message"])
	}
}

// TestBridge_MensagemSemArgsNaoPassaPorSprintf: uma mensagem literal com %
// não pode virar %!x(MISSING).
func TestBridge_MensagemSemArgsNaoPassaPorSprintf(t *testing.T) {
	b, read := newSink(t, ModuleClient, zerolog.WarnLevel)
	b.Warnf("100% done")

	recs := read()
	if len(recs) != 1 || recs[0]["message"] != "100% done" {
		t.Errorf("message = %v", recs)
	}
}

// TestBridge_Sub concatena o módulo e preserva sink e piso.
func TestBridge_Sub(t *testing.T) {
	b, read := newSink(t, ModuleClient, zerolog.WarnLevel)
	sub := b.Sub("Recv")
	sub.Errorf("boom")
	sub.Infof("ignorado pelo piso herdado")

	recs := read()
	if len(recs) != 1 {
		t.Fatalf("emitiu %d registros, queria 1 (%v)", len(recs), recs)
	}
	if recs[0][FieldModule] != ModuleClient+SubSeparator+"Recv" {
		t.Errorf("%s = %v", FieldModule, recs[0][FieldModule])
	}
}

// TestBridge_Sub_Aninhado: Sub de Sub continua concatenando.
func TestBridge_Sub_Aninhado(t *testing.T) {
	b, read := newSink(t, ModuleClient, zerolog.WarnLevel)
	b.Sub("Recv").Sub("Retry").Errorf("boom")

	recs := read()
	if len(recs) != 1 || recs[0][FieldModule] != "Client/Recv/Retry" {
		t.Errorf("%s = %v", FieldModule, recs)
	}
}

// TestBridge_Sub_ModuloVazio devolve o mesmo bridge, sem separador solto.
func TestBridge_Sub_ModuloVazio(t *testing.T) {
	b, read := newSink(t, ModuleClient, zerolog.WarnLevel)
	b.Sub("").Errorf("boom")

	recs := read()
	if len(recs) != 1 || recs[0][FieldModule] != ModuleClient {
		t.Errorf("%s = %v", FieldModule, recs)
	}
}

// TestNew_ModuloVazio cai em ModuleRoot em vez de emitir campo vazio.
func TestNew_ModuloVazio(t *testing.T) {
	b, read := newSink(t, "", zerolog.WarnLevel)
	b.Errorf("boom")

	recs := read()
	if len(recs) != 1 || recs[0][FieldModule] != ModuleRoot {
		t.Errorf("%s = %v", FieldModule, recs)
	}
}

// TestBridge_ImplementaWaLogLogger: o contrato do vendored, exercitado pela
// interface e não pelo tipo concreto.
func TestBridge_ImplementaWaLogLogger(t *testing.T) {
	var logger sdklog.Logger = New(zerolog.Nop(), ModuleDatabase, zerolog.WarnLevel)
	logger.Debugf("d")
	logger.Infof("i")
	logger.Warnf("w")
	logger.Errorf("e")
	if logger.Sub("x") == nil {
		t.Error("Sub devolveu nil")
	}
}
