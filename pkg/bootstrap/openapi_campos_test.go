package bootstrap

import (
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"wa-api/pkg/presentation/http/apidocs"
)

// Este ficheiro é o gate da SEMÂNTICA DE CAMPO.
//
// O gate de cobertura (openapi_coverage_test.go) garante que toda rota está
// documentada. Isso não impede o caso que motivou esta auditoria: uma rota
// presente, com título e descrição, e um campo dizendo apenas `type: string` —
// sobre o qual o consumidor continua sem saber se é obrigatório, se aceita
// `null`, se aceita `""`, e o que acontece com um valor desconhecido.
//
// A regra que estes testes impõem: **nenhuma propriedade sem semântica de
// presença**. Escrever `type: string` e mais nada deixa de compilar.

// propriedade é uma folha do documento, com o caminho por onde se lá chega.
type propriedade struct {
	caminho string
	corpo   map[string]any
}

// propriedadesDosEsquemas percorre components.schemas e devolve toda
// propriedade inline, descendo por allOf, oneOf, anyOf e items.
//
// Desce de propósito: a maior parte dos esquemas de pedido desta API são
// composições `allOf` sobre `AlvoConversa`, e um percurso que só olhasse para
// `properties` do nível de topo veria zero campos e daria tudo por conforme.
func propriedadesDosEsquemas(t *testing.T) []propriedade {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(apidocs.Specification(), &doc); err != nil {
		t.Fatalf("especificação não é YAML válido: %v", err)
	}
	componentes, _ := doc["components"].(map[string]any)
	esquemas, _ := componentes["schemas"].(map[string]any)
	if len(esquemas) == 0 {
		t.Fatal("nenhum esquema: o gate estaria a afirmar conformidade sobre o vazio")
	}

	var out []propriedade
	var desce func(prefixo string, no any, prof int)
	desce = func(prefixo string, no any, prof int) {
		mapa, ok := no.(map[string]any)
		if !ok || prof > profundidadeMaxima {
			return
		}
		if props, ok := mapa["properties"].(map[string]any); ok {
			for nome, corpo := range props {
				filho, ok := corpo.(map[string]any)
				if !ok {
					continue
				}
				// Uma propriedade que é só `$ref` herda a semântica do esquema
				// referenciado — que é analisado por si próprio.
				if _, apenasRef := filho["$ref"]; !apenasRef || len(filho) > 1 {
					out = append(out, propriedade{caminho: prefixo + "." + nome, corpo: filho})
				}
				desce(prefixo+"."+nome, filho, prof+1)
			}
		}
		for _, chave := range []string{"allOf", "oneOf", "anyOf"} {
			if lista, ok := mapa[chave].([]any); ok {
				for _, sub := range lista {
					desce(prefixo, sub, prof+1)
				}
			}
		}
		if itens, ok := mapa["items"]; ok {
			desce(prefixo+"[]", itens, prof+1)
		}
	}
	for nome, esquema := range esquemas {
		desce(nome, esquema, 0)
	}
	return out
}

// profundidadeMaxima trava a recursão. Seis níveis cobrem o esquema mais
// aninhado desta API (carrossel: cartões -> botões -> propriedades) com folga.
const profundidadeMaxima = 8

// TestTodaPropriedadeTemDescricao: um campo sem descrição obriga o consumidor
// a adivinhar ou a ler o código — que é exactamente o que esta auditoria
// existe para eliminar.
func TestTodaPropriedadeTemDescricao(t *testing.T) {
	var sem []string
	for _, p := range propriedadesDosEsquemas(t) {
		if _, ref := p.corpo["$ref"]; ref {
			continue
		}
		if strings.TrimSpace(texto(p.corpo["description"])) == "" {
			sem = append(sem, p.caminho)
		}
	}
	sort.Strings(sem)
	if len(sem) > 0 {
		t.Errorf("%d propriedades sem description:\n  %s", len(sem), strings.Join(sem, "\n  "))
	}
}

// TestTodaPropriedadeDeclaraNulabilidade é o coração desta auditoria.
//
// `type: string` não diz se o campo aceita `null`. Nesta API a resposta quase
// nunca é óbvia: para um tipo não-ponteiro em Go, `null` é INDISTINGUÍVEL de
// ausente (medido em 2026-08-26), e num booleano isso significa `false`
// silencioso. Deixar implícito é deixar o consumidor descobrir por acidente.
func TestTodaPropriedadeDeclaraNulabilidade(t *testing.T) {
	var sem []string
	for _, p := range propriedadesDosEsquemas(t) {
		if _, ref := p.corpo["$ref"]; ref {
			continue
		}
		if _, declarado := p.corpo["nullable"]; declarado {
			continue
		}
		// A descrição pode carregar a semântica em prosa ou em tabela; nesse
		// caso ela é a fonte, e é aceite. O que não se aceita é o silêncio.
		if descreveNulabilidade(texto(p.corpo["description"])) {
			continue
		}
		sem = append(sem, p.caminho)
	}
	sort.Strings(sem)
	if len(sem) > 0 {
		t.Errorf("%d propriedades sem semântica de nulabilidade — declare `nullable:` "+
			"ou diga na description o que acontece com `null`:\n  %s",
			len(sem), strings.Join(sem, "\n  "))
	}
}

// descreveNulabilidade procura, na descrição, sinal de que a questão foi
// respondida. Não julga a resposta — julga que alguém a deu.
func descreveNulabilidade(descricao string) bool {
	baixa := strings.ToLower(descricao)
	for _, marca := range []string{"null", "nulo", "ausente", "omitid"} {
		if strings.Contains(baixa, marca) {
			return true
		}
	}
	return false
}

// TestTodoEnumDizOQueAconteceComValorDesconhecido: um enum sem essa frase
// obriga o consumidor a experimentar em produção para saber se um valor novo
// é recusado, ignorado ou aceite.
func TestTodoEnumDizOQueAconteceComValorDesconhecido(t *testing.T) {
	var sem []string
	for _, p := range propriedadesDosEsquemas(t) {
		if _, tem := p.corpo["enum"]; !tem {
			continue
		}
		descricao := strings.ToLower(texto(p.corpo["description"]))
		respondido := false
		for _, marca := range []string{"desconhecid", "fora do", "outro valor", "qualquer outro", "não reconhecid", "inválid"} {
			if strings.Contains(descricao, marca) {
				respondido = true
				break
			}
		}
		if !respondido {
			sem = append(sem, p.caminho)
		}
	}
	sort.Strings(sem)
	if len(sem) > 0 {
		t.Errorf("%d enums que não dizem o que acontece com valor fora da lista:\n  %s",
			len(sem), strings.Join(sem, "\n  "))
	}
}

func texto(v any) string { s, _ := v.(string); return s }
