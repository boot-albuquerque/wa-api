package waLog

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

// newZerolog devolve o Logger e o buffer para onde ele escreve. E' o caminho de
// producao do wa-api: o bootstrap embrulha um zerolog e o passa ao whatsmeow.
func newZerolog(t *testing.T) (Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return Zerolog(zerolog.New(&buf).Level(zerolog.DebugLevel)), &buf
}

// decodeLines desserializa cada linha JSON que o zerolog escreveu.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("linha nao e' JSON: %q (%v)", line, err)
		}
		out = append(out, entry)
	}
	return out
}

// Cada metodo do Logger tem que mapear para o nivel correspondente do zerolog.
// E' a razao de o adaptador existir, e trocar dois deles faria um erro sair
// como debug e ser descartado pelo filtro de nivel do zerolog em producao.
func TestZerologMapsEachLevel(t *testing.T) {
	tests := []struct {
		name  string
		call  func(Logger)
		level string
	}{
		{"Errorf", func(l Logger) { l.Errorf("m") }, "error"},
		{"Warnf", func(l Logger) { l.Warnf("m") }, "warn"},
		{"Infof", func(l Logger) { l.Infof("m") }, "info"},
		{"Debugf", func(l Logger) { l.Debugf("m") }, "debug"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log, buf := newZerolog(t)
			tc.call(log)
			entries := decodeLines(t, buf)
			if len(entries) != 1 {
				t.Fatalf("linhas escritas = %d, esperado 1", len(entries))
			}
			if got := entries[0]["level"]; got != tc.level {
				t.Errorf("level = %v, esperado %q", got, tc.level)
			}
		})
	}
}

func TestZerologFormatsArguments(t *testing.T) {
	log, buf := newZerolog(t)
	log.Infof("%s tem %d itens", "fila", 3)
	entries := decodeLines(t, buf)
	if got := entries[0]["message"]; got != "fila tem 3 itens" {
		t.Errorf("message = %v", got)
	}
}

// Sub grava o nome do modulo no campo `sublogger` do contexto, em vez de
// concatenar na mensagem. E' o que permite filtrar por modulo no agregador de
// log sem parsear texto.
func TestZerologSubAddsSubloggerField(t *testing.T) {
	log, buf := newZerolog(t)
	log.Sub("Send").Infof("m")
	entries := decodeLines(t, buf)
	if got := entries[0]["sublogger"]; got != "Send" {
		t.Errorf("sublogger = %v, esperado \"Send\"", got)
	}
}

func TestZerologSubNestsModuleNames(t *testing.T) {
	log, buf := newZerolog(t)
	log.Sub("Send").Sub("Retry").Sub("Phone").Warnf("m")
	entries := decodeLines(t, buf)
	want := "Send" + moduleSeparator + "Retry" + moduleSeparator + "Phone"
	if got := entries[0]["sublogger"]; got != want {
		t.Errorf("sublogger = %v, esperado %q", got, want)
	}
}

// O sublogger nao pode contaminar o pai: os dois escrevem no mesmo destino, e
// se Sub mutasse o logger original todas as linhas seguintes do pai ganhariam
// o campo sublogger.
func TestZerologSubDoesNotMutateItsParent(t *testing.T) {
	log, buf := newZerolog(t)
	sub := log.Sub("Send")
	sub.Infof("do filho")
	log.Infof("do pai")

	entries := decodeLines(t, buf)
	if len(entries) != 2 {
		t.Fatalf("linhas = %d, esperado 2", len(entries))
	}
	if entries[0]["sublogger"] != "Send" {
		t.Errorf("a linha do filho perdeu o sublogger: %v", entries[0])
	}
	if _, has := entries[1]["sublogger"]; has {
		t.Errorf("a linha do pai ganhou sublogger: %v", entries[1])
	}
}

// O nivel do zerolog embrulhado continua valendo: o adaptador nao pode
// reintroduzir logs que a configuracao do zerolog descarta.
func TestZerologRespectsTheWrappedLoggerLevel(t *testing.T) {
	var buf bytes.Buffer
	log := Zerolog(zerolog.New(&buf).Level(zerolog.WarnLevel))
	log.Debugf("descartado")
	log.Infof("descartado")
	log.Warnf("mantido")

	entries := decodeLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("linhas = %d, esperado 1: %v", len(entries), entries)
	}
	if entries[0]["message"] != "mantido" {
		t.Errorf("= %v", entries[0])
	}
}

func TestZerologSatisfiesTheLoggerInterface(t *testing.T) {
	var _ Logger = Zerolog(zerolog.Nop())
}
