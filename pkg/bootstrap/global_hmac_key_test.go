package bootstrap

import (
	"os"
	"strings"
	"testing"

	"wa-api/pkg/infra/auth"
)

// Testes da F156 — a chave HMAC GLOBAL não pode vazar em log, e quando é
// gerada tem de sair de um gerador criptográfico.
//
// O bloco testado vivia dentro de Main(), lendo ponteiros de flag do pacote:
// nenhum teste conseguia alcançá-lo. Foi extraído para
// `resolveGlobalHMACKey`/`generateGlobalHMACKey` em global_hmac_key.go
// exatamente para que estas asserções existam.
//
// Com o fail-closed (F156, opção 1), resolveGlobalHMACKey já não chama
// generateGlobalHMACKey. Os testes do gerador ficam porque travam propriedades
// reais da função que ainda existe (ver comentário em global_hmac_key.go).

// chaveDeEncriptacaoDeTeste tem 32 bytes porque o AES de auth.EncryptHMACKey
// aceita 16, 24 ou 32.
const chaveDeEncriptacaoDeTeste = "0123456789abcdef0123456789abcdef"

// valorConfiguradoDeTeste é um valor de chave reconhecível na saída de log:
// se ele aparecer lá, o caminho "configurada" virou o novo vazamento.
const valorConfiguradoDeTeste = "chave-hmac-do-operador-nao-pode-vazar"

// TestF156_ChaveGeradaNaoApareceNoLog tests that generateGlobalHMACKey does
// not leak the key value into any log line. The function has no production
// caller since the fail-closed change (F156), but the property is still worth
// locking because the function is kept for its other tests.
func TestF156_ChaveGeradaNaoApareceNoLog(t *testing.T) {
	captura := captureLogInto(t)

	chave, err := generateGlobalHMACKey()
	if err != nil {
		t.Fatalf("generateGlobalHMACKey returned error: %v", err)
	}

	saida := captura.String()
	if strings.Contains(saida, chave) {
		t.Errorf("a chave HMAC gerada aparece no log:\n%s", saida)
	}
	for _, pedaco := range []string{chave[:8], chave[len(chave)-8:]} {
		if strings.Contains(saida, pedaco) {
			t.Errorf("um prefixo/sufixo da chave gerada aparece no log (%q):\n%s", pedaco, saida)
		}
	}
}

// TestF156_DuasGeracoesProduzemChavesDiferentes é sanidade do gerador: uma
// constante disfarçada de chave passaria em todos os outros testes.
func TestF156_DuasGeracoesProduzemChavesDiferentes(t *testing.T) {
	captureLogInto(t)

	primeira, err := generateGlobalHMACKey()
	if err != nil {
		t.Fatalf("primeira geração falhou: %v", err)
	}
	segunda, err := generateGlobalHMACKey()
	if err != nil {
		t.Fatalf("segunda geração falhou: %v", err)
	}
	if primeira == segunda {
		t.Errorf("duas gerações consecutivas devolveram a MESMA chave (%q)", primeira)
	}
}

// TestF156_ChaveGeradaTemFormatoUtilizavel prova que o formato novo não quebrou
// o consumidor: a chave é do tamanho esperado, é alfanumérica, e faz a ida e a
// volta por auth.EncryptHMACKey/DecryptHMACKey — o par que Main() usa logo
// depois de resolver a chave (main.go, encryptHMACKey).
func TestF156_ChaveGeradaTemFormatoUtilizavel(t *testing.T) {
	captureLogInto(t)

	chave, err := generateGlobalHMACKey()
	if err != nil {
		t.Fatalf("geração falhou: %v", err)
	}
	if len(chave) != generatedHMACKeyLength {
		t.Errorf("tamanho da chave = %d, esperado %d", len(chave), generatedHMACKeyLength)
	}
	for i, c := range chave {
		if !strings.ContainsRune(hmacKeyCharset, c) {
			t.Errorf("caractere %q na posição %d está fora do alfabeto declarado", c, i)
		}
	}

	cifrada, err := auth.EncryptHMACKey(chave, chaveDeEncriptacaoDeTeste)
	if err != nil {
		t.Fatalf("auth.EncryptHMACKey recusou a chave gerada: %v", err)
	}
	devolvida, err := auth.DecryptHMACKey(cifrada, []byte(chaveDeEncriptacaoDeTeste))
	if err != nil {
		t.Fatalf("auth.DecryptHMACKey falhou: %v", err)
	}
	if devolvida != chave {
		t.Errorf("ida e volta mudou a chave: %q != %q", devolvida, chave)
	}
}

// TestF156_ChaveDoAmbienteEUsadaENaoEcoada cobre o caminho "configurada por
// variável de ambiente": a chave do operador tem de ser usada tal como veio,
// nada pode ser gerado, e o log não pode ecoar o valor dela. O caminho que não
// vazava por acidente é o que vira o próximo vazamento.
func TestF156_ChaveDoAmbienteEUsadaENaoEcoada(t *testing.T) {
	captura := captureLogInto(t)

	chave, origem, err := resolveGlobalHMACKey("", valorConfiguradoDeTeste)
	if err != nil {
		t.Fatalf("resolveGlobalHMACKey devolveu erro: %v", err)
	}
	if chave != valorConfiguradoDeTeste {
		t.Errorf("chave = %q, esperada a do ambiente %q — algo foi gerado no lugar",
			chave, valorConfiguradoDeTeste)
	}
	if origem != hmacKeyFromEnv {
		t.Errorf("origem = %v, esperada %v", origem, hmacKeyFromEnv)
	}
	if saida := captura.String(); strings.Contains(saida, valorConfiguradoDeTeste) {
		t.Errorf("a chave do ambiente aparece no log:\n%s", saida)
	}
}

// TestF156_ChaveDaLinhaDeComandoEUsadaENaoEcoada é o mesmo para a flag, que
// tem precedência sobre o ambiente.
func TestF156_ChaveDaLinhaDeComandoEUsadaENaoEcoada(t *testing.T) {
	captura := captureLogInto(t)

	const valorDoAmbiente = "valor-do-ambiente-que-perde-para-a-flag"
	chave, origem, err := resolveGlobalHMACKey(valorConfiguradoDeTeste, valorDoAmbiente)
	if err != nil {
		t.Fatalf("resolveGlobalHMACKey devolveu erro: %v", err)
	}
	if chave != valorConfiguradoDeTeste {
		t.Errorf("chave = %q, esperada a da flag %q", chave, valorConfiguradoDeTeste)
	}
	if origem != hmacKeyFromFlag {
		t.Errorf("origem = %v, esperada %v", origem, hmacKeyFromFlag)
	}
	saida := captura.String()
	if strings.Contains(saida, valorConfiguradoDeTeste) {
		t.Errorf("a chave da linha de comando aparece no log:\n%s", saida)
	}
	if strings.Contains(saida, valorDoAmbiente) {
		t.Errorf("o valor do ambiente, descartado, aparece no log:\n%s", saida)
	}
}

// TestF156_GeradorEhCriptografico trava a segunda metade do achado.
//
// # Por que um teste ESTRUTURAL e não comportamental
//
// Uma amostra de saída de math/rand e uma de crypto/rand são
// INDISTINGUÍVEIS: ambas passam em "duas gerações diferem", em "tamanho
// esperado" e em qualquer teste de distribuição que caiba num teste unitário.
// Não existe asserção sobre o VALOR devolvido que morda a troca de gerador —
// e um teste que passa por acaso é pior que nenhum.
//
// O que dá para travar é a FONTE. Este teste lê o arquivo do gerador e exige
// crypto/rand, recusando math/rand. Ele morde na hora em que alguém reverte a
// correção, que é o controle negativo (b) da política anti-regressão.
func TestF156_GeradorEhCriptografico(t *testing.T) {
	const arquivoDoGerador = "global_hmac_key.go"

	conteudo, err := os.ReadFile(arquivoDoGerador)
	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", arquivoDoGerador, err)
	}
	fonte := string(conteudo)

	if !strings.Contains(fonte, `cryptorand "crypto/rand"`) {
		t.Errorf("%s não importa crypto/rand: a chave HMAC global precisa de CSPRNG", arquivoDoGerador)
	}
	for _, proibido := range []string{`"math/rand"`, "rand.Intn(", "rand.Int63n("} {
		if strings.Contains(fonte, proibido) {
			t.Errorf("%s usa %s: math/rand não é CSPRNG e não pode gerar chave de assinatura",
				arquivoDoGerador, proibido)
		}
	}
}

// --- F156 fail-closed tests -------------------------------------------------

// TestF156_FailClosed_SemFlagSemAmbienteDevolvErro is the primary assertion
// for the fail-closed decision: without flag and without environment,
// resolveGlobalHMACKey returns an error — not an empty string, not a generated
// value.
func TestF156_FailClosed_SemFlagSemAmbienteDevolvErro(t *testing.T) {
	captureLogInto(t)

	chave, _, err := resolveGlobalHMACKey("", "")
	if err == nil {
		t.Fatal("resolveGlobalHMACKey(\"\", \"\") did not return an error: the fail-closed path is broken")
	}
	if chave != "" {
		t.Errorf("resolveGlobalHMACKey returned a non-empty key %q alongside the error", chave)
	}
}

// TestF156_FailClosed_MensagemEhAcionavel asserts that the error message names
// both the environment variable and the flag — an error that does not say what
// to set forces the operator to read source code.
func TestF156_FailClosed_MensagemEhAcionavel(t *testing.T) {
	captureLogInto(t)

	_, _, err := resolveGlobalHMACKey("", "")
	if err == nil {
		t.Fatal("resolveGlobalHMACKey(\"\", \"\") did not return an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, envGlobalHMACKey) {
		t.Errorf("the error does not mention the environment variable %q:\n%s",
			envGlobalHMACKey, msg)
	}
	if !strings.Contains(msg, flagGlobalHMACKey) {
		t.Errorf("the error does not mention the flag %q:\n%s",
			flagGlobalHMACKey, msg)
	}
}

// TestF156_FailClosed_CaminhosConfiguradosNaoRegredem asserts that the flag
// and environment paths still work and that the flag takes precedence — the
// fail-closed change must not break existing deployments that already set
// the variable.
func TestF156_FailClosed_CaminhosConfiguradosNaoRegredem(t *testing.T) {
	captureLogInto(t)

	const (
		fromEnv  = "env-key-for-precedence-test"
		fromFlag = "flag-key-for-precedence-test"
	)

	t.Run("env_only", func(t *testing.T) {
		chave, origem, err := resolveGlobalHMACKey("", fromEnv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chave != fromEnv {
			t.Errorf("key = %q, want %q", chave, fromEnv)
		}
		if origem != hmacKeyFromEnv {
			t.Errorf("source = %v, want %v", origem, hmacKeyFromEnv)
		}
	})

	t.Run("flag_only", func(t *testing.T) {
		chave, origem, err := resolveGlobalHMACKey(fromFlag, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chave != fromFlag {
			t.Errorf("key = %q, want %q", chave, fromFlag)
		}
		if origem != hmacKeyFromFlag {
			t.Errorf("source = %v, want %v", origem, hmacKeyFromFlag)
		}
	})

	t.Run("flag_beats_env", func(t *testing.T) {
		chave, origem, err := resolveGlobalHMACKey(fromFlag, fromEnv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if chave != fromFlag {
			t.Errorf("key = %q, want flag %q (flag must take precedence)", chave, fromFlag)
		}
		if origem != hmacKeyFromFlag {
			t.Errorf("source = %v, want %v", origem, hmacKeyFromFlag)
		}
	})
}
