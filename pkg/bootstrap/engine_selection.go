package bootstrap

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

// Seleção de ENGINE por sessão (decisão 94).
//
// # Por que a escolha é explícita, e nunca inferida
//
// Há dois transportes que servem os mesmos ports: o socket (`wa-noise`) e a
// SPA dirigida por navegador (`wa-headless`). Eles NÃO servem o mesmo conjunto:
// medido, o headless satisfaz 17 dos 25 ports, recusa 6 com motivo medido e tem
// 2 pendentes.
//
// Isso torna a escolha uma decisão de operação, não um detalhe: uma sessão em
// headless que chame um port não servido tem de FALHAR DIZENDO ISSO. A decisão
// 94 chama-lhe "sem fallback silencioso por port", e a razão é a mesma do resto
// deste repositório — cair no outro transporte devolveria uma resposta correta
// para uma pergunta que ninguém fez, e ninguém saberia que o engine escolhido
// não foi o usado.

const (
	// EngineWaNoise é o transporte de socket. É o padrão.
	EngineWaNoise = "wanoise"
	// EngineWaHeadless é a SPA dirigida por navegador.
	EngineWaHeadless = "headless"

	envEngineDefault  = "WA_API_ENGINE"
	envEngineSessions = "WA_API_ENGINE_HEADLESS_SESSIONS"
)

// EngineSelection diz qual engine serve cada sessão.
//
// É imutável depois de construída, e a construção é a única porta de entrada:
// não há como uma sessão ganhar engine por acidente em tempo de execução.
type EngineSelection struct {
	padrao      string
	porSessao   map[string]string
	origemLista string
}

// EngineFor devolve o engine desta sessão.
//
// Nunca devolve vazio e nunca erra: erro de configuração acontece na
// construção, que é onde alguém está a olhar.
func (s EngineSelection) EngineFor(txtID string) string {
	if s.porSessao != nil {
		if e, ok := s.porSessao[strings.TrimSpace(txtID)]; ok {
			return e
		}
	}
	return s.padrao
}

// Default devolve o engine das sessões não listadas.
func (s EngineSelection) Default() string { return s.padrao }

// SessoesEmHeadless lista, ordenadas, as sessões explicitamente em headless.
// Existe para o arranque poder DIZER o que vai fazer, em vez de o operador
// descobrir pelo comportamento.
func (s EngineSelection) SessoesEmHeadless() []string {
	var out []string
	for id, e := range s.porSessao {
		if e == EngineWaHeadless {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// engineSelectionConfigurada lê a seleção do ambiente.
//
// Ausente é `wanoise` — o padrão seguro, que é exatamente o comportamento de
// antes desta decisão. Valor DESCONHECIDO é erro e não cai no padrão: um typo
// em `WA_API_ENGINE=headles` não pode virar silenciosamente um processo que
// serve tudo pelo socket enquanto o operador acredita ter mudado de transporte.
func engineSelectionConfigurada() (EngineSelection, error) {
	padrao, err := engineValido(os.Getenv(envEngineDefault), envEngineDefault, EngineWaNoise)
	if err != nil {
		return EngineSelection{}, err
	}
	lista := strings.TrimSpace(os.Getenv(envEngineSessions))
	sel := EngineSelection{padrao: padrao, origemLista: lista}
	if lista == "" {
		return sel, nil
	}
	sel.porSessao = map[string]string{}
	for _, bruto := range strings.Split(lista, ",") {
		id := strings.TrimSpace(bruto)
		if id == "" {
			// Vírgula sobrando é engano de edição, não intenção. Recusar aqui
			// custa uma mensagem; aceitar custa uma sessão que ninguém pediu.
			return EngineSelection{}, fmt.Errorf("%s=%q tem entrada vazia: remova a vírgula sobrando",
				envEngineSessions, lista)
		}
		if anterior, repetida := sel.porSessao[id]; repetida {
			return EngineSelection{}, fmt.Errorf("%s lista %q duas vezes (já era %q): "+
				"uma sessão tem um engine, e repetir esconde qual venceu",
				envEngineSessions, id, anterior)
		}
		sel.porSessao[id] = EngineWaHeadless
	}
	return sel, nil
}

// engineValido aceita o vazio como o padrão dado e recusa o resto.
func engineValido(bruto, nomeVar, padrao string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(bruto))
	switch v {
	case "":
		return padrao, nil
	case EngineWaNoise, EngineWaHeadless:
		return v, nil
	default:
		return "", fmt.Errorf("%s=%q invalido: use %q ou %q", nomeVar, bruto, EngineWaNoise, EngineWaHeadless)
	}
}

// setupEngineSelection lê a seleção e DIZ o que vai fazer.
//
// Falha aqui é fatal, pela mesma razão que a posse de sessão o é: uma seleção
// inutilizável significa que o processo não sabe por qual transporte serve cada
// sessão, e servir pelo errado em silêncio é exatamente o que a decisão 94
// proíbe. Arrancar assim seria escolher a falha mais difícil de diagnosticar.
//
// O log de arranque existe para o operador confirmar o que pediu ANTES de a
// primeira chamada chegar. Só contagens e identificadores de sessão — nunca
// dados de conta.
func setupEngineSelection(s *server) {
	sel, err := engineSelectionConfigurada()
	if err != nil {
		log.Fatal().Err(err).Msg("invalid engine selection configuration")
	}
	s.Engines = sel
	emHeadless := sel.SessoesEmHeadless()
	log.Info().
		Str("default_engine", sel.Default()).
		Int("sessions_on_headless", len(emHeadless)).
		Strs("session_ids", emHeadless).
		Msg("engine selection")
}
