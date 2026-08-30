package bootstrap

import (
	"os"
	"strings"
	"testing"
)

// TestSetupEngineSelectionAntesDeInitCustomHandlers trava uma ORDEM de boot
// em Main() — não uma unidade isolável sem reescrever a função inteira, daí
// o teste ler o próprio arquivo-fonte em vez de chamar Main() (que bloqueia
// servindo HTTP e faz flag.Parse do processo de teste).
//
// # O defeito medido
//
// setupEngineSelection(s) é quem lê WA_API_HEADLESS_CHROME e popula
// s.Headless (engine_selection.go). initCustomHandlers(s)
// (wiring_handlers.go) lê s.Headless.ChromePath para decidir se constrói o
// Disconnector/QRReader/Starter de wa_headless (F370/H145).
//
// Até esta correção, Main() chamava initCustomHandlers ANTES de
// setupEngineSelection — initCustomHandlers via' sempre s.Headless ZERO,
// então TODO adapter de sessão headless nunca era ligado em produção, com
// ou sem Chrome configurado. MEDIDO ao vivo (servidor real, Claude in
// Chrome): o log de arranque dizia "headless_ports=0" numa linha e
// "headless_configured=true" na linha seguinte — a prova de que a
// configuração chegava DEPOIS de já ter sido consultada. Nenhum teste
// unitário deste pacote pegou isso porque todos constroem os componentes
// diretamente, sem passar pela ORDEM real de Main().
func TestSetupEngineSelectionAntesDeInitCustomHandlers(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("lendo main.go: %v", err)
	}
	src := string(body)

	// Casa a CHAMADA, não a declaração: "func setupEngineSelection(s *server)"
	// e "func initCustomHandlers(s *server)" vivem noutros arquivos e não
	// aparecem aqui, então esta forma já é específica o suficiente — mas
	// escrita por extenso porque um literal solto repetido é o mesmo bug à
	// espera de divergir (ADR-0004).
	const chamadaSelecao = "setupEngineSelection(s)"
	const chamadaHandlers = "initCustomHandlers(s)"

	iSelecao := strings.Index(src, chamadaSelecao)
	iHandlers := strings.Index(src, chamadaHandlers)
	if iSelecao < 0 {
		t.Fatalf("main.go não chama %q — engine_selection.go ficou órfão?", chamadaSelecao)
	}
	if iHandlers < 0 {
		t.Fatalf("main.go não chama %q — wiring_handlers.go ficou órfão?", chamadaHandlers)
	}
	if iSelecao >= iHandlers {
		t.Errorf("%q (offset %d) sai DEPOIS de %q (offset %d): initCustomHandlers "+
			"leria s.Headless ainda zerado, e nenhum adapter de sessão wa_headless seria "+
			"construído — o defeito medido ao vivo que esta correção resolveu",
			chamadaSelecao, iSelecao, chamadaHandlers, iHandlers)
	}
}
