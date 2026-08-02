package bootstrap

import (
	"testing"

	"github.com/rs/zerolog"
)

// A Fase 4b consertou o nivel de log global, mas a regra vivia solta dentro de
// Main() e nao havia como exercita-la. boundary_log_test.go ja cobre o EFEITO
// (um logger em info suprime debug); o que faltava era a DECISAO — qual nivel
// sai de qual valor de -loglevel. E' onde estava o defeito original.

func TestResolveLogLevel_MapsRecognizedValues(t *testing.T) {
	cases := map[string]zerolog.Level{
		"debug": zerolog.DebugLevel,
		"info":  zerolog.InfoLevel,
		"warn":  zerolog.WarnLevel,
		"error": zerolog.ErrorLevel,
		// A flag e' documentada em minusculas, mas variavel de ambiente vem
		// como o operador digitou.
		"DEBUG": zerolog.DebugLevel,
		"Warn":  zerolog.WarnLevel,
	}

	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			got, recognized := resolveLogLevel(raw)
			if !recognized {
				t.Fatalf("%q nao foi reconhecido", raw)
			}
			if got != want {
				t.Fatalf("%q -> %v, want %v", raw, got, want)
			}
		})
	}
}

// TestResolveLogLevel_EmptyDoesNotDisableFiltering e o teste que trava o
// defeito da Fase 4b na sua forma exata.
//
// zerolog.ParseLevel("") devolve (NoLevel, nil) — erro NIL. Uma regra que so
// olhasse o erro passaria NoLevel para SetGlobalLevel, o que DESLIGA a
// filtragem e manda todo Debug para producao: precisamente o comportamento
// que a fase existiu para acabar.
func TestResolveLogLevel_EmptyDoesNotDisableFiltering(t *testing.T) {
	got, recognized := resolveLogLevel("")

	if recognized {
		t.Fatal("valor vazio foi tratado como nivel reconhecido — o operador nao seria avisado")
	}
	if got == zerolog.NoLevel {
		t.Fatal("valor vazio virou NoLevel: a filtragem foi desligada e todo Debug volta a ir para producao " +
			"(a regressao da Fase 4b; ParseLevel(\"\") devolve NoLevel com erro NIL)")
	}
	if got != zerolog.InfoLevel {
		t.Fatalf("valor vazio -> %v, want info", got)
	}
}

// TestZerologParseLevel_EmptyStillReturnsNilError trava a PREMISSA da regra
// acima numa dependencia externa. Se um upgrade do zerolog passar a devolver
// erro para "", a guarda explicita de vazio deixa de ser necessaria — e este
// teste e' o que avisa, em vez de a guarda virar codigo morto silencioso.
func TestZerologParseLevel_EmptyStillReturnsNilError(t *testing.T) {
	lvl, err := zerolog.ParseLevel("")

	if err != nil {
		t.Fatalf("o zerolog passou a rejeitar \"\" (%v) — a guarda de vazio em resolveLogLevel "+
			"pode ser simplificada para tratar so o erro", err)
	}
	if lvl != zerolog.NoLevel {
		t.Fatalf("ParseLevel(\"\") = %v, e nao NoLevel — reavaliar a premissa de resolveLogLevel", lvl)
	}
}

func TestResolveLogLevel_UnrecognizedFallsBackToInfo(t *testing.T) {
	for _, raw := range []string{"verbose", "trace-ish", "info ", "warning", " "} {
		t.Run(raw, func(t *testing.T) {
			got, recognized := resolveLogLevel(raw)
			if recognized {
				t.Fatalf("%q foi aceito como nivel valido", raw)
			}
			if got != zerolog.InfoLevel {
				t.Fatalf("%q -> %v, want info", raw, got)
			}
		})
	}
}

// TestResolveLogLevel_NeverReturnsALevelBelowInfoByAccident: nenhuma entrada
// INVALIDA pode aumentar o volume de log. Entradas validas que pedem verbosidade
// (debug, trace, e os numericos — ver o teste seguinte) estao fora daqui de
// proposito: pedir debug e' o contrato da flag.
func TestResolveLogLevel_NeverReturnsALevelBelowInfoByAccident(t *testing.T) {
	for _, raw := range []string{"", " ", "x", "DEBUGG", "null", "verbose"} {
		if got, _ := resolveLogLevel(raw); got < zerolog.InfoLevel {
			t.Fatalf("entrada invalida %q produziu nivel %v, mais verboso que info", raw, got)
		}
	}
}

// TestResolveLogLevel_AcceptsNumericLevels documenta um comportamento HERDADO
// do zerolog que ninguem escolheu e que a flag nao documenta: ParseLevel aceita
// numero. `-loglevel=-1` liga TRACE em producao — a mesma classe de defeito que
// a Fase 4b consertou, por outra porta —, `-loglevel=0` liga DEBUG, e
// `-loglevel=9` silencia TUDO, inclusive erro e panic.
//
// Nenhum destes e' desejavel, e nenhum e' consertado aqui: o teste registra o
// comportamento atual para que a decisao de restringir a flag a nomes seja
// tomada de propria vontade, e nao descoberta em producao. Ver o relatorio da
// Fase 8.
func TestResolveLogLevel_AcceptsNumericLevels(t *testing.T) {
	cases := map[string]zerolog.Level{
		"-1": zerolog.TraceLevel,
		"0":  zerolog.DebugLevel,
		"1":  zerolog.InfoLevel,
	}
	for raw, want := range cases {
		got, recognized := resolveLogLevel(raw)
		if !recognized || got != want {
			t.Fatalf("comportamento numerico mudou: %q -> (%v, %v), antes era (%v, true). "+
				"Se a flag passou a rejeitar numero, isto e' o conserto esperado — "+
				"atualize este teste e remova a pendencia do relatorio da Fase 8", raw, got, recognized, want)
		}
	}

	// O caso que silencia tudo.
	if got, recognized := resolveLogLevel("9"); !recognized || got <= zerolog.PanicLevel {
		t.Fatalf("`-loglevel=9` -> (%v, %v); antes era um nivel acima de panic, que silencia ate erro",
			got, recognized)
	}
}
