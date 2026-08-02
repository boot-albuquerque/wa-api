package main

// analyzer.go — carga type-aware dos pacotes, construcao do universo de
// funcoes e aplicacao das regras da secao 9.1 (cmd/logcov/METRIC.md).

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const loggerPortPath = "/pkg/application/contracts"

// analysis carrega o estado global de uma execucao.
type analysis struct {
	fset         *token.FileSet
	modulePath   string
	root         string
	excludes     []string // prefixos module-relative de .logcov-exclude
	depguardDirs []string // diretorios da regra depguard/camada-de-aplicacao

	loggerIface        *types.Interface
	errorIface         *types.Interface
	ctxIface           *types.Interface
	httpHandler        *types.Interface
	httpResponseWriter *types.Interface
	httpRequestPtr     types.Type
	zerologLogger      types.Type
}

func (a *analysis) implements(t types.Type, iface *types.Interface) bool {
	if t == nil || iface == nil {
		return false
	}
	return types.Implements(t, iface) || types.Implements(types.NewPointer(t), iface)
}

// pkgCtx carrega o estado por pacote.
type pkgCtx struct {
	a      *analysis
	pkg    *packages.Package
	info   *types.Info
	parent map[ast.Node]ast.Node
	block  map[ast.Node]*ast.BlockStmt
}

// entry e' uma unidade do universo: um *ast.FuncDecl com corpo, ou um
// *ast.FuncLit (elegivel por X6 ou excluido por ele).
type entry struct {
	Key      string
	PkgRel   string
	FileRel  string
	FileBase string
	Line     int
	Excluded string // "" quando elegivel; senao "X1".."X8"
	Detail   string // motivo textual da exclusao (X8)
	L1       bool
	L3       bool
	Sites    []logSite
	Paths    []exitPath
}

func (e entry) covered() bool { return e.L1 && e.L3 }

// status monta a coluna <detalhe> do golden.
func (e entry) status() string {
	if e.Excluded != "" {
		if e.Detail != "" {
			return e.Excluded + ":" + e.Detail
		}
		return e.Excluded
	}
	var issues []string
	switch {
	case !e.L1:
		issues = append(issues, "uncovered:L1")
	case !e.L3:
		for _, s := range e.Sites {
			if !s.valid {
				issues = append(issues, fmt.Sprintf("uncovered:L3(%s,%s:%d)", s.form, filepath.Base(s.pos.Filename), s.pos.Line))
			}
		}
	}
	for _, p := range e.Paths {
		if !p.covered {
			issues = append(issues, fmt.Sprintf("uncovered:L2(%s,%s:%d)", p.kind, filepath.Base(p.pos.Filename), p.pos.Line))
		}
	}
	if len(issues) == 0 {
		return "covered"
	}
	return strings.Join(issues, ";")
}

// report e' o resultado agregado.
type report struct {
	Entries      []entry
	Eligible     int
	Covered      int
	Paths        int
	PathsCovered int
	Exempt       int
	Forms        map[string]*formStat
}

type formStat struct {
	CallSites int
	files     map[string]bool
}

func (f *formStat) Files() int { return len(f.files) }

// ---------------------------------------------------------------------------
// Configuracao lida do repositorio
// ---------------------------------------------------------------------------

// readExcludeFile le .logcov-exclude. Linhas vazias e comentarios sao
// ignoradas; cada item e' um prefixo de caminho module-relative.
func readExcludeFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasSuffix(line, "/") {
			line += "/"
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// readDepguardDirs extrai os diretorios da regra depguard/camada-de-aplicacao
// do .golangci.yml. O acoplamento e' deliberado: a restricao de forma L1-b
// vale exatamente onde o lint proibe zerolog, e as duas listas nao podem
// divergir silenciosamente.
func readDepguardDirs(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caminho vem do repositorio
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	inRule, inFiles := false, false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "camada-de-aplicacao:" {
			inRule = true
			continue
		}
		if !inRule {
			continue
		}
		if trimmed == "files:" {
			inFiles = true
			continue
		}
		if !inFiles {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		item := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
		item = strings.TrimPrefix(item, "**/")
		item = strings.TrimSuffix(item, "/*.go")
		item = strings.TrimSuffix(item, "/**")
		dirs = append(dirs, item)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// countExemptAnnotations conta os //log:exempt sob um diretorio, ignorando
// arquivos de teste. E' a mesma contagem do criterio de pronto via grep.
func countExemptAnnotations(root string) (int, error) {
	n := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // caminho vem do walk
		if err != nil {
			return err
		}
		n += strings.Count(string(data), "//log:exempt")
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return n, err
}

// ---------------------------------------------------------------------------
// Carga
// ---------------------------------------------------------------------------

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
	packages.NeedSyntax | packages.NeedTypesInfo

func newAnalysis(root, modulePath string, excludes, depguardDirs []string) *analysis {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &analysis{
		fset:         token.NewFileSet(),
		modulePath:   modulePath,
		root:         root,
		excludes:     excludes,
		depguardDirs: depguardDirs,
	}
}

// load carrega os pacotes do padrao dado e resolve os tipos de referencia.
func (a *analysis) load(patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  loadMode,
		Dir:   a.root,
		Fset:  a.fset,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("erro ao carregar pacotes:\n%s", strings.Join(errs, "\n"))
	}
	a.resolveReferenceTypes(pkgs)
	return pkgs, nil
}

// resolveReferenceTypes acha, no grafo de dependencias carregado, os tipos
// contra os quais as regras resolvem. Sem eles a metrica seria por nome.
func (a *analysis) resolveReferenceTypes(pkgs []*packages.Package) {
	a.errorIface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types == nil {
			return
		}
		switch {
		case strings.HasSuffix(p.PkgPath, loggerPortPath):
			a.loggerIface = lookupInterface(p, "Logger")
		case p.PkgPath == "context":
			a.ctxIface = lookupInterface(p, "Context")
		case p.PkgPath == "net/http":
			a.resolveNetHTTP(p)
		case p.PkgPath == "github.com/rs/zerolog":
			if o := p.Types.Scope().Lookup("Logger"); o != nil {
				a.zerologLogger = o.Type()
			}
		}
	})
}

func (a *analysis) resolveNetHTTP(p *packages.Package) {
	a.httpHandler = lookupInterface(p, "Handler")
	a.httpResponseWriter = lookupInterface(p, "ResponseWriter")
	if o := p.Types.Scope().Lookup("Request"); o != nil {
		a.httpRequestPtr = types.NewPointer(o.Type())
	}
}

// lookupInterface acha uma interface nomeada no escopo de um pacote.
func lookupInterface(p *packages.Package, name string) *types.Interface {
	o := p.Types.Scope().Lookup(name)
	if o == nil {
		return nil
	}
	i, _ := o.Type().Underlying().(*types.Interface)
	return i
}

// ---------------------------------------------------------------------------
// Universo e analise
// ---------------------------------------------------------------------------

// isGenerated — arquivo com a marca canonica de codigo gerado.
func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "// Code generated ") && strings.HasSuffix(c.Text, "DO NOT EDIT.") {
				return true
			}
		}
	}
	return false
}

// Analyze roda a metrica inteira sobre os pacotes carregados.
func (a *analysis) Analyze(pkgs []*packages.Package) *report {
	rep := &report{Forms: map[string]*formStat{
		"port":    {files: map[string]bool{}},
		"zerolog": {files: map[string]bool{}},
		"hlog":    {files: map[string]bool{}},
	}}
	var roots []*packages.Package
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if len(p.Syntax) > 0 && strings.HasPrefix(p.PkgPath, a.modulePath+"/") {
			roots = append(roots, p)
		}
	})
	sort.Slice(roots, func(i, j int) bool { return roots[i].PkgPath < roots[j].PkgPath })

	for _, p := range roots {
		a.analyzePackage(p, rep)
	}
	sort.Slice(rep.Entries, func(i, j int) bool {
		if rep.Entries[i].Key != rep.Entries[j].Key {
			return rep.Entries[i].Key < rep.Entries[j].Key
		}
		return rep.Entries[i].Line < rep.Entries[j].Line
	})
	rep.aggregate()
	return rep
}

// aggregate soma os totais de numerador e denominador das duas metricas.
func (rep *report) aggregate() {
	for _, e := range rep.Entries {
		if e.Excluded != "" {
			continue
		}
		rep.Eligible++
		if e.covered() {
			rep.Covered++
		}
		for _, pa := range e.Paths {
			rep.Paths++
			if pa.covered {
				rep.PathsCovered++
			}
		}
	}
}

func (a *analysis) analyzePackage(p *packages.Package, rep *report) {
	pkgRel := strings.TrimPrefix(p.PkgPath, a.modulePath+"/")
	ctx := &pkgCtx{a: a, pkg: p, info: p.TypesInfo}

	for _, file := range p.Syntax {
		fname := a.fset.Position(file.Pos()).Filename
		if strings.HasSuffix(fname, "_test.go") || isGenerated(file) {
			continue
		}
		fileRel, err := filepath.Rel(a.root, fname)
		if err != nil {
			fileRel = fname
		}
		fileRel = filepath.ToSlash(fileRel)

		ctx.parent = map[ast.Node]ast.Node{}
		ctx.block = map[ast.Node]*ast.BlockStmt{}
		indexFile(ctx, file)

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Body == nil {
					continue
				}
				base := declKey(pkgRel, d)
				rep.Entries = append(rep.Entries, a.analyzeFunc(ctx, pkgRel, fileRel, base, d, d.Body, d.Doc, rep)...)
			case *ast.GenDecl:
				rep.Entries = append(rep.Entries, a.analyzePackageVars(ctx, pkgRel, fileRel, d, rep)...)
			}
		}
	}
}

// analyzePackageVars acha os *ast.FuncLit dentro de declaracoes var de nivel
// de pacote e aplica a MESMA promocao X6 usada para FuncLits dentro de corpos
// de funcao (R1): um FuncLit numa `var` de pacote — por exemplo
// `var x = sync.OnceValue(func() {...})` — e' invisivel a metrica se so' os
// corpos de *ast.FuncDecl forem percorridos. Tratamos cada especificador da
// declaracao como um "corpo proprio" candidato, reusando analyzeFuncLitsIn em
// vez de duplicar a logica de promocao/analise.
func (a *analysis) analyzePackageVars(ctx *pkgCtx, pkgRel, fileRel string, gd *ast.GenDecl, rep *report) []entry {
	if gd.Tok != token.VAR {
		return nil
	}
	var out []entry
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) == 0 {
			continue
		}
		base := pkgRel + "." + vs.Names[0].Name
		subEntries, _ := a.analyzeFuncLitsIn(ctx, pkgRel, fileRel, base, vs, rep)
		out = append(out, subEntries...)
	}
	return out
}

// indexFile constroi os mapas de pai e de menor BlockStmt.
func indexFile(ctx *pkgCtx, file *ast.File) {
	var stack []ast.Node
	var blocks []*ast.BlockStmt
	walk := func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				if _, ok := stack[len(stack)-1].(*ast.BlockStmt); ok {
					blocks = blocks[:len(blocks)-1]
				}
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if len(stack) > 0 {
			ctx.parent[n] = stack[len(stack)-1]
		}
		if len(blocks) > 0 {
			ctx.block[n] = blocks[len(blocks)-1]
		}
		stack = append(stack, n)
		if b, ok := n.(*ast.BlockStmt); ok {
			blocks = append(blocks, b)
		}
		return true
	}
	ast.Inspect(file, walk)
}

func declKey(pkgRel string, fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		return pkgRel + "." + typeExprName(fd.Recv.List[0].Type) + "." + fd.Name.Name
	}
	return pkgRel + "." + fd.Name.Name
}

func typeExprName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return typeExprName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return typeExprName(x.X)
	case *ast.IndexListExpr:
		return typeExprName(x.X)
	}
	return "?"
}

// analyzeFunc produz a entrada da funcao e, recursivamente, as entradas dos
// FuncLits contidos nela.
func (a *analysis) analyzeFunc(ctx *pkgCtx, pkgRel, fileRel, key string, fd *ast.FuncDecl, body *ast.BlockStmt, doc *ast.CommentGroup, rep *report) []entry {
	subEntries, promoted := a.analyzeFuncLitsIn(ctx, pkgRel, fileRel, key, body, rep)
	main := a.analyzeBody(ctx, pkgRel, fileRel, key, fd, body, doc, rep, promoted)
	return append(main, subEntries...)
}

// analyzeFuncLitsIn acha os *ast.FuncLit diretamente sob root — o corpo de
// uma funcao ou, para R1, o *ast.ValueSpec de uma `var` de nivel de pacote —
// e aplica a promocao X6: literais qualificados (go/defer, errgroup.Go,
// sync.OnceFunc/OnceValue, forma http.Handler) viram entradas proprias
// analisadas recursivamente; os demais viram uma entrada excluida por X6.
// Devolve tambem o conjunto de literais promovidos, que o chamador usa para
// recortar o corpo proprio da unidade envolvente (skipPromoted).
func (a *analysis) analyzeFuncLitsIn(ctx *pkgCtx, pkgRel, fileRel, key string, root ast.Node, rep *report) ([]entry, map[*ast.FuncLit]bool) {
	lits := collectFuncLits(ctx, root)

	// Os FuncLits qualificados por X6 sao entradas proprias; seus corpos saem
	// do corpo proprio da unidade que os contem — inclusive quando essa
	// unidade e' outro FuncLit promovido. Sem esse recorte, uma cadeia de
	// closures aninhadas contaria o mesmo call site duas vezes.
	promoted := map[*ast.FuncLit]bool{}
	for _, lit := range lits {
		if ruleX6FuncLitEligible(ctx, lit) {
			promoted[lit] = true
		}
	}

	var subEntries []entry
	for i, lit := range lits {
		litKey := fmt.Sprintf("%s.func%d", key, i+1)
		if !promoted[lit] {
			subEntries = append(subEntries, entry{
				Key: litKey, PkgRel: pkgRel, FileRel: fileRel,
				FileBase: filepath.Base(fileRel),
				Line:     a.fset.Position(lit.Pos()).Line,
				Excluded: "X6",
			})
			continue
		}
		subEntries = append(subEntries, a.analyzeBody(ctx, pkgRel, fileRel, litKey, nil, lit.Body, nil, rep, promoted)...)
	}
	return subEntries, promoted
}

// analyzeBody aplica as regras a um corpo, ignorando os FuncLits promovidos.
func (a *analysis) analyzeBody(ctx *pkgCtx, pkgRel, fileRel, key string, fd *ast.FuncDecl, body *ast.BlockStmt, doc *ast.CommentGroup, rep *report, promoted map[*ast.FuncLit]bool) []entry {
	e := entry{
		Key: key, PkgRel: pkgRel, FileRel: fileRel,
		FileBase: filepath.Base(fileRel),
		Line:     a.fset.Position(body.Pos()).Line,
	}

	sites := a.collectLogSites(ctx, fileRel, body, promoted)
	paths := a.collectExitPaths(ctx, body, promoted)

	e.Excluded, e.Detail = classifyEligibility(a, ctx, pkgRel, fd, body, doc, paths)

	// As chamadas de log contam para by_form mesmo em funcao excluida: a
	// contagem de call sites descreve o repositorio, nao o denominador.
	for _, s := range sites {
		fs := rep.Forms[s.form]
		fs.CallSites++
		fs.files[fileRel] = true
	}

	e.Sites = sites
	if e.Excluded != "" {
		return []entry{e}
	}

	e.L1 = len(sites) > 0
	e.L3 = true
	for i := range e.Sites {
		e.Sites[i].valid = ruleL3Structured(e.Sites[i])
		if !e.Sites[i].valid {
			e.L3 = false
		}
	}
	e.Paths = a.markCoverage(ctx, paths, sites)
	return []entry{e}
}

// classifyEligibility aplica X5, X3, X4, X8, X1, X2 e X7 na precedencia
// declarada em METRIC.md. Devolve o codigo da regra que excluiu a funcao, ou
// string vazia quando ela e' elegivel.
func classifyEligibility(a *analysis, ctx *pkgCtx, pkgRel string, fd *ast.FuncDecl, body *ast.BlockStmt, doc *ast.CommentGroup, paths []exitPath) (string, string) {
	switch {
	case ruleX5PackageExcluded(a, pkgRel):
		return "X5", ""
	case ruleX3Init(fd):
		return "X3", ""
	case ruleX4SelfReferential(ctx, fd):
		return "X4", ""
	}
	if ok, motivo := ruleX8Exempt(doc); ok {
		return "X8", motivo
	}
	switch {
	case ruleX1Trivial(body, paths):
		return "X1", ""
	case ruleX2PureAccessor(ctx, fd, body):
		return "X2", ""
	case ruleX7PureDelegation(ctx, paramNames(fd), body):
		return "X7", ""
	}
	return "", ""
}

func paramNames(fd *ast.FuncDecl) []string {
	if fd == nil || fd.Type.Params == nil {
		return nil
	}
	var out []string
	for _, f := range fd.Type.Params.List {
		if len(f.Names) == 0 {
			out = append(out, "_anon_")
			continue
		}
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// collectFuncLits devolve os FuncLits contidos em root, em ordem de fonte
// (inclui os aninhados, para que todo literal tenha uma linha no golden).
// root e' tipicamente o corpo de uma funcao (*ast.BlockStmt), mas tambem pode
// ser o *ast.ValueSpec de uma `var` de nivel de pacote (R1): ast.Inspect
// percorre qualquer ast.Node.
func collectFuncLits(ctx *pkgCtx, root ast.Node) []*ast.FuncLit {
	var out []*ast.FuncLit
	ast.Inspect(root, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			out = append(out, lit)
		}
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos() < out[j].Pos() })
	return out
}

// skipPromoted diz se a travessia deve parar num FuncLit promovido a entrada
// propria (o corpo dele nao pertence ao corpo proprio da funcao envolvente).
func skipPromoted(n ast.Node, promoted map[*ast.FuncLit]bool) bool {
	lit, ok := n.(*ast.FuncLit)
	return ok && promoted[lit]
}

// collectLogSites acha todas as chamadas de log reconhecidas no corpo proprio.
func (a *analysis) collectLogSites(ctx *pkgCtx, fileRel string, body *ast.BlockStmt, promoted map[*ast.FuncLit]bool) []logSite {
	var out []logSite
	ast.Inspect(body, func(n ast.Node) bool {
		if skipPromoted(n, promoted) {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if s, ok := ruleL1aPort(ctx, call); ok {
			out = append(out, s)
			return true
		}
		if s, ok := ruleL1bcChain(ctx, call); ok {
			// Restricao por diretorio: nos diretorios da regra depguard,
			// L1-b nao e' forma valida.
			if s.form == "zerolog" && !ruleL1bValidHere(a, fileRel) {
				return true
			}
			out = append(out, s)
		}
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].pos.Offset < out[j].pos.Offset })
	return out
}

// collectExitPaths acha S-ret, S-http e S-consume no corpo proprio.
func (a *analysis) collectExitPaths(ctx *pkgCtx, body *ast.BlockStmt, promoted map[*ast.FuncLit]bool) []exitPath {
	var out []exitPath
	add := func(kind string, node ast.Node, blk *ast.BlockStmt, byB bool) {
		out = append(out, exitPath{
			kind: kind, node: node, block: blk, satisfiedByB: byB,
			pos: a.fset.Position(node.Pos()),
		})
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if skipPromoted(n, promoted) {
			return false
		}
		switch x := n.(type) {
		case *ast.ReturnStmt:
			if expr, ok := ruleL2SRet(ctx, x); ok {
				add("S-ret", x, ctx.block[x], ruleL2PropagatesCause(ctx, expr, nil))
			}
		case *ast.CallExpr:
			if ruleL2SHTTP(ctx, x) {
				add("S-http", x, ctx.block[x], false)
			}
		case *ast.IfStmt:
			if _, ok := ruleL2SConsumeIf(ctx, x); ok {
				// O bloco relevante e' o corpo do if: e' ali que o log da
				// forma (a) tem de estar.
				add("S-consume", x.Body, x.Body, false)
			}
		case *ast.SwitchStmt:
			if _, ok := ruleL2SConsumeSwitch(ctx, x); ok {
				add("S-consume", x.Body, x.Body, false)
			}
		}
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].pos.Offset < out[j].pos.Offset })
	return out
}

// markCoverage aplica (a) log Warn/Error no menor bloco que contem o caminho,
// e (b) restrita a S-ret com propagacao de causa sem descarte.
func (a *analysis) markCoverage(ctx *pkgCtx, paths []exitPath, sites []logSite) []exitPath {
	warnBlocks := map[*ast.BlockStmt]bool{}
	for _, s := range sites {
		if logLevelRank[s.level] < logLevelRank["Warn"] {
			continue
		}
		if b := ctx.block[s.node]; b != nil {
			warnBlocks[b] = true
		}
	}
	for i := range paths {
		p := &paths[i]
		if p.block != nil && warnBlocks[p.block] {
			p.covered = true
			continue
		}
		if p.kind == "S-ret" && p.satisfiedByB {
			p.covered = true
		}
	}
	return paths
}

// tenths converte uma fracao em decimos de ponto percentual, a mesma unidade
// inteira de .coverage-baseline (comparacao exata, independente de locale).
func tenths(num, den int) int {
	if den == 0 {
		return 0
	}
	return int(float64(num)*1000.0/float64(den) + 0.5)
}

func pct(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) * 100.0 / float64(den)
}
