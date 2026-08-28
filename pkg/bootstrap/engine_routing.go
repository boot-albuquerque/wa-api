package bootstrap

import (
	"fmt"

	"wa-api/pkg/domain"
)

// A regra de roteamento por engine (decisão 94), isolada do roteador para
// poder ser exercitada sem port nenhum.
//
// Extrair a regra em vez de a testar através de um roteador é a lição da
// decisão 82: uma regra que vive num `if` dentro de vinte e cinco roteadores
// é vinte e cinco oportunidades de divergir, e nenhum teste que a segure.

// ErrEngineSemPort diz que a sessão está num engine que NÃO serve este port.
//
// É deliberadamente um erro, e não uma queda para o outro transporte. Cair
// devolveria uma resposta correta para uma pergunta que ninguém fez: o
// operador escolheu um engine, e a operação teria acontecido noutro sem que
// nada o dissesse.
type ErrEngineSemPort struct {
	Port   string
	Engine string
	TxtID  string
	// Motivo é o que o inventário mediu, quando há: "recusado por capacidade
	// ausente do build", "pendente por falta de medição". Vazio quando o port
	// simplesmente ainda não foi ligado.
	Motivo string
}

func (e ErrEngineSemPort) Error() string {
	base := fmt.Sprintf("engine %q nao serve o port %q para esta sessao", e.Engine, e.Port)
	if e.Motivo != "" {
		return base + ": " + e.Motivo
	}
	return base + " (sem fallback silencioso, decisao 94)"
}

// rotaDeEngine decide, para UMA chamada, se ela vai ao headless, ao socket, ou
// a lado nenhum.
//
// `temHeadless` é a presença da implementação headless daquele port — nil
// significa "este engine não serve isto", e nunca "use o outro".
func rotaDeEngine(engine string, temHeadless bool) (usarHeadless bool, recusar bool) {
	if engine != domain.EngineWaHeadless {
		return false, false
	}
	if !temHeadless {
		return false, true
	}
	return true, false
}
