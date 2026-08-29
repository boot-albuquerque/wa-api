package log

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout roda fn com o stdout redirecionado e devolve o que foi escrito.
// stdoutLogger usa fmt.Printf direto, sem writer injetavel, entao nao ha outro
// jeito de observar a saida.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Noop tem que engolir tudo sem escrever nada. E' o fallback usado quando o
// bootstrap nao passa logger: se ele imprimisse, o wa-api sujaria o stdout.
func TestNoopWritesNothing(t *testing.T) {
	out := captureStdout(t, func() {
		Noop.Errorf("erro %d", 1)
		Noop.Warnf("aviso")
		Noop.Infof("info")
		Noop.Debugf("debug")
		Noop.Sub("modulo").Errorf("do sublogger")
	})
	if out != "" {
		t.Errorf("Noop escreveu %q", out)
	}
}

// Sub do Noop devolve o proprio Noop: nao ha estado a acumular, e devolver uma
// instancia nova a cada Sub vazaria memoria em codigo que chama Sub em laco.
func TestNoopSubReturnsItself(t *testing.T) {
	if Noop.Sub("a") != Noop {
		t.Error("Noop.Sub devolveu outra instancia")
	}
}

func TestStdoutIncludesModuleLevelAndMessage(t *testing.T) {
	out := captureStdout(t, func() {
		Stdout("Client", "", false).Infof("conectado em %s", "s.whatsapp.net")
	})
	for _, want := range []string{"Client", LevelInfo, "conectado em s.whatsapp.net"} {
		if !strings.Contains(out, want) {
			t.Errorf("saida %q nao contem %q", out, want)
		}
	}
}

// Cada metodo tem que sair com o SEU nivel. Trocar dois deles faria um erro
// aparecer como debug e sumir sob qualquer nivel minimo configurado.
func TestEachMethodLogsItsOwnLevel(t *testing.T) {
	tests := []struct {
		level string
		call  func(Logger)
	}{
		{LevelError, func(l Logger) { l.Errorf("m") }},
		{LevelWarn, func(l Logger) { l.Warnf("m") }},
		{LevelInfo, func(l Logger) { l.Infof("m") }},
		{LevelDebug, func(l Logger) { l.Debugf("m") }},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			out := captureStdout(t, func() { tc.call(Stdout("M", "", false)) })
			if !strings.Contains(out, "["+"M"+" "+tc.level+"]") {
				t.Errorf("saida %q nao traz o nivel %s", out, tc.level)
			}
		})
	}
}

// O nivel minimo filtra tudo que esta abaixo dele. E' a unica coisa em log.go
// com logica de verdade.
func TestMinimumLevelFiltersLowerLevels(t *testing.T) {
	tests := []struct {
		min     string
		visible []string
		hidden  []string
	}{
		{"", []string{LevelDebug, LevelInfo, LevelWarn, LevelError}, nil},
		{"DEBUG", []string{LevelDebug, LevelInfo, LevelWarn, LevelError}, nil},
		{"INFO", []string{LevelInfo, LevelWarn, LevelError}, []string{LevelDebug}},
		{"WARN", []string{LevelWarn, LevelError}, []string{LevelDebug, LevelInfo}},
		{"ERROR", []string{LevelError}, []string{LevelDebug, LevelInfo, LevelWarn}},
	}
	for _, tc := range tests {
		t.Run("min="+tc.min, func(t *testing.T) {
			out := captureStdout(t, func() {
				l := Stdout("M", tc.min, false)
				l.Debugf("d")
				l.Infof("i")
				l.Warnf("w")
				l.Errorf("e")
			})
			for _, level := range tc.visible {
				if !strings.Contains(out, level) {
					t.Errorf("nivel minimo %q escondeu %s: %q", tc.min, level, out)
				}
			}
			for _, level := range tc.hidden {
				if strings.Contains(out, level) {
					t.Errorf("nivel minimo %q deixou passar %s: %q", tc.min, level, out)
				}
			}
		})
	}
}

// O nivel minimo aceita minusculas: a configuracao vem de variavel de ambiente
// e ninguem digita maiusculo.
func TestMinimumLevelIsCaseInsensitive(t *testing.T) {
	out := captureStdout(t, func() {
		l := Stdout("M", "warn", false)
		l.Infof("nao deveria aparecer")
		l.Warnf("deveria aparecer")
	})
	if strings.Contains(out, "nao deveria aparecer") {
		t.Errorf("minusculo nao foi reconhecido: %q", out)
	}
	if !strings.Contains(out, "deveria aparecer") {
		t.Errorf("= %q", out)
	}
}

// Um nivel minimo desconhecido cai em levelUnset (-1), que e' MENOR que DEBUG:
// imprime tudo. Falhar aberto e' o comportamento certo para um logger — errar a
// configuracao nao pode silenciar os erros da aplicacao.
func TestUnknownMinimumLevelLogsEverything(t *testing.T) {
	out := captureStdout(t, func() {
		l := Stdout("M", "nivel-inexistente", false)
		l.Debugf("d")
		l.Errorf("e")
	})
	if !strings.Contains(out, LevelDebug) || !strings.Contains(out, LevelError) {
		t.Errorf("nivel desconhecido silenciou logs: %q", out)
	}
}

func TestColorIsOnlyEmittedWhenRequested(t *testing.T) {
	plain := captureStdout(t, func() { Stdout("M", "", false).Errorf("m") })
	if strings.Contains(plain, "\033[") {
		t.Errorf("saida sem cor trouxe escape ANSI: %q", plain)
	}

	colored := captureStdout(t, func() { Stdout("M", "", true).Errorf("m") })
	if !strings.Contains(colored, ansiRed) {
		t.Errorf("erro colorido nao usou vermelho: %q", colored)
	}
	if !strings.HasSuffix(strings.TrimRight(colored, "\n"), ansiReset) {
		t.Errorf("a linha colorida nao terminou com o reset: %q", colored)
	}
}

// DEBUG nao tem cor no mapa. O codigo tem que sair sem escape, nao com uma
// string vazia entre escapes quebrados.
func TestDebugHasNoColorButStillResets(t *testing.T) {
	out := captureStdout(t, func() { Stdout("M", "", true).Debugf("m") })
	if strings.Contains(out, ansiRed) || strings.Contains(out, ansiCyan) || strings.Contains(out, ansiYellow) {
		t.Errorf("DEBUG saiu colorido: %q", out)
	}
}

// Sub concatena os modulos com "/", e o resultado herda cor e nivel minimo do
// pai. Sem herdar o nivel, um sublogger reabriria os logs que o pai filtra.
func TestSubNestsModulesAndInheritsSettings(t *testing.T) {
	out := captureStdout(t, func() {
		Stdout("Client", "WARN", false).Sub("Send").Sub("Retry").Errorf("m")
	})
	if !strings.Contains(out, "Client"+moduleSeparator+"Send"+moduleSeparator+"Retry") {
		t.Errorf("modulos nao foram aninhados: %q", out)
	}

	filtered := captureStdout(t, func() {
		Stdout("Client", "WARN", false).Sub("Send").Infof("nao deveria aparecer")
	})
	if filtered != "" {
		t.Errorf("o sublogger nao herdou o nivel minimo: %q", filtered)
	}

	colored := captureStdout(t, func() {
		Stdout("Client", "", true).Sub("Send").Errorf("m")
	})
	if !strings.Contains(colored, ansiRed) {
		t.Errorf("o sublogger nao herdou a cor: %q", colored)
	}
}

func TestStdoutFormatsArguments(t *testing.T) {
	out := captureStdout(t, func() {
		Stdout("M", "", false).Warnf("%s tem %d itens", "fila", 3)
	})
	if !strings.Contains(out, "fila tem 3 itens") {
		t.Errorf("= %q", out)
	}
}

// A linha comeca com o horario no formato configurado — e' o que permite
// correlacionar com outros logs.
func TestLineStartsWithTimestamp(t *testing.T) {
	out := captureStdout(t, func() { Stdout("M", "", false).Infof("m") })
	// hh:mm:ss.mmm = 12 caracteres, com ':' nas posicoes 2 e 5.
	if len(out) < len(timestampFormat) || out[2] != ':' || out[5] != ':' || out[8] != '.' {
		t.Errorf("a linha nao comeca com o horario: %q", out)
	}
}

// Os tres mapas de nivel tem que concordar sobre os nomes. Uma constante nova
// sem entrada em levelToInt sairia com prioridade zero silenciosamente.
func TestEveryLevelIsRankedAndOnlyDebugHasNoColor(t *testing.T) {
	for _, level := range []string{LevelDebug, LevelInfo, LevelWarn, LevelError} {
		if _, ok := levelToInt[level]; !ok {
			t.Errorf("nivel %s nao esta em levelToInt", level)
		}
	}
	if _, ok := colors[LevelDebug]; ok {
		t.Error("DEBUG passou a ter cor; atualize TestDebugHasNoColorButStillResets")
	}
	for _, level := range []string{LevelInfo, LevelWarn, LevelError} {
		if _, ok := colors[level]; !ok {
			t.Errorf("nivel %s nao tem cor", level)
		}
	}
	if levelToInt[""] >= levelToInt[LevelDebug] {
		t.Error("o nivel vazio deixou de ser o mais permissivo")
	}
}
