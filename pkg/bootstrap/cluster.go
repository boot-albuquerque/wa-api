package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// Modo de cluster (ADR-0005, D1).
//
// O alvo é rodar em N pods, mas o núcleo — manter sessões e entregar eventos —
// nunca exige mais que SQLite e um processo. Os dois extremos são modos
// suportados, e o que os separa é uma decisão EXPLÍCITA, nunca inferida.
//
// Isto existe por causa de dois modos de falha SILENCIOSOS medidos na F89:
//
//  1. A instalação de produção roda em SQLite sem que ninguém tenha pedido: as
//     variáveis de Postgres estavam PARCIALMENTE definidas e o processo
//     degradou com um `warn`. Em N pods isso seria cada réplica com um banco
//     próprio, todas se achando donas de tudo.
//  2. Duas réplicas na mesma sessão não brigam em laço, como se supunha. O
//     WhatsApp manda UM `StreamReplaced`, o perdedor fica com a sessão morta e
//     NUNCA tenta reconectar — processo vivo, HTTP respondendo, liveness
//     verde, sessão morta. E o banco continua dizendo `connected=1`.
//
// Nos dois casos o sistema seguiu de pé mentindo sobre o próprio estado. A
// resposta aqui é recusar o arranque em vez de degradar.

const (
	envClusterMode = "WA_API_CLUSTER_MODE"

	// clusterModeSingle é o padrão, e é o modo do cenário catastrófico:
	// SQLite, um processo, sem coordenação. Continua funcional porque o núcleo
	// não precisa de mais que isso — ver o princípio do ADR-0005.
	clusterModeSingle = "single"

	// clusterModeMulti admite N processos e, por isso, EXIGE um banco que
	// coordene. É onde o lease da D2 passa a valer.
	clusterModeMulti = "multi"

	// arquivoTravaInstancia fica no diretório de dados porque é ele que
	// identifica a instalação: dois processos com data dirs diferentes são
	// instalações diferentes e não conflitam.
	arquivoTravaInstancia = ".wa-api-instance.lock"
)

// clusterModeConfigurado lê o modo do ambiente.
//
// Valor ausente é `single` — o padrão seguro, alinhado a "por padrão sempre
// SQLite". Valor DESCONHECIDO é erro, e não cai no padrão: um typo em
// `WA_API_CLUSTER_MODE=mutli` num manifesto de k8s não pode virar
// silenciosamente um processo que se acha dono de tudo.
func clusterModeConfigurado() (string, error) {
	bruto := strings.ToLower(strings.TrimSpace(os.Getenv(envClusterMode)))
	switch bruto {
	case "":
		return clusterModeSingle, nil
	case clusterModeSingle, clusterModeMulti:
		return bruto, nil
	default:
		return "", fmt.Errorf("%s=%q invalido: use %q ou %q", envClusterMode, bruto, clusterModeSingle, clusterModeMulti)
	}
}

// validarStackDoModo recusa combinações que não podem funcionar.
//
// Em `multi`, SQLite não é degradação aceitável: é corrupção esperando. Cada
// réplica teria o próprio arquivo, o próprio conjunto de sessões e nenhuma
// forma de saber da outra. O erro sobe para o chamador matar o processo.
func validarStackDoModo(modo, tipoBanco string) error {
	if modo == clusterModeMulti && tipoBanco != "postgres" {
		return fmt.Errorf(
			"%s=%s exige Postgres, mas o banco resolvido foi %q: defina DB_USER, DB_PASSWORD, DB_NAME, DB_HOST e DB_PORT (todas, nao um subconjunto)",
			envClusterMode, clusterModeMulti, tipoBanco)
	}
	return nil
}

// travarInstanciaUnica garante que só um processo use este diretório de dados.
//
// `flock` e não arquivo-com-PID: a trava do sistema operacional é liberada
// AUTOMATICAMENTE quando o processo morre, inclusive num `kill -9`. Um arquivo
// com PID exigiria detectar trava obsoleta, que é onde esse tipo de mecanismo
// costuma falhar — e falhar liberando quando não devia.
//
// LIMITAÇÃO, e ela é importante: isto protege contra um segundo processo NA
// MESMA MÁQUINA. Duas máquinas apontando para o mesmo Postgres em modo
// `single` não são detectadas aqui — é para isso que existe o modo `multi`
// com lease (D2 do ADR-0005). O caso que esta trava cobre é o que de fato
// acontece: alguém sobe um segundo processo sem perceber.
func travarInstanciaUnica(dirDados string) (func(), error) {
	if err := os.MkdirAll(dirDados, 0o755); err != nil {
		return nil, fmt.Errorf("criando diretorio de dados %s: %w", dirDados, err)
	}
	caminho := filepath.Join(dirDados, arquivoTravaInstancia)

	f, err := os.OpenFile(caminho, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("abrindo trava de instancia %s: %w", caminho, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf(
			"outro processo ja' usa o diretorio de dados %s: em %s=%s so' um processo pode rodar por instalacao (use %s=%s com Postgres para N replicas)",
			dirDados, envClusterMode, clusterModeSingle, envClusterMode, clusterModeMulti)
	}

	// Registrar o PID é diagnóstico, não controle: quem decide é o flock.
	if err := f.Truncate(0); err == nil {
		if _, err := f.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0); err != nil {
			log.Warn().Err(err).Str("arquivo", caminho).Msg("nao consegui registrar o PID na trava de instancia")
		}
	}

	return func() {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			log.Warn().Err(err).Str("arquivo", caminho).Msg("falha ao liberar a trava de instancia")
		}
		if err := f.Close(); err != nil {
			log.Warn().Err(err).Str("arquivo", caminho).Msg("falha ao fechar a trava de instancia")
		}
	}, nil
}

// prepararCluster resolve o modo, valida a stack e trava a instância quando o
// modo for `single`. Devolve a função de liberação.
//
// Chamado ANTES de abrir o banco: uma configuração impossível tem de morrer
// sem ter tocado em estado nenhum.
func prepararCluster(dirDados, tipoBanco string) (func(), error) {
	modo, err := clusterModeConfigurado()
	if err != nil {
		return nil, err
	}
	if err := validarStackDoModo(modo, tipoBanco); err != nil {
		return nil, err
	}

	if modo == clusterModeMulti {
		// Em `multi` quem garante exclusividade é o lease por sessão (D2), que
		// ainda não existe. Enquanto não existir, este modo NÃO deve ser usado
		// com mais de uma réplica — e o aviso é a única barreira honesta.
		log.Warn().
			Str("modo", modo).
			Msg("modo multi: a posse de sessao por lease (ADR-0005 D2) ainda nao esta' implementada; nao suba mais de uma replica")
		return func() {}, nil
	}

	liberar, err := travarInstanciaUnica(dirDados)
	if err != nil {
		return nil, err
	}
	log.Info().Str("modo", modo).Str("dir_dados", dirDados).Msg("instancia unica travada")
	return liberar, nil
}
