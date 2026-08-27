package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// Este ficheiro implementava a padronização de caminhos (F269) SEM partir
// clientes: cada rota antiga continuava registada, e ao lado dela passava a
// existir a forma canónica, para sempre.
//
// REVERSÃO (2026-08-27, ver HOUSEKEEP.md): decisão explícita do utilizador —
// o projeto não tem consumidores reais antes do lançamento, então não há
// cliente a proteger, e manter as duas formas registadas era pagar o custo de
// compatibilidade sem ter quem a use. CanonicalizeRoutes agora RENOMEIA a
// rota em vez de lhe acrescentar um alias: o caminho antigo deixa de responder
// (404), só o canónico fica registado. A tabela e o mecanismo de
// correspondência sobrevivem porque continuam a ser a única forma de aplicar
// a mudança a noventa e uma rotas sem editar cada `Register` à mão.
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

// CanonicalizeRoutes substitui, para cada entrada registada que tenha forma
// canónica na tabela, o registo antigo pelo canónico — mesmo manipulador,
// caminho novo. A rota antiga deixa de estar no registry e, portanto, deixa
// de responder.
//
// A ORDEM IMPORTA: chame isto DEPOIS de todas as rotas antigas estarem
// registadas. Uma entrada da tabela sem rota antiga correspondente é erro do
// chamador — e é devolvida, não engolida, porque uma tabela que aponta para
// rotas inexistentes é uma tabela que já não descreve o serviço.
func (r *HandlerRegistry) CanonicalizeRoutes(tabela []CanonicalRoute) []string {
	porChave := map[string]routeEntry{}
	for _, entrada := range r.routes {
		for _, metodo := range entrada.methods {
			porChave[strings.ToUpper(metodo)+" "+entrada.path] = entrada
		}
	}

	consumidas := map[string]bool{}
	var canonicas []routeEntry
	var orfas []string
	for _, linha := range tabela {
		chave := strings.ToUpper(linha.LegacyMethod) + " " + linha.LegacyPath
		entrada, existe := porChave[chave]
		if !existe {
			orfas = append(orfas, chave)
			continue
		}
		consumidas[chave] = true
		manipulador := entrada.handler
		// Quando o caminho canónico traz parâmetros, o identificador deixa de
		// vir no corpo. O manipulador continua a lê-lo do corpo — logo o
		// adaptador injecta-o antes de lhe passar o pedido.
		if strings.Contains(linha.CanonicalPath, "{") {
			manipulador = injectPathParams(manipulador)
		}
		canonicas = append(canonicas, routeEntry{
			path:    linha.CanonicalPath,
			handler: manipulador,
			methods: []string{linha.CanonicalMethod},
		})
	}

	// Remove do registry os métodos que a tabela consumiu, um a um — uma
	// entrada pode combinar vários métodos no mesmo Register (ex.:
	// "/user/privacy" com GET e POST), e só alguns podem ter linha na
	// tabela. Uma entrada com TODOS os métodos consumidos desaparece;
	// com só ALGUNS, fica registada com os que sobraram.
	var restantes []routeEntry
	for _, entrada := range r.routes {
		var mantidos []string
		for _, metodo := range entrada.methods {
			chave := strings.ToUpper(metodo) + " " + entrada.path
			if !consumidas[chave] {
				mantidos = append(mantidos, metodo)
			}
		}
		if len(mantidos) == 0 {
			continue
		}
		if len(mantidos) != len(entrada.methods) {
			entrada.methods = mantidos
		}
		restantes = append(restantes, entrada)
	}
	r.routes = append(restantes, canonicas...)
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
