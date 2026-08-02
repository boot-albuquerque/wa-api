package main

// logcov — mede a cobertura de log de pkg/** conforme cmd/logcov/METRIC.md
// (secao 9 do plano de cobertura). Duas metricas:
//
//	func_coverage    = elegiveis que satisfazem L1 e L3 / elegiveis
//	errpath_coverage = caminhos de saida cobertos / caminhos de saida
//
// Estagio do gate nesta fase: advisory. A ferramenta imprime; nao falha.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type options struct {
	jsonOut      bool
	byPackage    bool
	listUncov    bool
	golden       bool
	coverProfile string
	root         string
	patterns     []string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "logcov:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	opt, err := parseFlags(args)
	if err != nil {
		return err
	}
	if opt.coverProfile != "" {
		return writeCoverageReport(opt.coverProfile, out)
	}
	rep, exempt, err := measure(opt)
	if err != nil {
		return err
	}
	rep.Exempt = exempt
	return emit(opt, rep, out)
}

func parseFlags(args []string) (*options, error) {
	opt := &options{}
	fs := flag.NewFlagSet("logcov", flag.ContinueOnError)
	fs.BoolVar(&opt.jsonOut, "json", false, "saida em JSON")
	fs.BoolVar(&opt.byPackage, "by-package", false, "detalha por pacote")
	fs.BoolVar(&opt.listUncov, "list-uncovered", false, "lista nominal das funcoes elegiveis sem log")
	fs.BoolVar(&opt.golden, "golden", false, "emite o golden de elegibilidade")
	fs.StringVar(&opt.coverProfile, "coverprofile", "", "relatorio de cobertura por pacote, com dedup de blocos")
	fs.StringVar(&opt.root, "root", "", "raiz do modulo (default: procura go.mod a partir do cwd)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	opt.patterns = normalizePatterns(fs.Args())
	if opt.root == "" {
		root, err := findModuleRoot()
		if err != nil {
			return nil, err
		}
		opt.root = root
	}
	return opt, nil
}

// normalizePatterns aceita `./pkg` e `pkg` como sinonimos de `./pkg/...`.
func normalizePatterns(args []string) []string {
	if len(args) == 0 {
		return []string{"./pkg/..."}
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		if !strings.HasSuffix(a, "...") {
			a = strings.TrimSuffix(a, "/") + "/..."
		}
		if !strings.HasPrefix(a, "./") && !strings.HasPrefix(a, "/") {
			a = "./" + a
		}
		out = append(out, a)
	}
	return out
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod nao encontrado a partir do diretorio atual")
		}
		dir = parent
	}
}

func measure(opt *options) (*report, int, error) {
	excludes, err := readExcludeFile(filepath.Join(opt.root, ".logcov-exclude"))
	if err != nil {
		return nil, 0, err
	}
	golangciPath := filepath.Join(opt.root, ".golangci.yml")
	depguard, err := readDepguardDirs(golangciPath)
	if err != nil {
		return nil, 0, err
	}
	if len(depguard) == 0 {
		if _, statErr := os.Stat(golangciPath); statErr == nil {
			return nil, 0, fmt.Errorf(
				".golangci.yml existe mas nenhum diretorio de depguard/camada-de-aplicacao foi extraido — " +
					"parsing pode ter quebrado; a restricao L1-b nao pode ser aplicada com seguranca")
		}
	}
	a := newAnalysis(opt.root, "wa-api", excludes, depguard)
	pkgs, err := a.load(opt.patterns...)
	if err != nil {
		return nil, 0, err
	}
	if a.loggerIface == nil {
		return nil, 0, fmt.Errorf("porta contracts.Logger nao resolvida: L1-a estaria quebrada, abortando")
	}
	rep := a.Analyze(pkgs)
	exempt, err := countExemptAnnotations(filepath.Join(opt.root, "pkg"))
	if err != nil {
		return nil, 0, err
	}
	return rep, exempt, nil
}

// ---------------------------------------------------------------------------
// Saida
// ---------------------------------------------------------------------------

func emit(opt *options, rep *report, out io.Writer) error {
	switch {
	case opt.golden:
		return emitGolden(rep, out)
	case opt.jsonOut:
		return emitJSON(rep, out)
	case opt.listUncov && opt.byPackage:
		return emitBudget(rep, out)
	case opt.listUncov:
		return emitUncovered(rep, out)
	case opt.byPackage:
		return emitByPackage(rep, out)
	}
	return emitSummary(rep, out)
}

// lineWriter acumula linhas num buffer e devolve o erro so' no flush: o erro
// de cada Fprintf individual e' redundante com o do Flush, e checar os dois
// espalha ruido por todos os emissores.
type lineWriter struct{ w *bufio.Writer }

func newLineWriter(out io.Writer) *lineWriter { return &lineWriter{w: bufio.NewWriter(out)} }

func (l *lineWriter) printf(format string, a ...any) { _, _ = fmt.Fprintf(l.w, format, a...) }

func (l *lineWriter) flush() error { return l.w.Flush() }

func emitGolden(rep *report, out io.Writer) error {
	w := newLineWriter(out)
	for _, e := range rep.Entries {
		state := "ELIGIBLE"
		if e.Excluded != "" {
			state = "EXCLUDED"
		}
		w.printf("%s\t%s\t%s\n", e.Key, state, e.status())
	}
	return w.flush()
}

type jsonForm struct {
	CallSites int `json:"call_sites"`
	Files     int `json:"files"`
}

type jsonPkg struct {
	Eligible     int     `json:"eligible"`
	Covered      int     `json:"covered"`
	FuncCoverage float64 `json:"func_coverage"`
	Paths        int     `json:"errpaths"`
	PathsCovered int     `json:"errpaths_covered"`
}

type jsonReport struct {
	Eligible          int                 `json:"eligible"`
	Covered           int                 `json:"covered"`
	FuncCoverage      float64             `json:"func_coverage"`
	FuncCovTenths     int                 `json:"func_coverage_tenths"`
	ErrPaths          int                 `json:"errpaths"`
	ErrPathsCovered   int                 `json:"errpaths_covered"`
	ErrPathCoverage   float64             `json:"errpath_coverage"`
	ErrPathCovTenths  int                 `json:"errpath_coverage_tenths"`
	Total             int                 `json:"total_functions"`
	ExemptAnnotations int                 `json:"exempt_annotations"`
	CallSitesTotal    int                 `json:"call_sites_total"`
	ByForm            map[string]jsonForm `json:"by_form"`
	ByPackage         map[string]jsonPkg  `json:"by_package"`
}

func buildJSON(rep *report) jsonReport {
	jr := jsonReport{
		Eligible:          rep.Eligible,
		Covered:           rep.Covered,
		FuncCoverage:      pct(rep.Covered, rep.Eligible),
		FuncCovTenths:     tenths(rep.Covered, rep.Eligible),
		ErrPaths:          rep.Paths,
		ErrPathsCovered:   rep.PathsCovered,
		ErrPathCoverage:   pct(rep.PathsCovered, rep.Paths),
		ErrPathCovTenths:  tenths(rep.PathsCovered, rep.Paths),
		Total:             len(rep.Entries),
		ExemptAnnotations: rep.Exempt,
		ByForm:            map[string]jsonForm{},
		ByPackage:         map[string]jsonPkg{},
	}
	for name, f := range rep.Forms {
		jr.ByForm[name] = jsonForm{CallSites: f.CallSites, Files: f.Files()}
		jr.CallSitesTotal += f.CallSites
	}
	for name, s := range packageStats(rep) {
		jr.ByPackage[name] = jsonPkg{
			Eligible: s.eligible, Covered: s.covered,
			FuncCoverage: pct(s.covered, s.eligible),
			Paths:        s.paths, PathsCovered: s.pathsCovered,
		}
	}
	return jr
}

func emitJSON(rep *report, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(buildJSON(rep))
}

type pkgStat struct {
	eligible, covered, paths, pathsCovered int
}

func packageStats(rep *report) map[string]*pkgStat {
	m := map[string]*pkgStat{}
	for _, e := range rep.Entries {
		if e.Excluded != "" {
			continue
		}
		s := m[e.PkgRel]
		if s == nil {
			s = &pkgStat{}
			m[e.PkgRel] = s
		}
		s.eligible++
		if e.covered() {
			s.covered++
		}
		for _, p := range e.Paths {
			s.paths++
			if p.covered {
				s.pathsCovered++
			}
		}
	}
	return m
}

func sortedPkgNames(m map[string]*pkgStat) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func emitSummary(rep *report, out io.Writer) error {
	f := rep.Forms
	_, err := fmt.Fprintf(out, `logcov — cobertura de log de pkg/** (METRIC.md, estagio advisory)

func_coverage    = %.1f%% (%d/%d elegiveis)
errpath_coverage = %.1f%% (%d/%d caminhos de saida)
eligible         = %d
exempt           = %d
total_functions  = %d
call_sites_total = %d (port=%d zerolog=%d hlog=%d)
usecase_files_port = %d
`,
		pct(rep.Covered, rep.Eligible), rep.Covered, rep.Eligible,
		pct(rep.PathsCovered, rep.Paths), rep.PathsCovered, rep.Paths,
		rep.Eligible, rep.Exempt, len(rep.Entries),
		f["port"].CallSites+f["zerolog"].CallSites+f["hlog"].CallSites,
		f["port"].CallSites, f["zerolog"].CallSites, f["hlog"].CallSites,
		f["port"].Files(),
	)
	return err
}

func emitByPackage(rep *report, out io.Writer) error {
	stats := packageStats(rep)
	w := newLineWriter(out)
	w.printf("%-45s %8s %8s %8s %10s %8s\n", "PACOTE", "ELEG", "COBERTO", "FUNC%", "ERRPATHS", "ERRP%")
	for _, name := range sortedPkgNames(stats) {
		s := stats[name]
		w.printf("%-45s %8d %8d %7.1f%% %10d %7.1f%%\n",
			name, s.eligible, s.covered, pct(s.covered, s.eligible), s.paths, pct(s.pathsCovered, s.paths))
	}
	return w.flush()
}

func emitUncovered(rep *report, out io.Writer) error {
	w := newLineWriter(out)
	for _, e := range rep.Entries {
		if e.Excluded != "" || e.covered() {
			continue
		}
		w.printf("%s\t%s:%d\t%s\n", e.Key, e.FileRel, e.Line, e.status())
	}
	return w.flush()
}

// emitBudget produz .omc/reports/coverage-budget.md — o backlog nominal que as
// fases F10-F15 consomem.
func emitBudget(rep *report, out io.Writer) error {
	w := newLineWriter(out)
	stats := packageStats(rep)
	w.printf("# Orcamento de cobertura de log — gerado por `go run ./cmd/logcov -list-uncovered -by-package`\n\n")
	w.printf("Nao edite a mao. Regenerar apos qualquer mudanca em `pkg/**` ou nas regras de `cmd/logcov/METRIC.md`.\n\n")
	w.printf("## Resumo\n\n")
	w.printf("| metrica | valor |\n|---|---|\n")
	w.printf("| func_coverage | %.1f%% (%d/%d) |\n", pct(rep.Covered, rep.Eligible), rep.Covered, rep.Eligible)
	w.printf("| errpath_coverage | %.1f%% (%d/%d) |\n", pct(rep.PathsCovered, rep.Paths), rep.PathsCovered, rep.Paths)
	w.printf("| elegiveis | %d |\n", rep.Eligible)
	w.printf("| funcoes no universo | %d |\n", len(rep.Entries))
	w.printf("| anotacoes //log:exempt | %d |\n\n", rep.Exempt)

	w.printf("## Por pacote\n\n")
	w.printf("| pacote | elegiveis | cobertos | func%% | caminhos | cobertos | errpath%% |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, name := range sortedPkgNames(stats) {
		s := stats[name]
		w.printf("| %s | %d | %d | %.1f%% | %d | %d | %.1f%% |\n",
			name, s.eligible, s.covered, pct(s.covered, s.eligible), s.paths, s.pathsCovered, pct(s.pathsCovered, s.paths))
	}

	w.printf("\n## Backlog nominal — funcoes elegiveis sem log (L1 ou L3)\n\n")
	emitBudgetList(w, rep, stats, func(e entry) bool { return !e.covered() })
	w.printf("\n## Backlog nominal — caminhos de saida descobertos (L2)\n\n")
	emitBudgetPaths(w, rep, stats)
	return w.flush()
}

func emitBudgetList(w *lineWriter, rep *report, stats map[string]*pkgStat, keep func(entry) bool) {
	for _, name := range sortedPkgNames(stats) {
		var lines []string
		for _, e := range rep.Entries {
			if e.Excluded != "" || e.PkgRel != name || !keep(e) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- `%s` — %s:%d — %s", e.Key, e.FileRel, e.Line, e.status()))
		}
		if len(lines) == 0 {
			continue
		}
		w.printf("### %s (%d)\n\n%s\n\n", name, len(lines), strings.Join(lines, "\n"))
	}
}

func emitBudgetPaths(w *lineWriter, rep *report, stats map[string]*pkgStat) {
	for _, name := range sortedPkgNames(stats) {
		var lines []string
		for _, e := range rep.Entries {
			if e.Excluded != "" || e.PkgRel != name {
				continue
			}
			for _, p := range e.Paths {
				if p.covered {
					continue
				}
				lines = append(lines, fmt.Sprintf("- `%s` — %s — %s:%d", e.Key, p.kind, e.FileRel, p.pos.Line))
			}
		}
		if len(lines) == 0 {
			continue
		}
		w.printf("### %s (%d)\n\n%s\n\n", name, len(lines), strings.Join(lines, "\n"))
	}
}

// ---------------------------------------------------------------------------
// Relatorio de cobertura de testes com DEDUP DE BLOCOS (§0.3a do plano)
// ---------------------------------------------------------------------------
//
// Com -coverpkg=./... o mesmo bloco aparece uma vez por pacote de teste que o
// executou. Somar as linhas do perfil cru infla o denominador ~13x. A chave de
// deduplicacao e' `arquivo:range`, e a contagem retida e' o max.

type coverBlock struct {
	numStmt int
	count   int
}

// parseCoverProfile le um perfil de cobertura deduplicando blocos.
func parseCoverProfile(r io.Reader) (map[string]map[string]coverBlock, error) {
	out := map[string]map[string]coverBlock{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<22)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first && strings.HasPrefix(line, "mode:") {
			first = false
			continue
		}
		first = false
		file, key, blk, ok := parseCoverLine(line)
		if !ok {
			return nil, fmt.Errorf("linha de perfil invalida: %q", line)
		}
		if out[file] == nil {
			out[file] = map[string]coverBlock{}
		}
		if prev, seen := out[file][key]; seen && prev.count > blk.count {
			blk.count = prev.count
		}
		out[file][key] = blk
	}
	return out, sc.Err()
}

func parseCoverLine(line string) (file, key string, blk coverBlock, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", "", blk, false
	}
	colon := strings.LastIndex(fields[0], ":")
	if colon < 0 {
		return "", "", blk, false
	}
	n, err1 := strconv.Atoi(fields[1])
	c, err2 := strconv.Atoi(fields[2])
	if err1 != nil || err2 != nil {
		return "", "", blk, false
	}
	return fields[0][:colon], fields[0], coverBlock{numStmt: n, count: c}, true
}

// blockLine devolve a linha inicial de um bloco a partir da chave do perfil.
func blockLine(key string) int {
	colon := strings.LastIndex(key, ":")
	if colon < 0 {
		return 0
	}
	dot := strings.Index(key[colon+1:], ".")
	if dot < 0 {
		return 0
	}
	n, err := strconv.Atoi(key[colon+1 : colon+1+dot])
	if err != nil {
		return 0
	}
	return n
}

// restrictToFuncDecls descarta os blocos que nao caem dentro de nenhum
// *ast.FuncDecl — exatamente o recorte que `go tool cover -func` faz, e a
// unica razao pela qual os dois totais poderiam divergir. Blocos de literais
// atribuidos a variaveis de pacote sao os que caem fora.
func restrictToFuncDecls(root string, blocks map[string]map[string]coverBlock) map[string]map[string]coverBlock {
	out := make(map[string]map[string]coverBlock, len(blocks))
	for file, byKey := range blocks {
		ranges, err := funcDeclRanges(root, file)
		if err != nil {
			out[file] = byKey
			continue
		}
		kept := map[string]coverBlock{}
		for key, blk := range byKey {
			line := blockLine(key)
			for _, r := range ranges {
				if line >= r[0] && line <= r[1] {
					kept[key] = blk
					break
				}
			}
		}
		if len(kept) > 0 {
			out[file] = kept
		}
	}
	return out
}

// funcDeclRanges devolve as faixas de linhas das declaracoes de funcao de um
// arquivo do perfil (caminho no formato <modulo>/<caminho>).
func funcDeclRanges(root, profilePath string) ([][2]int, error) {
	rel := profilePath
	if i := strings.Index(rel, "/"); i >= 0 {
		rel = rel[i+1:]
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, abs, nil, 0)
	if err != nil {
		return nil, err
	}
	var out [][2]int
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		out = append(out, [2]int{fset.Position(fd.Pos()).Line, fset.Position(fd.End()).Line})
	}
	return out, nil
}

// writeCoverageReport imprime cobertura por pacote e o total agregado.
func writeCoverageReport(path string, out io.Writer) error {
	f, err := os.Open(path) //nolint:gosec // caminho vem da linha de comando
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	blocks, err := parseCoverProfile(f)
	if err != nil {
		return err
	}
	root, err := findModuleRoot()
	if err != nil {
		return err
	}
	perPkg, total, covered := aggregateCoverage(restrictToFuncDecls(root, blocks))
	w := newLineWriter(out)
	w.printf("%-60s %10s %10s %8s\n", "PACOTE", "STMTS", "COBERTOS", "%")
	names := make([]string, 0, len(perPkg))
	for k := range perPkg {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		s := perPkg[n]
		w.printf("%-60s %10d %10d %7.1f%%\n", n, s.total, s.covered, pct(s.covered, s.total))
	}
	w.printf("total:\t\t\t\t\t\t\t(statements)\t\t%.1f%%\n", pct(covered, total))
	return w.flush()
}

type covStat struct{ total, covered int }

// aggregateCoverage soma os blocos ja deduplicados por pacote.
func aggregateCoverage(blocks map[string]map[string]coverBlock) (map[string]*covStat, int, int) {
	perPkg := map[string]*covStat{}
	total, covered := 0, 0
	for file, byKey := range blocks {
		pkg := filepath.Dir(file)
		s := perPkg[pkg]
		if s == nil {
			s = &covStat{}
			perPkg[pkg] = s
		}
		for _, b := range byKey {
			s.total += b.numStmt
			total += b.numStmt
			if b.count > 0 {
				s.covered += b.numStmt
				covered += b.numStmt
			}
		}
	}
	return perPkg, total, covered
}
