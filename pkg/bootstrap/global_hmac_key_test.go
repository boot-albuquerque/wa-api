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

// chaveDeEncriptacaoDeTeste tem 32 bytes porque o AES de auth.EncryptHMACKey
// aceita 16, 24 ou 32.
const chaveDeEncriptacaoDeTeste = "0123456789abcdef0123456789abcdef"

// valorConfiguradoDeTeste é um valor de chave reconhecível na saída de log:
// se ele aparecer lá, o caminho "configurada" virou o novo vazamento.
const valorConfiguradoDeTeste = "chave-hmac-do-operador-nao-pode-vazar"

// TestF156_ChaveGeradaNaoApareceNoLog é o teste do defeito.
//
// A asserção é sobre a LINHA INTEIRA capturada, não sobre o campo
// `global_hmac_key` que existia antes: mover o segredo para outro campo, ou
// para dentro da mensagem, continua sendo vazamento e continua sendo pego.
func TestF156_ChaveGeradaNaoApareceNoLog(t *testing.T) {
	captura := captureLogInto(t)

	chave, origem, err := resolveGlobalHMACKey("", "")
	if err != nil {
		t.Fatalf("resolveGlobalHMACKey devolveu erro: %v", err)
	}
	if origem != hmacKeyGenerated {
		t.Fatalf("origem = %v, esperada %v", origem, hmacKeyGenerated)
	}

	saida := captura.String()
	if saida == "" {
		t.Fatal("nada foi capturado: sem registro de log a asserção não mede nada")
	}
	if strings.Contains(saida, chave) {
		t.Errorf("a chave HMAC gerada aparece no log:\n%s", saida)
	}
	// O tamanho em caracteres do segredo também não: é informação sobre a
	// chave, e a mensagem foi escrita para não ter nenhuma.
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
