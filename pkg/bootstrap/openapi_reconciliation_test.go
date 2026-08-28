package bootstrap

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Este ficheiro RECONCILIA as fontes que contam evidência e que
// `cmd/openapidoc` NÃO gera a partir de `evidencias.tsv`.
//
// POR QUE ELE EXISTE. Os gates que já cá estavam verificam que cada operação
// TEM marca (TestOpenAPISummariesTrazemMarcaDeEvidencia) e que o documento
// embutido está em dia com as fontes (TestOpenAPIGeradoEstaAtualizado). Nenhum
// verificava que os NÚMEROS batem entre si — e foi por aí que a divergência
// passou: em 2d96535, com as formas antigas ainda no contrato, o
// `evidencias.tsv` e o relatório diziam 232 operações (178/13/6/35) enquanto a
// legenda de `info.description`, servida a toda a gente em /docs, continuava a
// dizer "As 141 estão documentadas" e "98 ✅, 8 🟡, 3 ❌, 32 ⬜". Toda a suíte
// passava.
//
// F239/F282 (2026-08-28) removeram DUAS das quatro fontes que este ficheiro
// reconciliava: `docs/OPENAPI-EVIDENCIAS.md` passou a ser GERADO por
// `cmd/openapidoc` a partir da própria `evidencias.tsv`
// (evidence_report.go), então "a tabela por grupo e o total do relatório
// batem com a fonte única" deixou de ser uma pergunta que um teste precisa
// de fazer — é verdade por construção, e `TestOpenAPIGeradoEstaAtualizado`
// já recusa um relatório desactualizado (o mesmo `-check` que já cobria
// `openapi.yaml`). Os `TestEvidenceReportMatchesSpec` e
// `TestEvidenceReportSummaryMatchesTable` que viviam aqui foram removidos —
// não porque a garantia deixou de importar, mas porque ela passou a viver
// num sítio mais forte (a geração em si, não uma reconciliação a posteriori).
//
// O que sobra é a fonte que CONTINUA hand-maintained e que `cmd/openapidoc`
// não toca por desenho: a legenda em `info.description` (api/openapi/base.yaml).
//
// As duas fontes que restam:
//   1. api/openapi/evidencias.tsv          — a fonte única declarada
//   2. a especificação EMBUTIDA            — marca no summary de cada operação
//   3. info.description da especificação   — a legenda com os totais, à mão

const (
	evidenceTableFile = "evidencias.tsv"

	// evidenceTableColumns é a forma de uma linha desde F239/F282: método,
	// caminho, marca, data, observador, evidência. Só os três primeiros
	// interessam a este ficheiro (a marca é o que a legenda soma); os
	// últimos três existem para a campanha de medição, não para a
	// reconciliação de contagens.
	evidenceTableColumns = 6
)

// As quatro marcas, nomeadas. Um símbolo solto repetido em cinco sítios é o
// mesmo defeito à espera de divergir (ADR-0004).
const (
	markConfirmed  = "✅"
	markUnobserved = "🟡"
	markFailed     = "❌"
	markUntested   = "⬜"
)

// orderedMarks fixa a ordem em que os totais aparecem na legenda: ✅, 🟡, ❌, ⬜.
var orderedMarks = []string{markConfirmed, markUnobserved, markFailed, markUntested}

// TestEvidenceMarkConstantsMatchGenerator amarra as constantes acima à lista
// que o gate antigo já usava. Sem isto, as duas listas podem divergir em
// silêncio e cada gate passaria a medir um alfabeto diferente.
func TestEvidenceMarkConstantsMatchGenerator(t *testing.T) {
	if len(orderedMarks) != len(marcasDeEvidencia) {
		t.Fatalf("orderedMarks tem %d marcas, marcasDeEvidencia tem %d",
			len(orderedMarks), len(marcasDeEvidencia))
	}
	known := map[string]bool{}
	for _, mark := range marcasDeEvidencia {
		known[mark] = true
	}
	for _, mark := range orderedMarks {
		if !known[mark] {
			t.Errorf("marca %q não está em marcasDeEvidencia", mark)
		}
	}
}

// markCounts é a contagem por marca, a unidade de comparação deste ficheiro.
type markCounts map[string]int

func (c markCounts) total() int {
	sum := 0
	for _, n := range c {
		sum += n
	}
	return sum
}

// format rende a contagem sempre na mesma ordem, para que a mensagem de falha
// de duas fontes seja comparável linha a linha.
func (c markCounts) format() string {
	parts := make([]string, 0, len(orderedMarks))
	for _, mark := range orderedMarks {
		parts = append(parts, mark+strconv.Itoa(c[mark]))
	}
	return strings.Join(parts, " ") + " (total " + strconv.Itoa(c.total()) + ")"
}

// marksFromTable lê a fonte única e devolve marca por rota.
//
// TrimRight(line, "\r"), não TrimSpace: desde F239/F282 cada linha tem TRÊS
// colunas finais opcionais (data, observador, evidência), vazias na maioria
// das rotas ainda por remedir. TrimSpace trata tabulação como espaço e comeria
// esses campos vazios do fim da linha — "GET\t/x\t✅\t\t\t" viraria
// "GET\t/x\t✅" e a leitura pareceria ter 3 colunas em vez de 6, certo por
// acidente numa linha e errado (por excesso de colunas noutras) — é o mesmo
// defeito que mordeu a primeira versão do gerador (ver evidence_report.go).
func marksFromTable(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(openapiSpecRoot, evidenceTableFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("tabela de evidência: %v", err)
	}
	marks := map[string]string{}
	for lineNo, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != evidenceTableColumns {
			t.Fatalf("%s:%d: esperava %d colunas separadas por tabulação, veio %d",
				evidenceTableFile, lineNo+1, evidenceTableColumns, len(fields))
		}
		marks[strings.ToUpper(fields[0])+" "+fields[1]] = fields[2]
	}
	return marks
}

// markOfSummary devolve a marca com que um summary abre, ou "".
func markOfSummary(summary string) string {
	for _, mark := range marcasDeEvidencia {
		if strings.HasPrefix(summary, mark) {
			return mark
		}
	}
	return ""
}

// marksFromSpec devolve a marca por operação, lida do documento EMBUTIDO.
//
// Lê o YAML em vez de procurar o símbolo no texto: o emoji 🟡 é U+1F7E1, fora
// do plano básico, e o serializador escapa-o para "\U0001F7E1" enquanto deixa
// ✅, ❌ e ⬜ literais. Um gate que fizesse grep ao ficheiro contaria zero 🟡 e
// diria que oito operações não têm marca.
func marksFromSpec(t *testing.T) map[string]string {
	t.Helper()
	marks := map[string]string{}
	for key, op := range operacoesDaEspecificacao(t, carregarEspecificacao(t)) {
		marks[key] = markOfSummary(op.resumo)
	}
	return marks
}

func countMarks(marks map[string]string) markCounts {
	counts := markCounts{}
	for _, mark := range marks {
		counts[mark]++
	}
	return counts
}

// TestEvidenceTableMatchesSpec confronta a fonte única com o que a
// especificação embutida realmente publica — rota a rota, e não só no total.
//
// Comparar apenas os totais deixaria passar o caso em que uma rota subiu de 🟡
// a ✅ e outra desceu de ✅ a 🟡: os totais fechariam com o defeito lá dentro.
func TestEvidenceTableMatchesSpec(t *testing.T) {
	fromTable := marksFromTable(t)
	fromSpec := marksFromSpec(t)

	var divergent, onlyInTable, onlyInSpec []string
	for key, tableMark := range fromTable {
		specMark, present := fromSpec[key]
		switch {
		case !present:
			onlyInTable = append(onlyInTable, key)
		case specMark != tableMark:
			divergent = append(divergent, key+": tabela "+tableMark+", especificação "+specMark)
		}
	}
	for key := range fromSpec {
		if _, present := fromTable[key]; !present {
			onlyInSpec = append(onlyInSpec, key)
		}
	}
	sort.Strings(divergent)
	sort.Strings(onlyInTable)
	sort.Strings(onlyInSpec)

	for _, line := range divergent {
		t.Errorf("marca divergente — %s", line)
	}
	for _, key := range onlyInTable {
		t.Errorf("%s está em %s mas não na especificação", key, evidenceTableFile)
	}
	for _, key := range onlyInSpec {
		t.Errorf("%s está na especificação mas não em %s", key, evidenceTableFile)
	}

	if tableCounts, specCounts := countMarks(fromTable), countMarks(fromSpec); tableCounts.total() != specCounts.total() {
		t.Errorf("total divergente — %s: %s / especificação: %s",
			evidenceTableFile, tableCounts.format(), specCounts.format())
	}
}

// legendTotals é a regex do parágrafo de totais da legenda, em
// info.description: "**98 ✅, 8 🟡, 3 ❌, 32 ⬜**".
var legendTotals = regexp.MustCompile(
	`\*\*(\d+)\s*` + markConfirmed +
		`,\s*(\d+)\s*` + markUnobserved +
		`,\s*(\d+)\s*` + markFailed +
		`,\s*(\d+)\s*` + markUntested + `\*\*`)

// legendDocumented apanha "As 141 estão documentadas".
var legendDocumented = regexp.MustCompile(`As (\d+) estão documentadas`)

// legendUntested apanha "As 32 por testar não são esquecimento".
var legendUntested = regexp.MustCompile(`As (\d+) por testar`)

// collapseSpaces junta corridas de espaço e quebra de linha num único espaço.
//
// A legenda é um bloco de texto com mudanças de linha no meio das frases —
// "As 141 estão\ndocumentadas" — e sem isto as regexes falhariam por causa da
// quebra, não por causa do número.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func specDescription(t *testing.T) string {
	t.Helper()
	doc := carregarEspecificacao(t)
	info, ok := doc["info"].(map[string]any)
	if !ok {
		t.Fatal("especificação sem secção info")
	}
	description, ok := info["description"].(string)
	if !ok {
		t.Fatal("info.description ausente ou não é texto")
	}
	return collapseSpaces(description)
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("número ilegível %q: %v", s, err)
	}
	return n
}

// TestEvidenceLegendMatchesTable é o gate que apanha a divergência medida.
//
// A legenda é o único sítio onde os totais são ESCRITOS À MÃO dentro de um
// documento que é gerado. É por isso o sítio onde eles envelhecem primeiro, e
// o pior sítio possível: /docs serve-a a quem consome a API.
func TestEvidenceLegendMatchesTable(t *testing.T) {
	counts := countMarks(marksFromTable(t))
	description := specDescription(t)

	totals := legendTotals.FindStringSubmatch(description)
	if totals == nil {
		t.Fatalf("info.description não traz o parágrafo de totais no formato "+
			"`**N %s, N %s, N %s, N %s**` — se o formato mudou, actualize legendTotals",
			markConfirmed, markUnobserved, markFailed, markUntested)
	}
	for i, mark := range orderedMarks {
		if got := mustAtoi(t, totals[i+1]); got != counts[mark] {
			t.Errorf("legenda diz %d %s, mas %s tem %d",
				got, mark, evidenceTableFile, counts[mark])
		}
	}

	if m := legendDocumented.FindStringSubmatch(description); m == nil {
		t.Errorf("info.description não traz \"As N estão documentadas\"")
	} else if got := mustAtoi(t, m[1]); got != counts.total() {
		t.Errorf("legenda diz \"As %d estão documentadas\", mas %s classifica %d operações",
			got, evidenceTableFile, counts.total())
	}

	if m := legendUntested.FindStringSubmatch(description); m == nil {
		t.Errorf("info.description não traz \"As N por testar\"")
	} else if got := mustAtoi(t, m[1]); got != counts[markUntested] {
		t.Errorf("legenda diz \"As %d por testar\", mas %s tem %d %s",
			got, evidenceTableFile, counts[markUntested], markUntested)
	}
}
