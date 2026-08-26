package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// Este ficheiro implementa a padronização de caminhos (F269) SEM partir
// clientes: cada rota antiga continua registada, e ao lado dela passa a existir
// a forma canónica.
//
// POR QUE A TABELA E NÃO 91 EDIÇÕES À MÃO. São noventa e uma rotas. Editar cada
// chamada a Register seria noventa e uma oportunidades de trocar um caractere,
// e o erro só apareceria quando alguém chamasse a rota errada. Com a tabela, a
// transformação é uma só, e o gate compara o que foi registado com o que a
// tabela manda.

// CanonicalRoute é uma linha da tabela: a rota antiga e a forma canónica dela.
type CanonicalRoute struct {
	LegacyMethod    string
	LegacyPath      string
	CanonicalMethod string
	CanonicalPath   string
}

// RegisterCanonicalAliases acrescenta, para cada entrada registada que tenha
// forma canónica na tabela, uma segunda rota com essa forma.
//
// A ORDEM IMPORTA: chame isto DEPOIS de todas as rotas antigas estarem
// registadas. Uma entrada da tabela sem rota antiga correspondente é erro do
// chamador — e é devolvida, não engolida, porque uma tabela que aponta para
// rotas inexistentes é uma tabela que já não descreve o serviço.
func (r *HandlerRegistry) RegisterCanonicalAliases(tabela []CanonicalRoute) []string {
	porChave := map[string]routeEntry{}
	for _, entrada := range r.routes {
		for _, metodo := range entrada.methods {
			porChave[strings.ToUpper(metodo)+" "+entrada.path] = entrada
		}
	}

	var orfas []string
	for _, linha := range tabela {
		chave := strings.ToUpper(linha.LegacyMethod) + " " + linha.LegacyPath
		entrada, existe := porChave[chave]
		if !existe {
			orfas = append(orfas, chave)
			continue
		}
		manipulador := entrada.handler
		// Quando o caminho canónico traz parâmetros, o identificador deixa de
		// vir no corpo. O manipulador continua a lê-lo do corpo — logo o
		// adaptador injecta-o antes de lhe passar o pedido.
		if strings.Contains(linha.CanonicalPath, "{") {
			manipulador = injectPathParams(manipulador)
		}
		r.Register(linha.CanonicalPath, manipulador, linha.CanonicalMethod)
	}
	return orfas
}

// bodyFieldForPathParam diz em que campo do corpo cada parâmetro de caminho
// deve ser injectado.
//
// Os nomes de destino são os que os manipuladores JÁ leem — não uma
// normalização nova. Mudar o nome do campo aqui mudaria o contrato do corpo, e
// o objectivo desta camada é exactamente o contrário: caminho novo, corpo
// igual.
var bodyFieldForPathParam = map[string]string{
	"group_jid":     "groupJID",
	"community_jid": "communityJID",
}

// injectPathParams copia os parâmetros do caminho para o corpo JSON, e só
// então chama o manipulador original.
//
// NÃO SOBRESCREVE. Se o corpo já trouxer o campo, o corpo ganha — o caminho é
// a forma nova de dizer a mesma coisa, e não uma autoridade sobre quem já a
// dizia. Isso mantém a rota canónica utilizável por um cliente que ainda
// envie o identificador no corpo, durante a migração.
func injectPathParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if len(vars) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		bruto, err := io.ReadAll(r.Body)
		if err != nil {
			// Ler o corpo falhou: deixa o manipulador original responder pelo
			// erro, com a mensagem que ele já dá. Inventar uma aqui seria uma
			// segunda forma de dizer a mesma falha.
			next.ServeHTTP(w, r)
			return
		}
		_ = r.Body.Close()

		corpo := map[string]any{}
		if len(bytes.TrimSpace(bruto)) > 0 {
			if err := json.Unmarshal(bruto, &corpo); err != nil {
				// Corpo ilegível: repõe-no tal e qual, para que a recusa venha
				// do decodificador do manipulador e traga o código habitual.
				r.Body = io.NopCloser(bytes.NewReader(bruto))
				next.ServeHTTP(w, r)
				return
			}
		}

		for parametro, valor := range vars {
			campo, conhecido := bodyFieldForPathParam[parametro]
			if !conhecido {
				continue
			}
			if _, jaVeio := corpo[campo]; jaVeio {
				continue
			}
			corpo[campo] = valor
		}

		novo, err := json.Marshal(corpo)
		if err != nil {
			r.Body = io.NopCloser(bytes.NewReader(bruto))
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(novo))
		r.ContentLength = int64(len(novo))
		next.ServeHTTP(w, r)
	})
}
