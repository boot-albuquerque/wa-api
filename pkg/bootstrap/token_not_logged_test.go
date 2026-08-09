package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTokenNaoSaiEmLog varre o código de produção e falha se alguém logar o
// token da API.
//
// Havia TRÊS sítios, todos em nível Info e todos em caminho rotineiro: o
// "QR Pair Success", o "User information set" e o "Connect to Whatsapp on
// startup". Quem lesse o log podia agir como o usuário — é a mesma classe da
// F76, que tirou os códigos de pareamento do log pelo mesmo motivo.
//
// # Por que uma varredura de fonte e não um teste de comportamento
//
// Um teste que capture a saída de log prova o sítio que ele exercita, e o
// defeito aqui é de CLASSE: o próximo `Str("token", ...)` entra num caminho que
// nenhum teste existente percorre. A varredura cobre o código todo, inclusive o
// que ainda não foi escrito.
//
// É deliberadamente rasa — casa o padrão textual, não a semântica. O falso
// positivo custa uma linha de exceção; o falso negativo custa credencial em log.
func TestTokenNaoSaiEmLog(t *testing.T) {
	// Varre pkg/ INTEIRO, e não só este pacote: os três sítios estavam aqui,
	// mas a classe não é deste pacote. O teste roda com o cwd em
	// pkg/bootstrap, daí o caminho relativo.
	raiz := filepath.Join("..", "..", "pkg")

	// Campos que NOMEIAM o token como valor logado. `token_present` e
	// `presented_len` (auth.ValidateAdminToken) são o oposto disto: descrevem o
	// token sem revelá-lo, e são exatamente o que se deve fazer.
	proibidos := []string{`Str("token"`, `Str("Token"`, `Str("api_token"`}

	visitados := 0
	err := filepath.Walk(raiz, func(caminho string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(caminho, ".go") || strings.HasSuffix(caminho, "_test.go") {
			return nil
		}

		conteudo, err := os.ReadFile(caminho)
		if err != nil {
			return err
		}
		visitados++

		for _, linha := range strings.Split(string(conteudo), "\n") {
			for _, padrao := range proibidos {
				if strings.Contains(linha, padrao) {
					t.Errorf("%s loga o token da API: %s\n"+
						"    Se precisar registrar algo sobre ele, registre um FATO sem o valor "+
						"(presenca, tamanho, prefixo de hash) — como auth.ValidateAdminToken ja faz.",
						caminho, strings.TrimSpace(linha))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("varrer o pacote: %v", err)
	}

	// Controle de que o CENÁRIO existiu, no espírito da ARMADILHAS 21: uma
	// varredura que não varreu nada passa silenciosamente e parece proteção.
	// O número é um piso folgado, não uma contagem exata — apertá-lo faria o
	// teste falhar a cada arquivo removido, por motivo nenhum.
	const pisoDeArquivos = 200
	if visitados < pisoDeArquivos {
		t.Fatalf("a varredura viu apenas %d arquivos (piso %d); o caminho %q provavelmente esta errado e este teste nao esta protegendo nada",
			visitados, pisoDeArquivos, raiz)
	}
}
