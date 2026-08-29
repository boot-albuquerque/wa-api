// Command openapidoc merges the OpenAPI fragments into the single document
// that the server embeds and Swagger UI reads.
//
// WHY FRAGMENTS AND A MERGE STEP. The specification covers 141 route entries
// across twelve families. Keeping it in one file makes every author write in
// the same place, and two authors in the same place is a conflict per pull
// request. One file per family removes the conflict without giving each
// family its own idea of what a response looks like: the shared parts live in
// base.yaml and are referenced, never copied.
//
// The merged document is COMMITTED (api/openapi/openapi.yaml) so the binary
// can embed it with go:embed and so a reviewer sees the real diff. A test
// re-runs the merge and refuses a stale file — the same golden discipline the
// repository already uses for routes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	baseFile   = "base.yaml"
	pathsDir   = "paths"
	schemasDir = "schemas"
	// evidenceFile carries the per-route evidence mark that gets prefixed onto
	// each summary. It lives OUTSIDE the path fragments on purpose: pasted into
	// a summary, the mark drifts the moment someone rewrites the title, and a
	// second paste doubles it. One table, applied at merge time, cannot do
	// either.
	evidenceFile = "evidencias.tsv"
	// pathsFile carries the canonical-path standardisation. The generator
	// clones each legacy operation onto its canonical path rather than having
	// anyone write 91 near-duplicates by hand — duplicates drift, a
	// transformation cannot.
	pathsFile   = "caminhos.tsv"
	yamlIndent  = 2
	defaultRoot = "api/openapi"
	// defaultOutput lives inside the package that embeds it: go:embed cannot
	// reach outside its own directory, and a second copy of the document is a
	// second thing to keep in step.
	defaultOutput = "pkg/presentation/http/apidocs/openapi.yaml"
)

// generatedHeader warns the reader before they edit the wrong file.
const generatedHeader = `# GERADO por "go run ./cmd/openapidoc" — NÃO EDITE À MÃO.
#
# As fontes são api/openapi/base.yaml (info, tags, segurança, componentes
# partilhados), api/openapi/paths/<grupo>.yaml (um por família de rotas) e
# api/openapi/schemas/<grupo>.yaml (esquemas próprios de cada família).
#
# O teste TestOpenAPIGeradoEstaAtualizado recusa este ficheiro se ele
# divergir das fontes.
`

func main() {
	root := flag.String("root", defaultRoot, "directory holding base.yaml, paths/ and schemas/")
	out := flag.String("out", defaultOutput, "path of the merged document")
	evidenceOut := flag.String("evidence-out", evidenceReportOut, "path of the generated evidence report")
	evidenceProsa := flag.String("evidence-prosa", evidenceProsaFile, "path of the hand-written evidence report fragment")
	check := flag.Bool("check", false, "do not write; fail if the committed file is stale")
	flag.Parse()

	merged, err := Merge(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: %v\n", err)
		os.Exit(1)
	}
	rendered, err := Render(merged)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: %v\n", err)
		os.Exit(1)
	}
	paths, _ := merged["paths"].(map[string]any)
	evidenceReport, err := GenerateEvidenceReport(*root, paths, *evidenceProsa)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: relatório de evidência: %v\n", err)
		os.Exit(1)
	}
	if *check {
		current, readErr := os.ReadFile(*out)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "openapidoc: %v\n", readErr)
			os.Exit(1)
		}
		if string(current) != string(rendered) {
			fmt.Fprintf(os.Stderr, "openapidoc: %s está desatualizado — corra `go run ./cmd/openapidoc`\n", *out)
			os.Exit(1)
		}
		currentReport, readErr := os.ReadFile(*evidenceOut)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "openapidoc: %v\n", readErr)
			os.Exit(1)
		}
		if string(currentReport) != string(evidenceReport) {
			fmt.Fprintf(os.Stderr, "openapidoc: %s está desatualizado — corra `go run ./cmd/openapidoc`\n", *evidenceOut)
			os.Exit(1)
		}
		fmt.Println("openapidoc: atualizado")
		return
	}
	if err := os.WriteFile(*out, rendered, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*evidenceOut, evidenceReport, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("openapidoc: %s escrito — %d caminhos\n", *out, len(paths))
	fmt.Printf("openapidoc: %s escrito\n", *evidenceOut)
}

// Merge reads base.yaml and every fragment and returns the whole document.
//
// Fragment collision is an ERROR and not a silent last-writer-wins: two files
// claiming the same path means two authors documented the same route, and
// choosing one of them quietly is how half the documentation disappears in a
// merge.
func Merge(root string) (map[string]any, error) {
	doc, err := readYAML(filepath.Join(root, baseFile))
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}

	paths := map[string]any{}
	if err := mergeDir(filepath.Join(root, pathsDir), paths, "caminho"); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("nenhum caminho encontrado em %s/%s", root, pathsDir)
	}
	if err := applyCanonicalPaths(filepath.Join(root, pathsFile), paths); err != nil {
		return nil, err
	}
	if err := applyEvidence(filepath.Join(root, evidenceFile), paths); err != nil {
		return nil, err
	}
	doc["paths"] = paths

	components, _ := doc["components"].(map[string]any)
	if components == nil {
		components = map[string]any{}
		doc["components"] = components
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		schemas = map[string]any{}
	}
	if err := mergeDir(filepath.Join(root, schemasDir), schemas, "esquema"); err != nil {
		return nil, err
	}
	components["schemas"] = schemas

	return doc, nil
}

// mergeDir folds every YAML file in dir into dst, refusing duplicate keys.
func mergeDir(dir string, dst map[string]any, kind string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	origin := map[string]string{}
	for _, name := range names {
		fragment, err := readYAML(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for key, value := range fragment {
			if prev, dup := origin[key]; dup {
				return fmt.Errorf("%s %q declarado em %s e em %s: dois autores no mesmo sítio", kind, key, prev, name)
			}
			origin[key] = name
			dst[key] = value
		}
	}
	return nil
}

// applyCanonicalPaths MOVES each legacy operation onto its canonical path.
//
// WHY MOVE INSTEAD OF DUPLICATING. The canonical route IS the legacy route —
// same handler, same body, same responses. Documenting both would put two
// names for one operation in front of the reader, which is exactly what the
// standardisation exists to remove: a consumer choosing between `/chat/list`
// and `/chats/list` has to guess which one is preferred, and the answer is
// never in the operation itself.
//
// THE LEGACY ROUTE CONTINUES TO BE SERVED. It is removed from the CONTRACT,
// not from the service. An existing client keeps working; what changes is that
// the documentation describes one name, and the equivalence table in
// docs/ENDPOINTS.md is where the old name is looked up.
//
// The gate TestOpenAPICobreTodasAsRotasRegistadas knows this: a legacy route
// counts as covered when its canonical twin is documented.
func applyCanonicalPaths(path string, paths map[string]any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("tabela de caminhos: %w", err)
	}

	type destino struct{ metodo, caminho string }
	clones := map[string][]destino{}
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != pathTableColumns {
			return fmt.Errorf("%s:%d: esperava %d colunas, veio %d",
				pathsFile, lineNo+1, pathTableColumns, len(fields))
		}
		chave := strings.ToLower(fields[0]) + " " + fields[1]
		clones[chave] = append(clones[chave], destino{strings.ToLower(fields[2]), fields[3]})
	}

	var ausentes []string
	for chave, destinos := range clones {
		partes := strings.SplitN(chave, " ", 2)
		metodo, caminhoAntigo := partes[0], partes[1]

		item, ok := paths[caminhoAntigo].(map[string]any)
		if !ok {
			ausentes = append(ausentes, caminhoAntigo)
			continue
		}
		op, ok := item[metodo].(map[string]any)
		if !ok {
			ausentes = append(ausentes, strings.ToUpper(metodo)+" "+caminhoAntigo)
			continue
		}

		for _, d := range destinos {
			clone := deepCopy(op).(map[string]any)
			clone["summary"] = toString(clone["summary"])
			clone["description"] = canonicalNote(caminhoAntigo, metodo) + toString(clone["description"])
			if parametros := pathParameters(d.caminho); len(parametros) > 0 {
				clone["parameters"] = append(parametros, existingParameters(clone)...)
			}
			alvo, existe := paths[d.caminho].(map[string]any)
			if !existe {
				alvo = map[string]any{}
				paths[d.caminho] = alvo
			}
			alvo[d.metodo] = clone
		}

		// A operação antiga sai do contrato. Se o caminho ficar sem métodos,
		// sai também — um caminho vazio no documento seria uma entrada que o
		// Swagger desenha sem nada dentro.
		delete(item, metodo)
		if len(item) == 0 {
			delete(paths, caminhoAntigo)
		}
	}

	sort.Strings(ausentes)
	if len(ausentes) > 0 {
		return fmt.Errorf("%d rotas da tabela de caminhos sem operação documentada:\n  %s",
			len(ausentes), strings.Join(ausentes, "\n  "))
	}
	return nil
}

// toString reads a string field, tolerating absence.
func toString(v any) string { s, _ := v.(string); return s }

// pathTableColumns is the shape of one line: legacy method and path, canonical
// method and path.
const pathTableColumns = 4

// canonicalNote is prepended to the canonical operation, naming the old path
// it replaces.
//
// The note stays even though the old operation is gone from the document: a
// reader arriving from an existing integration searches for the name they
// know, and finding it here is what tells them where it went.
//
// CORRECTED 2026-08-29 (found live, via /docs, while testing F373/F374): this
// used to say the old path "continua a ser servido e não tem data de
// remoção" — true under the ORIGINAL F269 policy, false since the
// 2026-08-27 reversal to a clean cutover (api/openapi/CAMINHOS-CANONICOS.md:
// "está no OpenAPI: sim (canónico) / não (antigo, nunca esteve)... é
// servido? sim / não — 404... tempo de vida: permanente / terminou em
// 2026-08-27"). The generator template was never updated when the policy
// changed, so every canonicalized operation's doc carried a false claim —
// confirmed live: GET /session/qr answers 404, and the served /docs page
// still said otherwise. HOUSEKEEP F376.
func canonicalNote(caminhoAntigo, metodo string) string {
	return "> **Substitui `" + strings.ToUpper(metodo) + " " + caminhoAntigo +
		"`.** O caminho antigo **não responde mais** (404) desde 2026-08-27 " +
		"— corte limpo, não período de transição. Fica citado aqui só para " +
		"quem chega de uma integração antiga e procura o nome que conhecia. " +
		"A tabela de equivalência completa está em `docs/ENDPOINTS.md`.\n\n"
}

// pathParameters builds the OpenAPI parameter list for a templated path.
func pathParameters(caminho string) []any {
	var out []any
	for _, bruto := range strings.Split(caminho, "/") {
		if !strings.HasPrefix(bruto, "{") || !strings.HasSuffix(bruto, "}") {
			continue
		}
		nome := strings.Trim(bruto, "{}")
		out = append(out, map[string]any{
			"name":        nome,
			"in":          "path",
			"required":    true,
			"description": pathParamDescription(nome),
			"schema":      map[string]any{"type": "string", "example": pathParamExample(nome)},
		})
	}
	return out
}

func pathParamDescription(nome string) string {
	switch nome {
	case "group_jid":
		return "JID do grupo, no servidor `@g.us`. Substitui o campo `groupJID` do corpo — " +
			"se ambos vierem, o **corpo ganha**, para que um cliente a meio da migração não parta."
	case "community_jid":
		return "JID da comunidade, no servidor `@g.us`. Substitui o campo `communityJID` do corpo; " +
			"se ambos vierem, o corpo ganha."
	default:
		return "Identificador no caminho."
	}
}

func pathParamExample(nome string) string {
	switch nome {
	case "community_jid":
		return "120363430034334401@g.us"
	default:
		return "120363411669320145@g.us"
	}
}

func existingParameters(op map[string]any) []any {
	lista, _ := op["parameters"].([]any)
	return lista
}

// deepCopy clones a decoded YAML tree. Without it the canonical operation and
// the legacy one would share the same maps, and marking one deprecated would
// mark both.
func deepCopy(no any) any {
	switch v := no.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, filho := range v {
			out[k] = deepCopy(filho)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, filho := range v {
			out[i] = deepCopy(filho)
		}
		return out
	default:
		return no
	}
}

// applyEvidence prefixes every operation summary with its evidence mark.
//
// A route documented in paths/ with no line in the table is an ERROR, not a
// default: a new route must be classified, and letting silence pass for a
// clean bill is exactly how a coverage report starts lying.
//
// The reverse — a line for a route that no longer exists — is also an error,
// because a stale entry is what keeps a removed route looking measured.
func applyEvidence(path string, paths map[string]any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("tabela de evidência: %w", err)
	}
	marks := map[string]string{}
	for lineNo, rawLine := range strings.Split(string(raw), "\n") {
		// TrimRight only, not TrimSpace: F239/F282 added three OPTIONAL
		// trailing columns (data, observador, evidência), and most rows
		// still have them empty. TrimSpace treats a tab as whitespace and
		// would eat those trailing empty fields off the end of the line —
		// "GET\t/x\t✅\t\t\t" becomes "GET\t/x\t✅" and Split reports 3
		// fields instead of 6. \r survives on a file saved with CRLF; \n
		// is already gone from the Split above.
		line := strings.TrimRight(rawLine, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != evidenceColumns {
			return fmt.Errorf("%s:%d: esperava %d colunas separadas por tabulação, veio %d",
				evidenceFile, lineNo+1, evidenceColumns, len(fields))
		}
		marks[strings.ToUpper(fields[0])+" "+fields[1]] = fields[2]
	}

	used := map[string]bool{}
	var missing []string
	for route, item := range paths {
		methods, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for method, body := range methods {
			op, ok := body.(map[string]any)
			if !ok {
				continue
			}
			key := strings.ToUpper(method) + " " + route
			mark, known := marks[key]
			if !known {
				missing = append(missing, key)
				continue
			}
			used[key] = true
			summary, _ := op["summary"].(string)
			op["summary"] = mark + " " + strings.TrimSpace(summary)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("%d rotas documentadas sem classificação de evidência em %s — "+
			"classifique-as antes de as publicar:\n  %s",
			len(missing), evidenceFile, strings.Join(missing, "\n  "))
	}

	var stale []string
	for key := range marks {
		if !used[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		return fmt.Errorf("%d linhas em %s para rotas que já não existem na especificação — "+
			"uma entrada obsoleta é o que mantém uma rota removida com ar de medida:\n  %s",
			len(stale), evidenceFile, strings.Join(stale, "\n  "))
	}
	return nil
}

// evidenceColumns is the shape of one line: method, path, mark, and (F239/
// F282, 2026-08-28) the three columns the campaign added — data, observador,
// evidência. applyEvidence only needs the mark for the OpenAPI summary
// prefix; the extra three are read here (evidence_report.go re-reads the
// same file for its own six-field shape) purely so a malformed row is
// caught before it silently loses columns to strings.Split.
const evidenceColumns = 6

func readYAML(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Render serialises the document with a stable key order so the committed
// file diffs cleanly.
func Render(doc map[string]any) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(generatedHeader)
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}
