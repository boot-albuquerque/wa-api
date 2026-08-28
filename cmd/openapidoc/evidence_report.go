package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file generates the two tables of docs/OPENAPI-EVIDENCIAS.md that are
// mechanically derivable from api/openapi/evidencias.tsv and the merged
// specification: "Por grupo" (counts by tag) and "Tabela completa" (one row
// per operation). Everything else in that document — the intro, the
// deep-dive sections on the ✅/🟡/⬜ groups, the "Resumo quantitativo" block
// that needs router-registration facts this tool does not read — stays
// hand-written in docs/openapi-evidencias-prosa.md, spliced at two markers.
//
// WHY A FRAGMENT-PLUS-SPLICE INSTEAD OF ONE MORE FULL TEMPLATE (F239/F282,
// 2026-08-28). Before this, docs/OPENAPI-EVIDENCIAS.md was entirely
// hand-maintained, and pkg/bootstrap/openapi_reconciliation_test.go existed
// SOLELY to catch it drifting from the real source (evidencias.tsv, the
// embedded spec, the legend in base.yaml) — four independent numbers for the
// same fact, checked pairwise. Generating the two tables removes the
// possibility of that specific drift by construction: there is no third copy
// to disagree with the first two. The narrative sections stay hand-written
// because they carry qualitative judgment (why a route is blocked, what the
// next experiment is) that a TSV cell cannot hold — turning THEM into
// generated content would mean inventing a second free-text column just to
// paste into markdown, which is what evidencias.tsv's `evidência` column
// already is for the per-route case.

const (
	// evidenceProsaFile is the hand-written fragment: everything in the
	// report except the two generated tables, with splice markers where
	// they go. Relative to the working directory the tool runs from (repo
	// root), the same way defaultOutput above is — not relative to -root,
	// which a caller can point elsewhere without moving docs/.
	evidenceProsaFile = "docs/openapi-evidencias-prosa.md"
	// evidenceReportOut is the generated file the fragment is merged into.
	evidenceReportOut = "docs/OPENAPI-EVIDENCIAS.md"

	marcaResumoMarker      = "<!-- GERADO:POR-GRUPO -->"
	marcaTabelaMarker      = "<!-- GERADO:TABELA-COMPLETA -->"
	evidenceTableRowFields = evidenceColumns
)

// marcasDeEvidencia is shared with applyEvidence's four-symbol alphabet —
// used here to strip the mark prefix a route's summary carries after
// Merge() has already run applyEvidence over it.
var marcasDeEvidencia = []string{"✅", "🟡", "❌", "⬜"}

// evidenceRow is one line of evidencias.tsv, all six columns.
type evidenceRow struct {
	metodo     string
	caminho    string
	marca      string
	data       string
	observador string
	evidencia  string
}

func (r evidenceRow) key() string { return strings.ToUpper(r.metodo) + " " + r.caminho }

// readEvidenceRows re-reads the table with the full six-column shape.
// applyEvidence (main.go) reads the same file for the three-column shape it
// needs to apply marks — two readers of one file, not two copies of the
// data, because the extra columns are meaningless to that function and
// forcing it to carry them would couple mark-application to report
// generation for no reason.
func readEvidenceRows(path string) ([]evidenceRow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tabela de evidência: %w", err)
	}
	var out []evidenceRow
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != evidenceTableRowFields {
			return nil, fmt.Errorf("%s:%d: esperava %d colunas separadas por tabulação, veio %d",
				evidenceFile, lineNo+1, evidenceTableRowFields, len(fields))
		}
		out = append(out, evidenceRow{
			metodo:     fields[0],
			caminho:    fields[1],
			marca:      fields[2],
			data:       fields[3],
			observador: fields[4],
			evidencia:  fields[5],
		})
	}
	return out, nil
}

// legacyOfCanonical inverts caminhos.tsv: for each canonical "METHOD path"
// it gives the legacy "METHOD path" that clones onto it, or "" when none —
// the report renders that as "—".
//
// A canonical route can have at most one legacy origin in this table today;
// if a future entry gave it two, the second silently overwriting the first
// would hide a real ambiguity, so that case is an error instead.
func legacyOfCanonical(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tabela de caminhos: %w", err)
	}
	out := map[string]string{}
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != pathTableColumns {
			return nil, fmt.Errorf("%s:%d: esperava %d colunas, veio %d",
				pathsFile, lineNo+1, pathTableColumns, len(fields))
		}
		legacyKey := strings.ToUpper(fields[0]) + " " + fields[1]
		canonicalKey := strings.ToUpper(fields[2]) + " " + fields[3]
		if prev, dup := out[canonicalKey]; dup {
			return nil, fmt.Errorf("%s: %q já tem origem legada %q, e %q também aponta para lá — "+
				"duas origens para o mesmo caminho canónico não é o que \"substitui\" pode expressar",
				pathsFile, canonicalKey, prev, legacyKey)
		}
		out[canonicalKey] = legacyKey
	}
	return out, nil
}

// reportOperation is one row's worth of data pulled from the merged spec,
// keyed the same way as evidenceRow.
type reportOperation struct {
	metodo  string
	caminho string
	grupo   string
	titulo  string
}

// operationsFromMergedPaths reads tag and summary (mark prefix stripped)
// for every operation in the merged document — the same map Merge() already
// built, so this never re-reads or re-parses the path fragments.
func operationsFromMergedPaths(paths map[string]any) (map[string]reportOperation, error) {
	out := map[string]reportOperation{}
	var semTag []string
	for caminho, item := range paths {
		methods, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for metodo, body := range methods {
			op, ok := body.(map[string]any)
			if !ok {
				continue
			}
			key := strings.ToUpper(metodo) + " " + caminho
			tags, _ := op["tags"].([]any)
			if len(tags) == 0 {
				semTag = append(semTag, key)
				continue
			}
			grupo, _ := tags[0].(string)
			summary, _ := op["summary"].(string)
			out[key] = reportOperation{
				metodo:  strings.ToUpper(metodo),
				caminho: caminho,
				grupo:   grupo,
				titulo:  stripMark(summary),
			}
		}
	}
	sort.Strings(semTag)
	if len(semTag) > 0 {
		return nil, fmt.Errorf("%d operações sem etiqueta — o relatório agrupa por etiqueta:\n  %s",
			len(semTag), strings.Join(semTag, "\n  "))
	}
	return out, nil
}

// stripMark removes a leading evidence mark and the space applyEvidence put
// after it, so the report's "Título" column shows the operation's own
// summary — the mark has its own column ("Teste") already.
func stripMark(summary string) string {
	for _, mark := range marcasDeEvidencia {
		if strings.HasPrefix(summary, mark+" ") {
			return summary[len(mark)+len(" "):]
		}
	}
	return summary
}

// evidenceCounts is shared shape with the reconciliation gate
// (pkg/bootstrap/openapi_reconciliation_test.go): map from mark to count.
type evidenceCounts map[string]int

func (c evidenceCounts) total() int {
	sum := 0
	for _, n := range c {
		sum += n
	}
	return sum
}

// buildPorGrupoTable renders "## Por grupo": one row per tag, sorted, plus
// a Total row that is the sum of the others — never an independent count,
// so it cannot itself drift from its own parts.
func buildPorGrupoTable(rows []evidenceRow, ops map[string]reportOperation) (string, error) {
	byGroup := map[string]evidenceCounts{}
	var missingOp []string
	for _, r := range rows {
		op, ok := ops[r.key()]
		if !ok {
			missingOp = append(missingOp, r.key())
			continue
		}
		if byGroup[op.grupo] == nil {
			byGroup[op.grupo] = evidenceCounts{}
		}
		byGroup[op.grupo][r.marca]++
	}
	sort.Strings(missingOp)
	if len(missingOp) > 0 {
		return "", fmt.Errorf("%d linhas de %s sem operação correspondente na especificação:\n  %s",
			len(missingOp), evidenceFile, strings.Join(missingOp, "\n  "))
	}

	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	var sb strings.Builder
	sb.WriteString("## Por grupo\n\n")
	sb.WriteString("| Grupo | Operações | ✅ | 🟡 | ❌ | ⬜ |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|\n")
	total := evidenceCounts{}
	totalOps := 0
	for _, g := range groups {
		c := byGroup[g]
		totalOps += c.total()
		for _, m := range marcasDeEvidencia {
			total[m] += c[m]
		}
		fmt.Fprintf(&sb, "| %s | %d | %d | %d | %d | %d |\n",
			g, c.total(), c["✅"], c["🟡"], c["❌"], c["⬜"])
	}
	fmt.Fprintf(&sb, "| **Total** | **%d** | **%d** | **%d** | **%d** | **%d** |\n",
		totalOps, total["✅"], total["🟡"], total["❌"], total["⬜"])
	return sb.String(), nil
}

// buildTabelaCompleta renders "## Tabela completa": one row per operation,
// sorted by group then path then method — the same order the hand-written
// table used, so a diff against the previous version stays readable.
//
// evidência falls back to a marker phrase, never to the banned generic
// sentence (SPEC §1) — an EMPTY evidência is not silently promoted to look
// measured.
func buildTabelaCompleta(rows []evidenceRow, ops map[string]reportOperation, legacyOf map[string]string) (string, error) {
	// caminho and metodo are kept RAW (no markdown backticks) for sorting —
	// wrapping them first and sorting the wrapped form put "`" (0x60) into
	// the comparison, which sorts AFTER "/" (0x2F): "/admin/users/{id}`"
	// then compared byte-for-byte against "/admin/users/{id}/full`" put
	// the SHORTER path after the longer one, inverting the expected order.
	// The backticks are added only when rendering each row to text.
	type line struct {
		grupo, metodo, caminho, substitui, marca, titulo, evidencia string
	}
	var lines []line
	var missingOp []string
	for _, r := range rows {
		op, ok := ops[r.key()]
		if !ok {
			missingOp = append(missingOp, r.key())
			continue
		}
		substitui := legacyOf[r.key()]
		if substitui == "" {
			substitui = "—"
		} else {
			parts := strings.SplitN(substitui, " ", 2)
			substitui = "`" + parts[0] + " " + parts[1] + "`"
		}
		evidencia := strings.TrimSpace(r.evidencia)
		if evidencia == "" {
			evidencia = "ainda não remedida por esta campanha (F239/F282) — ver RFC-cobertura-evidencia-rotas.md."
		}
		lines = append(lines, line{
			grupo:     op.grupo,
			metodo:    r.metodo,
			caminho:   r.caminho,
			substitui: substitui,
			marca:     r.marca,
			titulo:    op.titulo,
			evidencia: evidencia,
		})
	}
	sort.Strings(missingOp)
	if len(missingOp) > 0 {
		return "", fmt.Errorf("%d linhas de %s sem operação correspondente na especificação:\n  %s",
			len(missingOp), evidenceFile, strings.Join(missingOp, "\n  "))
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].grupo != lines[j].grupo {
			return lines[i].grupo < lines[j].grupo
		}
		if lines[i].caminho != lines[j].caminho {
			return lines[i].caminho < lines[j].caminho
		}
		return lines[i].metodo < lines[j].metodo
	})

	var sb strings.Builder
	sb.WriteString("## Tabela completa\n\n")
	sb.WriteString("A coluna **Evidência** traz o observador CONCRETO onde ele foi registado, " +
		"lido de `api/openapi/evidencias.tsv`. Linhas ainda por remedir dizem-no explicitamente " +
		"— ver `RFC-cobertura-evidencia-rotas.md`.\n\n")
	sb.WriteString("| Grupo | Método | Caminho | Substitui | Teste | Título | Evidência |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	for _, l := range lines {
		fmt.Fprintf(&sb, "| %s | `%s` | `%s` | %s | %s | %s | %s |\n",
			l.grupo, l.metodo, l.caminho, l.substitui, l.marca, l.titulo, l.evidencia)
	}
	return sb.String(), nil
}

// spliceMarker replaces exactly one occurrence of a marker line with
// content, so a marker present zero or more-than-once times fails loudly
// instead of silently writing nothing or overwriting the wrong occurrence.
func spliceMarker(fragment, marker, content, prosaPath string) (string, error) {
	n := strings.Count(fragment, marker)
	if n != 1 {
		return "", fmt.Errorf("%s: marcador %q aparece %d vezes, queria exactamente 1",
			prosaPath, marker, n)
	}
	return strings.Replace(fragment, marker, strings.TrimRight(content, "\n"), 1), nil
}

// GenerateEvidenceReport reads the hand-written fragment and splices in the
// two generated tables, using the already-merged spec (paths, post
// applyCanonicalPaths and applyEvidence) so this never disagrees with what
// Merge() actually produced.
//
// prosaPath is a PARAMETER and not the evidenceProsaFile constant, for the
// same reason -out is a flag and not defaultOutput baked in: a caller
// running from a different working directory (a test in pkg/bootstrap, in
// particular — TestOpenAPIGeradoEstaAtualizado shells out to this binary
// with an ABSOLUTE -root, and this tool's CWD there is the test's package
// directory, not the repo root) needs the real path, not one relative to a
// CWD that may not be the repo root.
func GenerateEvidenceReport(root string, paths map[string]any, prosaPath string) ([]byte, error) {
	rows, err := readEvidenceRows(filepath.Join(root, evidenceFile))
	if err != nil {
		return nil, err
	}
	ops, err := operationsFromMergedPaths(paths)
	if err != nil {
		return nil, err
	}
	legacyOf, err := legacyOfCanonical(filepath.Join(root, pathsFile))
	if err != nil {
		return nil, err
	}

	porGrupo, err := buildPorGrupoTable(rows, ops)
	if err != nil {
		return nil, err
	}
	tabela, err := buildTabelaCompleta(rows, ops, legacyOf)
	if err != nil {
		return nil, err
	}

	fragment, err := os.ReadFile(prosaPath)
	if err != nil {
		return nil, fmt.Errorf("fragmento do relatório: %w", err)
	}
	out := string(fragment)
	if out, err = spliceMarker(out, marcaResumoMarker, porGrupo, prosaPath); err != nil {
		return nil, err
	}
	if out, err = spliceMarker(out, marcaTabelaMarker, tabela, prosaPath); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}
