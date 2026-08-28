package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

// Configuração do lado headless.
//
// # Por que a leitura passou a ser sempre tentada, nunca exigida
//
// Até a remoção da decisão 94, esta configuração era exigida no arranque
// SOMENTE quando a seleção estática de engine (WA_API_ENGINE /
// WA_API_ENGINE_HEADLESS_SESSIONS) apontava para headless. Essa seleção não
// existe mais: o engine agora é escolhido por sessão, em tempo de requisição
// (POST /admin/users), então o arranque não pode mais saber se headless VAI
// ser pedido.
//
// A leitura passa a ser sempre tentada e nunca fatal por ausência: variáveis
// ausentes devolvem a configuração zero, silenciosamente. Quem recusa um
// pedido de sessão em headless sem Chrome configurado é
// user.AddUserUseCase, na hora do pedido — não o arranque, adivinhando. Isso
// preserva a regra de "sem fallback silencioso": o pedido FALHA com um erro
// dizendo por quê, em vez de cair para o socket sem avisar.
//
// Uma variável PRESENTE mas inválida (caminho que não existe, diretório no
// lugar de executável, número inválido) continua fatal: quem configurou
// pretendia usar headless, e um erro de digitação não deve virar "headless
// desligado" em silêncio.

const (
	envHeadlessChrome      = "WA_API_HEADLESS_CHROME"
	envHeadlessProfiles    = "WA_API_HEADLESS_PROFILES"
	envHeadlessMaxSessions = "WA_API_HEADLESS_MAX_SESSIONS"
	envHeadlessUserAgent   = "WA_API_HEADLESS_USER_AGENT"

	// headlessTargetURL é a SPA de produção.
	headlessTargetURL = "https://web.whatsapp.com/"

	// headlessDefaultUserAgent é exigido por COMPATIBILIDADE, não por evasão.
	//
	// O WhatsApp recusa o token HeadlessChrome e serve uma página de "atualize
	// o navegador"; o estudo perdeu uma corrida a lançar sem isto. O token
	// acompanha a versão real do navegador instalado, e nada mais da impressão
	// digital é tocado. Por isso é CONFIGURÁVEL: um host com outra versão de
	// Chrome precisa do token dele, não do meu.
	headlessDefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
)

// HeadlessConfig é o que o transporte headless precisa para existir.
type HeadlessConfig struct {
	ChromePath  string
	ProfileRoot string
	MaxSessions int
	UserAgent   string
}

// StartConfigFor monta a configuração de arranque de UMA sessão.
//
// O txtID vira nome de diretório, então ele é validado ANTES de virar caminho:
// um identificador com `..` ou separador escaparia da raiz de perfis e
// apontaria para qualquer lugar do disco. Um perfil é credencial de sessão, e
// a travessia aqui não seria um bug de caminho — seria escolher qual conta
// restaurar.
func (c HeadlessConfig) StartConfigFor(txtID string) (waheadless.StartConfig, error) {
	nome, err := nomeDePerfilSeguro(txtID)
	if err != nil {
		return waheadless.StartConfig{}, err
	}
	return waheadless.StartConfig{
		BinaryPath:  c.ChromePath,
		ProfileDir:  filepath.Join(c.ProfileRoot, nome),
		UserAgent:   c.UserAgent,
		NavigateURL: headlessTargetURL,
	}, nil
}

// nomeDePerfilSeguro recusa tudo que não seja um nome de diretório simples.
//
// A lista é de RECUSAS explícitas, e não de sanitização: substituir caracteres
// faria dois txtID diferentes colidirem no mesmo perfil, que é pior que
// recusar — duas sessões a partilhar credencial viola a invariante 13 sem que
// nada o diga.
func nomeDePerfilSeguro(txtID string) (string, error) {
	nome := strings.TrimSpace(txtID)
	var motivo string
	switch {
	case nome == "":
		motivo = "vazio"
	case nome == "." || nome == "..":
		motivo = "aponta para fora da raiz de perfis"
	case strings.ContainsAny(nome, `/\`):
		motivo = "tem separador de caminho e escaparia da raiz de perfis"
	case strings.ContainsRune(nome, 0):
		motivo = "tem byte nulo"
	case nome != filepath.Base(nome):
		motivo = "nao e' um nome de diretorio simples"
	}
	if motivo == "" {
		return nome, nil
	}
	// Logado em Warn, e não só devolvido: um txtID que teria escapado da raiz
	// de perfis é um pedido para restaurar OUTRA conta, e quem opera precisa de
	// o ver mesmo que a chamada de cima trate o erro em silêncio. O COMPRIMENTO
	// e o motivo bastam para investigar; o valor não vai para o log.
	log.Warn().
		Int("txt_id_len", len(txtID)).
		Str("reason", motivo).
		Msg("headless profile name refused")
	return "", fmt.Errorf("headless: txtID recusado como diretorio de perfil: %s", motivo)
}

// headlessConfigConfigurada lê a configuração do headless SE ela existir.
//
// Nenhuma das duas variáveis presentes: devolve a configuração zero, sem
// erro — headless simplesmente não está disponível neste processo, e
// AddUserUseCase recusa quem pedir esse engine (engineHeadlessUnavailableCode).
// Uma das duas presente sem a outra, ou presente e inválida: erro fatal, pela
// razão no comentário do pacote acima.
func headlessConfigConfigurada() (HeadlessConfig, error) {
	chrome := strings.TrimSpace(os.Getenv(envHeadlessChrome))
	perfis := strings.TrimSpace(os.Getenv(envHeadlessProfiles))
	if chrome == "" && perfis == "" {
		return HeadlessConfig{}, nil
	}
	if chrome == "" {
		return HeadlessConfig{}, fmt.Errorf("%s e' obrigatorio quando %s esta configurado",
			envHeadlessChrome, envHeadlessProfiles)
	}
	if perfis == "" {
		return HeadlessConfig{}, fmt.Errorf("%s e' obrigatorio quando %s esta configurado",
			envHeadlessProfiles, envHeadlessChrome)
	}
	// O binário é conferido AGORA, e não na primeira sessão: um caminho errado
	// descoberto no arranque custa uma linha de log, e descoberto na primeira
	// chamada custa um pedido de cliente.
	if info, err := os.Stat(chrome); err != nil {
		return HeadlessConfig{}, fmt.Errorf("%s=%q nao pode ser lido: %w", envHeadlessChrome, chrome, err)
	} else if info.IsDir() {
		return HeadlessConfig{}, fmt.Errorf("%s=%q e' um diretorio, nao um executavel", envHeadlessChrome, chrome)
	}
	raiz, err := filepath.Abs(perfis)
	if err != nil {
		return HeadlessConfig{}, fmt.Errorf("%s=%q nao resolve: %w", envHeadlessProfiles, perfis, err)
	}

	maximo := registry.DefaultMaxSessions
	if bruto := strings.TrimSpace(os.Getenv(envHeadlessMaxSessions)); bruto != "" {
		n, err := strconv.Atoi(bruto)
		if err != nil || n < 1 {
			return HeadlessConfig{}, fmt.Errorf("%s=%q invalido: use um inteiro >= 1", envHeadlessMaxSessions, bruto)
		}
		maximo = n
	}

	ua := strings.TrimSpace(os.Getenv(envHeadlessUserAgent))
	if ua == "" {
		ua = headlessDefaultUserAgent
	}
	return HeadlessConfig{ChromePath: chrome, ProfileRoot: raiz, MaxSessions: maximo, UserAgent: ua}, nil
}
