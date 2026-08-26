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

// Este ficheiro RECONCILIA as quatro fontes que contam evidência.
//
// POR QUE ELE EXISTE. Os gates que já cá estavam verificam que cada operação
// TEM marca (TestOpenAPISummariesTrazemMarcaDeEvidencia) e que o documento
// embutido está em dia com as fontes (TestOpenAPIGeradoEstaAtualizado). Nenhum
// verifica que os NÚMEROS batem entre si — e foi por aí que a divergência
// passou: em 2d96535, com as formas antigas ainda no contrato, o
// `evidencias.tsv` e o relatório diziam 232 operações (178/13/6/35) enquanto a
// legenda de `info.description`, servida a toda a gente em /docs, continuava a
// dizer "As 141 estão documentadas" e "98 ✅, 8 🟡, 3 ❌, 32 ⬜". Toda a suíte
// passava.
//
// Desde 3a0b48b4 o contrato voltou a 141 operações, agora com um nome só por
// capacidade. O gate não conhece nenhum desses números: ele lê a fonte única e
// exige que as outras três a espelhem, pelo que sobrevive ao próximo lote de
// promoções sem ser tocado.
//
// A causa é estrutural, não distracção: a marca de cada rota é GERADA da
// tabela, mas o RESUMO da legenda e o relatório são escritos à mão. Um número
// escrito à mão ao lado de um número gerado diverge no primeiro dia em que
// alguém acrescenta uma rota. Este gate liga os dois.
//
// As quatro fontes:
//   1. api/openapi/evidencias.tsv          — a fonte única declarada
//   2. a especificação EMBUTIDA            — marca no summary de cada operação
//   3. info.description da especificação   — a legenda com os totais
//   4. docs/OPENAPI-EVIDENCIAS.md          — resumo, tabela por grupo, total

const (
	evidenceTableFile  = "evidencias.tsv"
	evidenceReportFile = "../../docs/OPENAPI-EVIDENCIAS.md"

	// evidenceTableColumns é a forma de uma linha: método, caminho, marca.
	evidenceTableColumns = 3
)

// As quatro marcas, nomeadas. Um símbolo solto repetido em cinco sítios é o
// mesmo defeito à espera de divergir (ADR-0004).
const (
	markConfirmed  = "✅"
	markUnobserved = "🟡"
	markFailed     = "❌"
	markUntested   = "⬜"
)

// orderedMarks fixa a ordem em que os totais aparecem na legenda e no
// relatório: ✅, 🟡, ❌, ⬜.
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
func marksFromTable(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(openapiSpecRoot, evidenceTableFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("tabela de evidência: %v", err)
	}
	marks := map[string]string{}
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
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

// reportRow é uma linha da tabela "Por grupo": o nome e as cinco contagens.
type reportRow struct {
	group      string
	operations int
	byMark     markCounts
}

// stripEmphasis tira o negrito de markdown de uma célula: "**141**" -> "141".
func stripEmphasis(cell string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(cell), "*"))
}

// parseGroupTable lê a tabela sob "## Por grupo" do relatório.
//
// Devolve as linhas de grupo e a linha de Total em separado, porque o Total é
// uma afirmação sobre a soma e tem de ser conferido CONTRA a soma — aceitá-lo
// como mais uma linha deixaria passar um total que não fecha com as parcelas.
func parseGroupTable(t *testing.T) (groups []reportRow, total reportRow) {
	t.Helper()
	raw, err := os.ReadFile(evidenceReportFile)
	if err != nil {
		t.Fatalf("relatório de evidência: %v", err)
	}

	const groupHeading = "## Por grupo"
	inSection := false
	var found bool
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = trimmed == groupHeading
			if inSection {
				found = true
			}
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		// Grupo + operações + quatro marcas.
		if len(cells) != 2+len(orderedMarks) {
			continue
		}
		name := stripEmphasis(cells[0])
		// Salta o cabeçalho e a linha de separação.
		if name == "" || name == "Grupo" || strings.HasPrefix(name, "-") {
			continue
		}
		operations, err := strconv.Atoi(stripEmphasis(cells[1]))
		if err != nil {
			continue
		}
		row := reportRow{group: name, operations: operations, byMark: markCounts{}}
		for i, mark := range orderedMarks {
			row.byMark[mark] = mustAtoi(t, stripEmphasis(cells[2+i]))
		}
		if name == "Total" {
			total = row
			continue
		}
		groups = append(groups, row)
	}
	if !found {
		t.Fatalf("%s não tem secção %q", evidenceReportFile, groupHeading)
	}
	return groups, total
}

// specGroupCounts agrupa as operações da especificação pela primeira etiqueta,
// que é o "grupo" com que o relatório as apresenta.
func specGroupCounts(t *testing.T) map[string]markCounts {
	t.Helper()
	byGroup := map[string]markCounts{}
	for key, op := range operacoesDaEspecificacao(t, carregarEspecificacao(t)) {
		if len(op.tags) == 0 {
			t.Errorf("%s não tem etiqueta — o relatório agrupa por etiqueta", key)
			continue
		}
		group := op.tags[0]
		if byGroup[group] == nil {
			byGroup[group] = markCounts{}
		}
		byGroup[group][markOfSummary(op.resumo)]++
	}
	return byGroup
}

// TestEvidenceReportMatchesSpec confronta o relatório com a especificação,
// grupo a grupo — e não só no total.
//
// O total é a afirmação agregada: fecha mesmo quando dois grupos erram em
// sentidos opostos. É a tabela por grupo que diz onde.
func TestEvidenceReportMatchesSpec(t *testing.T) {
	groups, total := parseGroupTable(t)
	fromSpec := specGroupCounts(t)

	seen := map[string]bool{}
	summed := markCounts{}
	summedOperations := 0

	for _, row := range groups {
		seen[row.group] = true
		summedOperations += row.operations
		for _, mark := range orderedMarks {
			summed[mark] += row.byMark[mark]
		}

		actual, present := fromSpec[row.group]
		if !present {
			t.Errorf("relatório tem o grupo %q, que não existe como etiqueta na especificação", row.group)
			continue
		}
		for _, mark := range orderedMarks {
			if row.byMark[mark] != actual[mark] {
				t.Errorf("grupo %q: relatório diz %d %s, especificação tem %d",
					row.group, row.byMark[mark], mark, actual[mark])
			}
		}
		if row.operations != actual.total() {
			t.Errorf("grupo %q: relatório diz %d operações, especificação tem %d",
				row.group, row.operations, actual.total())
		}
	}

	var missing []string
	for group := range fromSpec {
		if !seen[group] {
			missing = append(missing, group)
		}
	}
	sort.Strings(missing)
	for _, group := range missing {
		t.Errorf("etiqueta %q existe na especificação e não tem linha na tabela por grupo do relatório", group)
	}

	// A linha de Total tem de fechar com a soma das parcelas...
	for _, mark := range orderedMarks {
		if total.byMark[mark] != summed[mark] {
			t.Errorf("linha Total diz %d %s, mas as parcelas somam %d",
				total.byMark[mark], mark, summed[mark])
		}
	}
	if total.operations != summedOperations {
		t.Errorf("linha Total diz %d operações, mas as parcelas somam %d",
			total.operations, summedOperations)
	}
	// ...e com a fonte única.
	tableCounts := countMarks(marksFromTable(t))
	for _, mark := range orderedMarks {
		if total.byMark[mark] != tableCounts[mark] {
			t.Errorf("linha Total diz %d %s, mas %s tem %d",
				total.byMark[mark], mark, evidenceTableFile, tableCounts[mark])
		}
	}
	if total.operations != tableCounts.total() {
		t.Errorf("linha Total diz %d operações, mas %s classifica %d",
			total.operations, evidenceTableFile, tableCounts.total())
	}
}

// reportValidationLine apanha as quatro linhas do bloco "Validação:" do resumo
// quantitativo, na ordem OK / AMR / ERR / NT.
var reportValidationLine = regexp.MustCompile(`(?m)^\s*(OK|AMR|ERR|NT)\s+.*?:\s*(\d+)\s*$`)

// validationPrefixToMark liga o rótulo do resumo quantitativo à marca. O
// resumo usa rótulos ASCII porque vive dentro de um bloco de código.
var validationPrefixToMark = map[string]string{
	"OK":  markConfirmed,
	"AMR": markUnobserved,
	"ERR": markFailed,
	"NT":  markUntested,
}

// reportDocumented apanha "Operações documentadas: 141".
var reportDocumented = regexp.MustCompile(`(?m)^\s*Operações documentadas:\s*(\d+)\s*$`)

// TestEvidenceReportSummaryMatchesTable confere o bloco de resumo do
// relatório, que é a parte que as pessoas leem e a que ninguém recalcula.
func TestEvidenceReportSummaryMatchesTable(t *testing.T) {
	raw, err := os.ReadFile(evidenceReportFile)
	if err != nil {
		t.Fatalf("relatório de evidência: %v", err)
	}
	report := string(raw)
	counts := countMarks(marksFromTable(t))

	matches := reportValidationLine.FindAllStringSubmatch(report, -1)
	if len(matches) != len(orderedMarks) {
		t.Fatalf("bloco \"Validação:\" tem %d linhas reconhecidas, esperava %d — "+
			"se os rótulos mudaram, actualize reportValidationLine", len(matches), len(orderedMarks))
	}
	for _, m := range matches {
		mark, known := validationPrefixToMark[m[1]]
		if !known {
			t.Errorf("rótulo %q sem marca correspondente", m[1])
			continue
		}
		if got := mustAtoi(t, m[2]); got != counts[mark] {
			t.Errorf("resumo diz %s %d, mas %s tem %d %s",
				m[1], got, evidenceTableFile, counts[mark], mark)
		}
	}

	if m := reportDocumented.FindStringSubmatch(report); m == nil {
		t.Errorf("resumo não traz \"Operações documentadas: N\"")
	} else if got := mustAtoi(t, m[1]); got != counts.total() {
		t.Errorf("resumo diz \"Operações documentadas: %d\", mas %s classifica %d",
			got, evidenceTableFile, counts.total())
	}
}
