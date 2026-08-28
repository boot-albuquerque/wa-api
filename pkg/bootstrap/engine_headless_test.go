package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/wa-headless/registry"
)

func chromeFalso(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// Nenhuma das duas variáveis presentes: a configuração fica zero, sem erro.
// Um processo sem Chrome apontado não pode falhar no arranque por uma
// capacidade que ninguém configurou — quem recusa um pedido de sessão em
// headless é AddUserUseCase, na hora do pedido (decisão 94 removida).
func TestConfiguracaoHeadlessNaoEExigidaQuandoNinguemAConfigurou(t *testing.T) {
	t.Setenv(envHeadlessChrome, "")
	t.Setenv(envHeadlessProfiles, "")
	cfg, err := headlessConfigConfigurada()
	if err != nil {
		t.Fatalf("configuração ausente virou erro sem nada configurado: %v", err)
	}
	if cfg.ChromePath != "" {
		t.Fatalf("ChromePath = %q, queria vazio", cfg.ChromePath)
	}
}

// Uma das duas variáveis presente sem a outra é configuração PARCIAL, e
// continua fatal: quem configurou uma pretendia configurar o headless.
func TestConfiguracaoParcialERecusada(t *testing.T) {
	t.Setenv(envHeadlessChrome, "")
	t.Setenv(envHeadlessProfiles, "/tmp/perfis")
	if _, err := headlessConfigConfigurada(); err == nil {
		t.Fatal("perfis configurado sem Chrome passou")
	}
	t.Setenv(envHeadlessChrome, chromeFalso(t))
	t.Setenv(envHeadlessProfiles, "")
	if _, err := headlessConfigConfigurada(); err == nil {
		t.Fatal("Chrome configurado sem raiz de perfis passou")
	}
}

func TestCaminhoDeChromeQuebradoERecusadoNoArranque(t *testing.T) {
	t.Setenv(envHeadlessProfiles, t.TempDir())
	t.Setenv(envHeadlessChrome, filepath.Join(t.TempDir(), "nao-existe"))
	if _, err := headlessConfigConfigurada(); err == nil {
		t.Fatal("um caminho de Chrome inexistente passou")
	}
	t.Setenv(envHeadlessChrome, t.TempDir())
	if _, err := headlessConfigConfigurada(); err == nil {
		t.Fatal("um DIRETÓRIO passou por executável")
	}
}

func TestPadroesSaoOsMedidos(t *testing.T) {
	t.Setenv(envHeadlessChrome, chromeFalso(t))
	t.Setenv(envHeadlessProfiles, t.TempDir())
	t.Setenv(envHeadlessMaxSessions, "")
	t.Setenv(envHeadlessUserAgent, "")
	cfg, err := headlessConfigConfigurada()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxSessions != registry.DefaultMaxSessions {
		t.Fatalf("MaxSessions = %d, queria o medido %d", cfg.MaxSessions, registry.DefaultMaxSessions)
	}
	if !strings.Contains(cfg.UserAgent, "Chrome/") || strings.Contains(cfg.UserAgent, "Headless") {
		t.Fatalf("o user agent padrão não é o token medido: %q", cfg.UserAgent)
	}
}

func TestTetoDeSessoesInvalidoERecusado(t *testing.T) {
	t.Setenv(envHeadlessChrome, chromeFalso(t))
	t.Setenv(envHeadlessProfiles, t.TempDir())
	for _, mau := range []string{"0", "-1", "quatro", "1.5"} {
		t.Setenv(envHeadlessMaxSessions, mau)
		if _, err := headlessConfigConfigurada(); err == nil {
			t.Errorf("%s=%q foi aceito", envHeadlessMaxSessions, mau)
		}
	}
}

// --- o txtID vira caminho, e é aí que mora o problema ---
//
// Um perfil é CREDENCIAL de sessão. Travessia aqui não seria um bug de caminho:
// seria escolher qual conta restaurar.

func TestTxtIDNaoESCAPADaRaizDePerfis(t *testing.T) {
	cfg := HeadlessConfig{ChromePath: "/bin/true", ProfileRoot: "/var/perfis", UserAgent: "ua"}
	maus := []string{"..", ".", "../outra", "a/b", `a\b`, "/etc/passwd", "", "   ",
		"../../.lab/conta-A", "conta\x00A"}
	for _, mau := range maus {
		if _, err := cfg.StartConfigFor(mau); err == nil {
			t.Errorf("txtID %q foi aceito e viraria caminho de perfil", mau)
		}
	}
}

func TestTxtIDSimplesViraPerfilSobARaiz(t *testing.T) {
	cfg := HeadlessConfig{ChromePath: "/bin/true", ProfileRoot: "/var/perfis", UserAgent: "ua"}
	sc, err := cfg.StartConfigFor("sessao-1")
	if err != nil {
		t.Fatalf("um txtID simples foi recusado: %v", err)
	}
	if sc.ProfileDir != filepath.Join("/var/perfis", "sessao-1") {
		t.Fatalf("ProfileDir = %q", sc.ProfileDir)
	}
	if sc.NavigateURL != headlessTargetURL {
		t.Fatalf("NavigateURL = %q, queria a SPA de produção", sc.NavigateURL)
	}
	if sc.BinaryPath != "/bin/true" || sc.UserAgent != "ua" {
		t.Fatal("a configuração não chegou inteira ao StartConfig")
	}
}

// Sanitizar em vez de recusar faria dois txtID diferentes colidirem no mesmo
// perfil — duas sessões a partilhar credencial, violando a invariante 13 sem
// que nada o dissesse.
func TestDoisTxtIDDiferentesNuncaVaoParaOMesmoPerfil(t *testing.T) {
	cfg := HeadlessConfig{ChromePath: "/bin/true", ProfileRoot: "/var/perfis", UserAgent: "ua"}
	vistos := map[string]string{}
	for _, id := range []string{"a-b", "a_b", "a.b", "aXb", "AB", "ab"} {
		sc, err := cfg.StartConfigFor(id)
		if err != nil {
			continue
		}
		if anterior, colidiu := vistos[sc.ProfileDir]; colidiu {
			t.Fatalf("txtID %q e %q foram para o MESMO perfil %q", anterior, id, sc.ProfileDir)
		}
		vistos[sc.ProfileDir] = id
	}
}

// A recusa não pode carregar o txtID para o log: um identificador de sessão
// recusado ainda é dado de quem chamou, e o que se investiga com ele é o
// MOTIVO, não o valor.
func TestARecusaDePerfilNaoVAZAOTxtID(t *testing.T) {
	var buf bytes.Buffer
	anterior := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = anterior }()

	cfg := HeadlessConfig{ChromePath: "/bin/true", ProfileRoot: "/var/perfis", UserAgent: "ua"}
	const veneno = "../../.lab/conta-A"
	if _, err := cfg.StartConfigFor(veneno); err == nil {
		t.Fatal("a travessia foi aceita")
	}
	saida := buf.String()
	if saida == "" {
		t.Fatal("a recusa não foi registrada: quem opera não veria a tentativa")
	}
	if strings.Contains(saida, veneno) || strings.Contains(saida, "conta-A") {
		t.Fatalf("o log vazou o txtID recusado: %s", saida)
	}
	if !strings.Contains(saida, "warn") {
		t.Fatalf("a recusa não é Warn: %s", saida)
	}
}
