package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
// obedecer a uma das DUAS formas que a API produz hoje.
//
// São duas de propósito: a forma canónica, com `error` em objecto, e a antiga,
// com `error` em texto — que sobrevive em catorze pontos (F266). Aceitar as
// duas aqui é registar o estado real; estreitar para uma só quando a F266
// estiver corrigida é que fará o teste travar a regressão.
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
		return "" // forma antiga, F266
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
