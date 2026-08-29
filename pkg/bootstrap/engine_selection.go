package bootstrap

import (
	"github.com/rs/zerolog/log"
)

// setupEngineSelection lê a configuração do lado headless e DIZ o que vai
// fazer.
//
// # A decisão 94 (removida) e o que ficou dela
//
// Até aqui, o engine de cada sessão era escolhido de forma ESTÁTICA no
// arranque, por duas variáveis de ambiente (WA_API_ENGINE,
// WA_API_ENGINE_HEADLESS_SESSIONS): um processo servia UM conjunto fixo de
// sessões por socket e outro por headless, decidido antes de qualquer
// requisição existir.
//
// Essa decisão foi removida a pedido explícito do usuário: o engine passa a
// ser escolhido pelo CHAMADOR, por sessão, no corpo de
// POST /admin/users (`engine`), e persistido na coluna `users.engine`
// (migração 19) — não mais inferido nem fixado no arranque. As duas
// variáveis de ambiente da decisão 94 deixam de existir; um processo com
// elas ainda no ambiente simplesmente as ignora.
//
// O que NÃO mudou é o princípio de "sem fallback silencioso": um pedido de
// sessão em domain.EngineHeadless num processo sem o Chrome do headless
// configurado FALHA, com um erro dizendo por quê
// (user.AddUserUseCase, engineHeadlessUnavailableCode) — nunca cai
// silenciosamente para o socket. E o roteamento por PORT que a decisão 94
// também descrevia (pkg/bootstrap/engine_routing.go, ErrEngineSemPort) segue
// existindo: aquele mecanismo nunca dependeu da seleção estática, e o
// headless continua servindo só 17 dos 25 ports que o socket serve — gap
// medido e deliberadamente fora do escopo desta mudança.
func setupEngineSelection(s *server) {
	hcfg, err := headlessConfigConfigurada()
	if err != nil {
		log.Fatal().Err(err).Msg("invalid headless engine configuration")
	}
	s.Headless = hcfg

	log.Info().
		Bool("headless_configured", hcfg.ChromePath != "").
		Int("headless_max_sessions", hcfg.MaxSessions).
		Msg("engine selection: per-session, chosen at POST /admin/users (decisão 94 removida)")
}
