package bootstrap

import (
	"crypto/aes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O run.sh é o caminho documentado para levantar o ambiente local, e o .env
// que ele gera alimenta WA_API_GLOBAL_ENCRYPTION_KEY (main.go:238).
//
// Até a F67 esse template trazia uma chave de 34 bytes anunciando
// "De32Caracteres" no próprio valor. AES aceita 16, 24 ou 32 — o startup
// logava `invalid key size 34`, e SUBIA assim mesmo, deixando a chave HMAC
// global sem ser gravada. Quem seguisse o caminho documentado ficava com o
// HMAC silenciosamente inerte.
//
// Corrigir o valor sem travá-lo deixaria o mesmo erro voltar na próxima vez
// que alguém editasse o template — e ele é invisível a olho nu, porque o
// texto da chave afirma um tamanho que ela não tem.

// chavesDoRunSh extrai as chaves criptográficas do template embutido no
// run.sh, na raiz do repositório.
func chavesDoRunSh(t *testing.T) map[string]string {
	t.Helper()

	// O teste roda em pkg/bootstrap; o run.sh está dois níveis acima.
	caminho := filepath.Join("..", "..", "run.sh")
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}

	chaves := map[string]string{}
	for _, linha := range strings.Split(string(conteudo), "\n") {
		linha = strings.TrimSpace(linha)
		if strings.HasPrefix(linha, "#") {
			continue
		}
		nome, valor, ok := strings.Cut(linha, "=")
		if !ok || !strings.HasSuffix(nome, "_KEY") {
			continue
		}
		chaves[nome] = valor
	}
	if len(chaves) == 0 {
		t.Fatal("nenhuma chave *_KEY encontrada no run.sh; o template mudou de forma")
	}
	return chaves
}

// TestRunSh_ChavesTemTamanhoValidoParaAES trava o defeito da F67.
func TestRunSh_ChavesTemTamanhoValidoParaAES(t *testing.T) {
	for nome, valor := range chavesDoRunSh(t) {
		t.Run(nome, func(t *testing.T) {
			// A verificação é o próprio construtor de cifra, e não uma
			// comparação com 32: é ele que decide o que é válido, e replicar
			// a regra aqui abriria espaço para as duas divergirem.
			if _, err := aes.NewCipher([]byte(valor)); err != nil {
				t.Errorf("%s tem %d bytes e o AES a rejeita: %v\n"+
					"AES aceita 16, 24 ou 32 bytes. Conte os bytes — não confie no texto da chave.",
					nome, len(valor), err)
			}
		})
	}
}

// TestRunSh_ChaveNaoMenteSobreOProprioTamanho: o valor anterior dizia
// "De32Caracteres" e tinha 34 bytes. Se o texto da chave afirma um tamanho,
// ele tem de ser verdade — foi essa contradição que fez ninguém desconfiar.
func TestRunSh_ChaveNaoMenteSobreOProprioTamanho(t *testing.T) {
	for nome, valor := range chavesDoRunSh(t) {
		if !strings.Contains(valor, "32") {
			continue
		}
		if len(valor) != 32 {
			t.Errorf("%s contém \"32\" no texto mas tem %d bytes", nome, len(valor))
		}
	}
}
