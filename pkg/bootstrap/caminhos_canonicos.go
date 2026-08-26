package bootstrap

import (
	_ "embed"
	"fmt"
	"strings"

	customhttp "wa-api/pkg/presentation/http"
)

// caminhosTSV é a tabela de padronização, embutida para que o binário não
// dependa de um ficheiro no disco em tempo de execução — pelo mesmo motivo da
// especificação OpenAPI.
//
//go:embed caminhos.tsv
var caminhosTSV string

// colunasDaTabela é a forma de uma linha: método e caminho antigos, método e
// caminho canónicos.
const colunasDaTabela = 4

// CaminhosCanonicos analisa a tabela embutida.
//
// Falha ALTO em vez de devolver uma lista parcial: uma tabela meio lida
// registaria metade dos aliases, e as rotas em falta só apareceriam quando um
// cliente as chamasse.
func CaminhosCanonicos() []customhttp.CanonicalRoute {
	rotas, err := analisarCaminhos(caminhosTSV)
	if err != nil {
		panic("bootstrap: tabela de caminhos canónicos inválida: " + err.Error())
	}
	return rotas
}

func analisarCaminhos(conteudo string) ([]customhttp.CanonicalRoute, error) {
	var out []customhttp.CanonicalRoute
	for numero, linha := range strings.Split(conteudo, "\n") {
		linha = strings.TrimSpace(linha)
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		campos := strings.Split(linha, "\t")
		if len(campos) != colunasDaTabela {
			return nil, fmt.Errorf("linha %d: esperava %d colunas separadas por tabulação, veio %d",
				numero+1, colunasDaTabela, len(campos))
		}
		out = append(out, customhttp.CanonicalRoute{
			LegacyMethod:    strings.ToUpper(campos[0]),
			LegacyPath:      campos[1],
			CanonicalMethod: strings.ToUpper(campos[2]),
			CanonicalPath:   campos[3],
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tabela vazia")
	}
	return out, nil
}
