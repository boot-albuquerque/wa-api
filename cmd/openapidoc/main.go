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
	yamlIndent   = 2
	defaultRoot  = "api/openapi"
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
		fmt.Println("openapidoc: atualizado")
		return
	}
	if err := os.WriteFile(*out, rendered, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapidoc: %v\n", err)
		os.Exit(1)
	}
	paths, _ := merged["paths"].(map[string]any)
	fmt.Printf("openapidoc: %s escrito — %d caminhos\n", *out, len(paths))
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
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
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

// evidenceColumns is the shape of one line: method, path, mark.
const evidenceColumns = 3

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
