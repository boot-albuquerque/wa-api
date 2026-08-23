package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Testes da F169 — o token de administração e a chave de encriptação global
// não podem vazar em log, e quando são gerados têm de sair de um gerador
// criptográfico. A chave de encriptação, além disso, deixou de ser gerada:
// sem ela o processo não sobe.
//
// Os dois blocos testados viviam dentro de Main(), lendo ponteiros de flag do
// pacote: nenhum teste conseguia alcançá-los. Foram extraídos para
// `resolveGlobalEncryptionKey`/`resolveAdminToken` em startup_secrets.go
// exatamente para que estas asserções existam.

// tokenConfiguradoDeTeste é um valor reconhecível na saída de log: se ele
// aparecer lá, o caminho "configurado" virou o novo vazamento.
const tokenConfiguradoDeTeste = "token-de-admin-do-operador-nao-pode-vazar"

// chaveDeEncriptacaoConfiguradaDeTeste, idem para a chave de encriptação.
const chaveDeEncriptacaoConfiguradaDeTeste = "chave-de-encriptacao-do-operador-nao-pode-vazar"

// modoEsperadoDoArquivoDoToken é um LITERAL, e não `adminTokenFileMode`, de
// propósito.
//
// A primeira versão deste teste comparava o modo do arquivo com a própria
// constante de produção. O controle negativo (c) — trocar a constante para
// 0644 — passou verde: mudar a constante muda os DOIS lados da comparação, e a
// asserção era uma tautologia. Um teste que não morde é pior que nenhum
// (ARMADILHA 3), então o valor exigido está escrito aqui de forma
// independente. `TestF169_ConstanteDeModoDoArquivoDoTokenEh0600` amarra a
// constante de produção a este mesmo literal, para que a divergência apareça
// nomeando a causa em vez de só o sintoma.
const modoEsperadoDoArquivoDoToken os.FileMode = 0o600

// TestF169_ChaveDeEncriptacaoAusenteDerrubaOArranque é o teste do defeito na
// metade "fail-closed".
//
// A asserção sobre a SUBSTRING com o nome da variável é deliberada: renomear
// WA_API_GLOBAL_ENCRYPTION_KEY sem atualizar a mensagem deixaria o operador
// com um erro que nomeia uma variável inexistente, e é justamente o tipo de
// erro que faz alguém desistir e desligar a proteção.
func TestF169_ChaveDeEncriptacaoAusenteDerrubaOArranque(t *testing.T) {
	captureLogInto(t)

	chave, _, err := resolveGlobalEncryptionKey("", "")
	if err == nil {
		t.Fatalf("resolveGlobalEncryptionKey aceitou a ausência das duas fontes e devolveu %q; "+
			"gerar uma chave aqui invalidaria, a cada reinício, todo segredo já cifrado", chave)
	}
	if chave != "" {
		t.Errorf("chave = %q com erro presente; nenhum valor pode escapar do caminho de falha", chave)
	}
	if !strings.Contains(err.Error(), envGlobalEncryptionKey) {
		t.Errorf("a mensagem de erro não nomeia %s:\n%s", envGlobalEncryptionKey, err.Error())
	}
	if !strings.Contains(err.Error(), flagGlobalEncryptionKey) {
		t.Errorf("a mensagem de erro não nomeia a flag %s:\n%s", flagGlobalEncryptionKey, err.Error())
	}
}

// TestF169_ChaveDeEncriptacaoDoAmbienteEUsadaENaoEcoada cobre o caminho de
// SUCESSO por variável de ambiente (ARMADILHA 2: o caminho que não vazava por
// acidente é o que vira o próximo vazamento).
func TestF169_ChaveDeEncriptacaoDoAmbienteEUsadaENaoEcoada(t *testing.T) {
	captura := captureLogInto(t)

	chave, origem, err := resolveGlobalEncryptionKey("", chaveDeEncriptacaoConfiguradaDeTeste)
	if err != nil {
		t.Fatalf("resolveGlobalEncryptionKey devolveu erro: %v", err)
	}
	if chave != chaveDeEncriptacaoConfiguradaDeTeste {
		t.Errorf("chave = %q, esperada a do ambiente %q", chave, chaveDeEncriptacaoConfiguradaDeTeste)
	}
	if origem != encryptionKeyFromEnv {
		t.Errorf("origem = %v, esperada %v", origem, encryptionKeyFromEnv)
	}
	exigeLogSemSegredo(t, captura.String(), chave)
}

// TestF169_ChaveDeEncriptacaoDaLinhaDeComandoEUsadaENaoEcoada é o mesmo para a
// flag, que tem precedência sobre o ambiente.
func TestF169_ChaveDeEncriptacaoDaLinhaDeComandoEUsadaENaoEcoada(t *testing.T) {
	captura := captureLogInto(t)

	const valorDoAmbiente = "valor-do-ambiente-que-perde-para-a-flag"
	chave, origem, err := resolveGlobalEncryptionKey(chaveDeEncriptacaoConfiguradaDeTeste, valorDoAmbiente)
	if err != nil {
		t.Fatalf("resolveGlobalEncryptionKey devolveu erro: %v", err)
	}
	if chave != chaveDeEncriptacaoConfiguradaDeTeste {
		t.Errorf("chave = %q, esperada a da flag %q", chave, chaveDeEncriptacaoConfiguradaDeTeste)
	}
	if origem != encryptionKeyFromFlag {
		t.Errorf("origem = %v, esperada %v", origem, encryptionKeyFromFlag)
	}
	exigeLogSemSegredo(t, captura.String(), chave, valorDoAmbiente)
}

// TestF169_TokenGeradoVaiParaArquivo0600ENaoParaOLog é o teste do defeito na
// metade "canal deliberado".
//
// Ele assevera o MODO do arquivo, e não só a existência: um arquivo 0644 com o
// token dentro troca o vazamento por log por um vazamento por sistema de
// arquivos, e passaria em qualquer asserção de existência.
func TestF169_TokenGeradoVaiParaArquivo0600ENaoParaOLog(t *testing.T) {
	captura := captureLogInto(t)
	dir := t.TempDir()

	token, origem, err := resolveAdminToken("", "", dir)
	if err != nil {
		t.Fatalf("resolveAdminToken devolveu erro: %v", err)
	}
	if origem != adminTokenGenerated {
		t.Fatalf("origem = %v, esperada %v", origem, adminTokenGenerated)
	}
	if len(token) != generatedAdminTokenLength {
		t.Errorf("tamanho do token = %d, esperado %d", len(token), generatedAdminTokenLength)
	}

	caminho := filepath.Join(dir, adminTokenFileName)
	info, err := os.Stat(caminho)
	if err != nil {
		t.Fatalf("o arquivo do token não foi criado em %s: %v", caminho, err)
	}
	if modo := info.Mode().Perm(); modo != modoEsperadoDoArquivoDoToken {
		t.Errorf("modo do arquivo %s = %#o, esperado %#o — um token legível por outros usuários "+
			"apenas troca o canal do vazamento", caminho, modo, modoEsperadoDoArquivoDoToken)
	}
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	if string(conteudo) != token {
		t.Errorf("o arquivo contém %q, e o token devolvido é %q — o canal não serve se divergirem",
			string(conteudo), token)
	}

	saida := captura.String()
	if !strings.Contains(saida, caminho) {
		t.Errorf("o log não informa o CAMINHO do arquivo do token; sem ele o operador não tem "+
			"como recuperar o valor:\n%s", saida)
	}
	exigeLogSemSegredo(t, saida, token)
}

// TestF169_TokenGeradoSobrescreveArquivoPermissivoAnterior trava a ORDEM da
// correção: um arquivo deixado para trás por uma versão antiga, com 0644, tem
// de sair com 0600 e não apenas ser truncado — O_TRUNC preserva a permissão do
// inode existente.
func TestF169_TokenGeradoSobrescreveArquivoPermissivoAnterior(t *testing.T) {
	captureLogInto(t)
	dir := t.TempDir()
	caminho := filepath.Join(dir, adminTokenFileName)

	const modoPermissivo os.FileMode = 0o644
	if err := os.WriteFile(caminho, []byte("token-antigo"), modoPermissivo); err != nil {
		t.Fatalf("preparar o arquivo antigo: %v", err)
	}
	// Confere o cenário: se o umask tiver apertado o modo, o teste não estaria
	// medindo o que diz medir.
	if info, err := os.Stat(caminho); err != nil {
		t.Fatalf("stat do arquivo antigo: %v", err)
	} else if info.Mode().Perm() != modoPermissivo {
		t.Fatalf("o arquivo antigo nasceu com %#o e não com %#o; o cenário não existe",
			info.Mode().Perm(), modoPermissivo)
	}

	if _, _, err := resolveAdminToken("", "", dir); err != nil {
		t.Fatalf("resolveAdminToken devolveu erro: %v", err)
	}
	info, err := os.Stat(caminho)
	if err != nil {
		t.Fatalf("stat depois da geração: %v", err)
	}
	if modo := info.Mode().Perm(); modo != modoEsperadoDoArquivoDoToken {
		t.Errorf("modo do arquivo = %#o, esperado %#o — a permissão antiga sobreviveu", modo, modoEsperadoDoArquivoDoToken)
	}
}

// TestF169_TokenDoAmbienteEUsadoENaoEcoado cobre o caminho de sucesso por
// variável de ambiente: o token do operador é usado tal como veio, nada é
// gerado, nenhum arquivo é escrito, e o log não ecoa o valor.
func TestF169_TokenDoAmbienteEUsadoENaoEcoado(t *testing.T) {
	captura := captureLogInto(t)
	dir := t.TempDir()

	token, origem, err := resolveAdminToken("", tokenConfiguradoDeTeste, dir)
	if err != nil {
		t.Fatalf("resolveAdminToken devolveu erro: %v", err)
	}
	if token != tokenConfiguradoDeTeste {
		t.Errorf("token = %q, esperado o do ambiente %q — algo foi gerado no lugar", token, tokenConfiguradoDeTeste)
	}
	if origem != adminTokenFromEnv {
		t.Errorf("origem = %v, esperada %v", origem, adminTokenFromEnv)
	}
	if _, err := os.Stat(filepath.Join(dir, adminTokenFileName)); !os.IsNotExist(err) {
		t.Errorf("um arquivo de token foi escrito para um token que o operador já conhece (err=%v)", err)
	}
	exigeLogSemSegredo(t, captura.String(), token)
}

// TestF169_TokenDaLinhaDeComandoEUsadoENaoEcoado é o mesmo para a flag, que
// tem precedência sobre o ambiente.
func TestF169_TokenDaLinhaDeComandoEUsadoENaoEcoado(t *testing.T) {
	captura := captureLogInto(t)
	dir := t.TempDir()

	const valorDoAmbiente = "token-do-ambiente-que-perde-para-a-flag"
	token, origem, err := resolveAdminToken(tokenConfiguradoDeTeste, valorDoAmbiente, dir)
	if err != nil {
		t.Fatalf("resolveAdminToken devolveu erro: %v", err)
	}
	if token != tokenConfiguradoDeTeste {
		t.Errorf("token = %q, esperado o da flag %q", token, tokenConfiguradoDeTeste)
	}
	if origem != adminTokenFromFlag {
		t.Errorf("origem = %v, esperada %v", origem, adminTokenFromFlag)
	}
	exigeLogSemSegredo(t, captura.String(), token, valorDoAmbiente)
}

// TestF169_DuasGeracoesProduzemTokensDiferentes é sanidade do gerador: uma
// constante disfarçada de token passaria em todos os outros testes.
func TestF169_DuasGeracoesProduzemTokensDiferentes(t *testing.T) {
	captureLogInto(t)

	primeiro, err := generateSecret(generatedAdminTokenLength)
	if err != nil {
		t.Fatalf("primeira geração falhou: %v", err)
	}
	segundo, err := generateSecret(generatedAdminTokenLength)
	if err != nil {
		t.Fatalf("segunda geração falhou: %v", err)
	}
	if primeiro == segundo {
		t.Errorf("duas gerações consecutivas devolveram o MESMO token (%q)", primeiro)
	}
	for i, c := range primeiro {
		if !strings.ContainsRune(secretCharset, c) {
			t.Errorf("caractere %q na posição %d está fora do alfabeto declarado", c, i)
		}
	}
}

// TestF169_ConstanteDeModoDoArquivoDoTokenEh0600 amarra a constante de
// produção ao valor exigido. Ela existe separada da asserção sobre o arquivo
// para que afrouxar a permissão falhe DUAS vezes, e a segunda falha diga qual
// linha causou a primeira.
func TestF169_ConstanteDeModoDoArquivoDoTokenEh0600(t *testing.T) {
	if adminTokenFileMode != modoEsperadoDoArquivoDoToken {
		t.Errorf("adminTokenFileMode = %#o, esperado %#o — o arquivo do token de administração "+
			"não pode ser legível por outro usuário da máquina", adminTokenFileMode, modoEsperadoDoArquivoDoToken)
	}
}

// TestF169_GeradorEhCriptografico trava a segunda metade do achado.
//
// # Por que um teste ESTRUTURAL e não comportamental
//
// Uma amostra de saída de math/rand e uma de crypto/rand são INDISTINGUÍVEIS:
// ambas passam em "duas gerações diferem", em "tamanho esperado" e em qualquer
// teste de distribuição que caiba num teste unitário. Não existe asserção sobre
// o VALOR devolvido que morda a troca de gerador — e um teste que passa por
// acaso é pior que nenhum.
//
// O que dá para travar é a FONTE. Este teste lê o arquivo do gerador e exige
// crypto/rand, recusando math/rand. Ele foi o único controle negativo que
// mordeu no CAP-34, e a razão continua valendo.
func TestF169_GeradorEhCriptografico(t *testing.T) {
	const arquivoDoGerador = "startup_secrets.go"

	conteudo, err := os.ReadFile(arquivoDoGerador)
	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", arquivoDoGerador, err)
	}
	fonte := string(conteudo)

	if !strings.Contains(fonte, `cryptorand "crypto/rand"`) {
		t.Errorf("%s não importa crypto/rand: token de administração e chave AES precisam de CSPRNG", arquivoDoGerador)
	}
	for _, proibido := range []string{`"math/rand"`, "rand.Intn(", "rand.Int63n("} {
		if strings.Contains(fonte, proibido) {
			t.Errorf("%s usa %s: math/rand não é CSPRNG e não pode gerar credencial",
				arquivoDoGerador, proibido)
		}
	}
}

// exigeLogSemSegredo assevera sobre a LINHA INTEIRA capturada, e não sobre um
// campo nomeado: mover o segredo para outro campo, ou para dentro do texto da
// mensagem, continua sendo vazamento e continua sendo pego.
//
// Também recusa prefixo e sufixo de 8 caracteres — informação sobre o segredo
// é informação que o log foi escrito para não ter.
func exigeLogSemSegredo(t *testing.T, saida string, segredos ...string) {
	t.Helper()

	if saida == "" {
		t.Fatal("nada foi capturado: sem registro de log a asserção não mede nada")
	}
	for _, segredo := range segredos {
		if segredo == "" {
			continue
		}
		if strings.Contains(saida, segredo) {
			t.Errorf("o segredo %q aparece no log:\n%s", segredo, saida)
		}
		if len(segredo) < 16 {
			continue
		}
		for _, pedaco := range []string{segredo[:8], segredo[len(segredo)-8:]} {
			if strings.Contains(saida, pedaco) {
				t.Errorf("um prefixo/sufixo do segredo aparece no log (%q):\n%s", pedaco, saida)
			}
		}
	}
}
