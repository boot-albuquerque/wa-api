package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"wa-api/pkg/presentation/http/apidocs"
)

// Testes de CONTRATO: a implementação real contra o que a especificação
// promete.
//
// POR QUE ESTES SÃO DIFERENTES DOS OUTROS GATES. Os gates anteriores olham
// para o documento — se está completo, se cobre as rotas, se cada campo tem
// semântica. Nenhum deles chama nada. Um documento pode passar em todos e
// descrever uma API que não existe.
//
// Estes chamam o router de verdade e comparam a resposta com o schema. O
// caminho que se exercita aqui é o de RECUSA — sem sessão, sem corpo, sem
// token —, porque é o único que se pode percorrer sem uma conta de WhatsApp
// ligada. É pouco, e é honesto dizê-lo: cobre o envelope, o status e a forma
// do erro, não a forma do sucesso.

// minimoDeRotasExercitadas é o piso abaixo do qual o teste se declara cego.
// O valor vem da medição: 138 operações respondem 401 sem token. Fica com
// folga para baixo, para não partir a cada rota nova, mas alto o suficiente
// para apanhar o caso em que o router deixa de casar.
const minimoDeRotasExercitadas = 120

func especificacao(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(apidocs.Specification(), &doc); err != nil {
		t.Fatalf("especificação não é YAML válido: %v", err)
	}
	return doc
}

// TestContratoRespostaDeRecusaBateComOEsquema percorre toda rota registada,
// chama-a SEM token, e afirma que:
//
//  1. o status devolvido está documentado para aquela operação;
//  2. o corpo é o envelope de erro que a especificação promete.
//
// Uma rota que responda 401 sem o documentar, ou que devolva um corpo com
// outra forma, é apanhada aqui.
func TestContratoRespostaDeRecusaBateComOEsquema(t *testing.T) {
	doc := especificacao(t)
	caminhos, _ := doc["paths"].(map[string]any)

	d := goldenTestDeps(t)
	router := NewRouter(d)

	var semDocumentar []string
	var envelopeErrado []string
	exercitadas := 0
	porEstado := map[int]int{}

	for _, rota := range Routes(d) {
		if strings.HasPrefix(rota.Path, apidocs.BasePath) || strings.Contains(rota.Path, "{rest:") {
			continue
		}
		item, ok := caminhos[rota.Path].(map[string]any)
		if !ok {
			continue // já apanhado por TestOpenAPICobreTodasAsRotasRegistadas
		}
		for _, metodo := range rota.Methods {
			op, ok := item[strings.ToLower(metodo)].(map[string]any)
			if !ok {
				continue
			}
			respostas, _ := op["responses"].(map[string]any)

			pedido := httptest.NewRequest(metodo, caminhoConcreto(rota.Path), strings.NewReader("{}"))
			pedido.Header.Set("Content-Type", "application/json")
			gravador := httptest.NewRecorder()
			router.ServeHTTP(gravador, pedido)

			chave := metodo + " " + rota.Path
			estado := gravador.Code

			// 405 e 404 vêm do próprio mux quando o caminho concreto não casa
			// com o padrão; não são resposta da operação.
			if estado == http.StatusNotFound || estado == http.StatusMethodNotAllowed {
				continue
			}
			exercitadas++
			porEstado[estado]++
			if _, documentado := respostas[itoa(estado)]; !documentado {
				semDocumentar = append(semDocumentar,
					chave+" devolveu "+itoa(estado)+", que não está nas responses")
			}
			if estado >= 400 {
				if motivo := envelopeDeErroValido(gravador.Body.Bytes()); motivo != "" {
					envelopeErrado = append(envelopeErrado, chave+" ["+itoa(estado)+"]: "+motivo)
				}
			}
		}
	}

	// Sem este piso, o teste passaria a verde no dia em que o router deixasse
	// de casar os caminhos — exercitando ZERO rotas e afirmando conformidade
	// sobre o vazio. É o modo de falha mais perigoso de um teste de contrato.
	if exercitadas < minimoDeRotasExercitadas {
		t.Fatalf("só %d operações foram exercitadas (mínimo %d): o teste não está a medir o que diz",
			exercitadas, minimoDeRotasExercitadas)
	}
	t.Logf("operações exercitadas: %d; estados observados: %v", exercitadas, porEstado)

	sort.Strings(semDocumentar)
	sort.Strings(envelopeErrado)
	if len(semDocumentar) > 0 {
		t.Errorf("%d respostas reais não documentadas:\n  %s",
			len(semDocumentar), strings.Join(semDocumentar, "\n  "))
	}
	if len(envelopeErrado) > 0 {
		t.Errorf("%d corpos de erro fora do envelope documentado:\n  %s",
			len(envelopeErrado), strings.Join(envelopeErrado, "\n  "))
	}
}

// envelopeDeErroValido devolve o motivo da divergência, ou "" se o corpo
// obedecer à ÚNICA forma que a API produz.
//
// Aceitava duas formas — a canónica, com `error` em objecto, e a antiga, com
// `error` em texto, que sobrevivia em catorze pontos (F266). A F266 está
// corrigida: `RespondJSON` devolve objecto em todo ramo, tipado ou não. A
// tolerância saiu junto, porque uma tolerância que sobrevive à causa deixa de
// registar o estado real e passa a esconder a regressão.
func envelopeDeErroValido(corpo []byte) string {
	var envelope map[string]any
	if err := json.Unmarshal(corpo, &envelope); err != nil {
		return "corpo não é JSON: " + err.Error()
	}
	if _, tem := envelope["code"]; !tem {
		return "sem campo code"
	}
	if sucesso, tem := envelope["success"]; !tem {
		return "sem campo success"
	} else if sucesso != false {
		return "success não é false num corpo de erro"
	}
	erro, tem := envelope["error"]
	if !tem {
		return "sem campo error"
	}
	switch valor := erro.(type) {
	case string:
		return "error é TEXTO: a forma antiga (F266) foi removida — " +
			"todo corpo de erro tem `error` em objecto {code, message}"
	case map[string]any:
		if _, tem := valor["code"]; !tem {
			return "error é objecto mas não tem code"
		}
		if _, tem := valor["message"]; !tem {
			return "error é objecto mas não tem message"
		}
		return ""
	default:
		return "error não é nem texto nem objecto"
	}
}

// caminhoConcreto substitui parâmetros de caminho por um valor que casa com o
// padrão, para que o pedido chegue ao manipulador em vez de morrer no router.
func caminhoConcreto(padrao string) string {
	caminho := padrao
	caminho = strings.ReplaceAll(caminho, "{id}", "00000000000000000000000000000000")
	caminho = strings.ReplaceAll(caminho, "{jid}", "554192421234@s.whatsapp.net")
	return caminho
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digitos []byte
	for n > 0 {
		digitos = append([]byte{byte('0' + n%10)}, digitos...)
		n /= 10
	}
	return string(digitos)
}

// TestContratoExemplosSaoValidosContraOTipo apanha o exemplo que contradiz o
// seu próprio esquema — um `example: "abc"` num `type: integer`, um exemplo com
// campo que o esquema não declara.
//
// É o erro que passa despercebido porque o Swagger desenha na mesma: quem
// copia o exemplo leva um 400 e não sabe porquê.
func TestContratoExemplosSaoValidosContraOTipo(t *testing.T) {
	var problemas []string
	for _, p := range propriedadesDosEsquemas(t) {
		exemplo, tem := p.corpo["example"]
		if !tem {
			continue
		}
		tipo := texto(p.corpo["type"])
		if motivo := tipoBate(tipo, exemplo); motivo != "" {
			problemas = append(problemas, p.caminho+": "+motivo)
		}
		if lista, ok := p.corpo["enum"].([]any); ok && tem {
			pertence := false
			for _, valor := range lista {
				if valor == exemplo {
					pertence = true
					break
				}
			}
			if !pertence {
				problemas = append(problemas, p.caminho+": example não pertence ao enum")
			}
		}
	}
	sort.Strings(problemas)
	if len(problemas) > 0 {
		t.Errorf("%d exemplos que contradizem o próprio esquema:\n  %s",
			len(problemas), strings.Join(problemas, "\n  "))
	}
}

// TestContratoCodigosDeErroExistemNoCodigo apanha a obsolescência que nenhum
// outro gate vê: um `error.code` documentado que o código já não produz.
//
// POR QUE É PRECISO. Corrigir uma validação renomeia códigos — `invalid_x` que
// se parte em `missing_x` e `invalid_x`, por exemplo. A especificação continua
// a citar o nome antigo em exemplos e em prosa, e nada estoira: o YAML é
// válido, a rota existe, o exemplo é do tipo certo. Só um cliente é que
// descobre, ramificando sobre um código que nunca chega.
//
// O que se afirma: todo código citado num EXEMPLO de resposta existe como
// literal no código Go. Não prova que a rota o devolve — prova que ele não foi
// inventado nem sobreviveu a uma renomeação.
func TestContratoCodigosDeErroExistemNoCodigo(t *testing.T) {
	fonte := lerFonteGo(t)

	citados := map[string][]string{}
	var recolhe func(caminho string, no any)
	recolhe = func(caminho string, no any) {
		switch valor := no.(type) {
		case map[string]any:
			if erro, ok := valor["error"].(map[string]any); ok {
				if codigo, ok := erro["code"].(string); ok {
					citados[codigo] = append(citados[codigo], caminho)
				}
			}
			for chave, filho := range valor {
				recolhe(caminho+"."+chave, filho)
			}
		case []any:
			for _, filho := range valor {
				recolhe(caminho, filho)
			}
		}
	}
	doc := especificacao(t)
	recolhe("", doc["paths"])
	recolhe("", doc["components"])

	if len(citados) < minimoDeCodigosCitados {
		t.Fatalf("só %d códigos de erro citados em exemplos (mínimo %d): "+
			"o teste não está a medir o que diz", len(citados), minimoDeCodigosCitados)
	}

	var inventados []string
	for codigo, onde := range citados {
		if !strings.Contains(fonte, `"`+codigo+`"`) {
			inventados = append(inventados, codigo+" (em "+onde[0]+")")
		}
	}
	sort.Strings(inventados)
	if len(inventados) > 0 {
		t.Errorf("%d códigos de erro documentados que NÃO existem no código Go — "+
			"foram renomeados, removidos, ou nunca existiram:\n  %s",
			len(inventados), strings.Join(inventados, "\n  "))
	}
	t.Logf("códigos de erro citados em exemplos: %d, todos presentes no código", len(citados))
}

// TestContrato422Implica429 é o gate que faltava na F293: um `429` é a
// resposta que a MESMA função (`errmap.classifyIQCode`) produz quando o
// WhatsApp estrangula em vez de recusar — as duas categorias vêm do mesmo
// call site, `errmap.ClassifyIQ`, então uma operação que documenta uma
// documenta necessariamente a outra. `422` (RecusadoPeloWhatsApp) é o marcador
// escolhido porque é omnipresente nas 31 operações que atravessam esse
// caminho; sem este teste, apagar o `429` de uma delas — ou esquecê-lo numa
// rota nova que ganhe `422` — passaria por todos os gates existentes, porque
// nenhum deles olha para a RELAÇÃO entre os dois códigos.
func TestContrato422Implica429(t *testing.T) {
	doc := especificacao(t)
	caminhos, _ := doc["paths"].(map[string]any)

	var faltando []string
	com422 := 0
	for caminho, item := range caminhos {
		metodos, _ := item.(map[string]any)
		for metodo, corpo := range metodos {
			op, ok := corpo.(map[string]any)
			if !ok {
				continue
			}
			respostas, _ := op["responses"].(map[string]any)
			if respostas == nil {
				continue
			}
			resp422, tem422 := respostas["422"]
			if !tem422 {
				continue
			}
			// EngineNaoSuporta (2026-08-28, registry de pareamento por
			// engine, F273) é um 422 que NUNCA passa por
			// errmap.ClassifyIQ — capability_not_supported é uma decisão
			// local do pkg/pairing, resolvida ANTES de qualquer chamada ao
			// WhatsApp. Documentar 429 ao lado seria afirmar um caminho de
			// estrangulamento que não existe para essas três rotas.
			// Excluído por $ref, não por caminho: se um dia estas rotas
			// ganharem TAMBÉM um 422 de errmap.ClassifyIQ (ex.: oneOf), a
			// checagem por $ref deixa de casar e a regra volta a valer.
			if m, ok := resp422.(map[string]any); ok && m["$ref"] == "#/components/responses/EngineNaoSuporta" {
				continue
			}
			com422++
			if _, tem429 := respostas["429"]; !tem429 {
				faltando = append(faltando, strings.ToUpper(metodo)+" "+caminho)
			}
		}
	}
	sort.Strings(faltando)

	if com422 == 0 {
		t.Fatal("nenhuma operação documenta 422: o teste não está a medir o que diz")
	}
	if len(faltando) > 0 {
		t.Errorf("%d operação(ões) documentam 422 (RecusadoPeloWhatsApp) sem "+
			"documentar 429 (LimiteDeRitmo) — as duas vêm do mesmo "+
			"errmap.ClassifyIQ, e uma sem a outra é o defeito que a F293 mediu:\n  %s",
			len(faltando), strings.Join(faltando, "\n  "))
	}
}

// TestContratoExemploDeErroBateComOEsquema apanha o par que diverge sem
// estoirar: o exemplo mostra `error` como objecto e o esquema diz que é texto,
// ou o contrário.
//
// POR QUE ACONTECE. Esta API tem DUAS formas de erro — a canónica, com
// `{code, message}`, e a antiga, com `error` em texto (F266). Escolher a
// errada não parte nada: o YAML é válido, o Swagger desenha o exemplo, e o
// gerador de clientes produz um tipo que nunca casa com o que chega.
//
// Medido a 2026-08-26: havia 23 divergências, e em todas era o ESQUEMA que
// estava errado — os exemplos tinham sido copiados de respostas reais.
func TestContratoExemploDeErroBateComOEsquema(t *testing.T) {
	doc := especificacao(t)
	caminhos, _ := doc["paths"].(map[string]any)

	var divergem []string
	verificados := 0
	for caminho, item := range caminhos {
		metodos, _ := item.(map[string]any)
		for metodo, corpo := range metodos {
			op, ok := corpo.(map[string]any)
			if !ok {
				continue
			}
			respostas, _ := op["responses"].(map[string]any)
			for codigo, bruto := range respostas {
				resposta, ok := bruto.(map[string]any)
				if !ok {
					continue
				}
				if _, ehRef := resposta["$ref"]; ehRef {
					continue // componente partilhado, verificado por si próprio
				}
				conteudo, _ := resposta["content"].(map[string]any)
				json, _ := conteudo["application/json"].(map[string]any)
				exemplo, _ := json["example"].(map[string]any)
				if exemplo == nil {
					continue
				}
				erro, tem := exemplo["error"]
				if !tem {
					continue
				}
				verificados++
				chave := strings.ToUpper(metodo) + " " + caminho + " [" + codigo + "]"
				// Já não há duas formas para conciliar: o esquema de erro é um
				// só, e o exemplo tem de ser objecto com code e message.
				objecto, ok := erro.(map[string]any)
				if !ok {
					divergem = append(divergem, chave+": exemplo com `error` em TEXTO (forma F266, removida)")
					continue
				}
				if _, tem := objecto["code"]; !tem {
					divergem = append(divergem, chave+": exemplo com `error` em objecto sem `code`")
				}
				if _, tem := objecto["message"]; !tem {
					divergem = append(divergem, chave+": exemplo com `error` em objecto sem `message`")
				}
			}
		}
	}
	if verificados < minimoDeExemplosDeErro {
		t.Fatalf("só %d exemplos de erro verificados (mínimo %d)", verificados, minimoDeExemplosDeErro)
	}
	sort.Strings(divergem)
	if len(divergem) > 0 {
		t.Errorf("%d exemplos de erro que contradizem o próprio esquema:\n  %s",
			len(divergem), strings.Join(divergem, "\n  "))
	}
	t.Logf("exemplos de erro verificados: %d, todos coerentes com o esquema", verificados)
}

// renderYAML serializa um nó para inspecção textual.
func renderYAML(no any) string {
	bruto, err := yaml.Marshal(no)
	if err != nil {
		return ""
	}
	return string(bruto)
}

// minimoDeExemplosDeErro é o piso que impede o teste de se declarar conforme
// sem ter olhado para nada.
const minimoDeExemplosDeErro = 50

// minimoDeCodigosCitados impede o teste de passar por não ter olhado para nada.
const minimoDeCodigosCitados = 20

// lerFonteGo concatena o código Go do repositório, sem testes.
//
// Sem testes de propósito: um código que só apareça num ficheiro _test.go é um
// código que a produção não emite, e documentá-lo seria descrever a suite em
// vez da API.
func lerFonteGo(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	raizes := []string{"../../pkg", "../../internal/wa-noise", "../../cmd"}
	for _, raiz := range raizes {
		err := filepath.Walk(raiz, func(caminho string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(caminho, ".go") || strings.HasSuffix(caminho, "_test.go") {
				return nil
			}
			conteudo, err := os.ReadFile(caminho)
			if err != nil {
				return nil
			}
			sb.Write(conteudo)
			return nil
		})
		if err != nil {
			t.Fatalf("percorrer %s: %v", raiz, err)
		}
	}
	if sb.Len() == 0 {
		t.Fatal("nenhum código Go lido: o gate estaria a comparar contra o vazio")
	}
	return sb.String()
}

func tipoBate(tipo string, valor any) string {
	switch tipo {
	case "string":
		if _, ok := valor.(string); !ok {
			return "type: string mas o example não é texto"
		}
	case "integer":
		switch valor.(type) {
		case int, int64, float64:
		default:
			return "type: integer mas o example não é número"
		}
	case "number":
		switch valor.(type) {
		case int, int64, float64:
		default:
			return "type: number mas o example não é número"
		}
	case "boolean":
		if _, ok := valor.(bool); !ok {
			return "type: boolean mas o example não é booleano"
		}
	case "array":
		if _, ok := valor.([]any); !ok {
			return "type: array mas o example não é lista"
		}
	case "object":
		if _, ok := valor.(map[string]any); !ok {
			return "type: object mas o example não é objecto"
		}
	}
	return ""
}
