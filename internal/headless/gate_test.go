package headless

// Module-wide gates for the foundation.
//
// They live in the facade package because they are statements about the TREE,
// not about any one layer, and because the rule that produced them is written
// at the top of main.go: a rule that does not fail the build is a comment, not
// a rule. internal/noise/ learned that the expensive way and now has
// scripts/waclient-facade-check.sh; this module gets its version before the
// code arrives rather than after.
//
// Each gate below closes a defect that has already been paid for once, with the
// evidence named beside it.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	moduleRoot     = "."
	chromedpImport = "github.com/chromedp/chromedp"
	// engineDir is the ONLY directory allowed to import the driver.
	engineDir = "engine"
)

// goFiles walks the module's production sources.
func goFiles(t *testing.T, includeTests bool) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}
	return out
}

// ADR-0006 D1 chose chromedp on the explicit grounds that the choice stays
// REVERSIBLE — D3 may force a re-evaluation if detection mitigation needs an
// ecosystem chromedp lacks. A decision that may be revisited has to be isolated
// when it is made, not when it is revisited, and "isolated by convention" is
// how a dependency ends up spread across a tree.
func TestOnlyTheEngineImportsTheDriver(t *testing.T) {
	var offenders []string

	for _, path := range goFiles(t, true) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(p, chromedpImport) && !strings.HasPrefix(p, "github.com/chromedp/") {
				continue
			}
			if dir := filepath.Base(filepath.Dir(path)); dir == engineDir {
				continue
			}
			offenders = append(offenders, path+" imports "+p)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("the driver escaped engine/ (ADR-0006 D1 keeps that choice "+
			"reversible, and a dependency spread across the tree is not "+
			"reversible):\n  %s", strings.Join(offenders, "\n  "))
	}
}

// pollingHelpers are waits whose CLOCK LIVES IN THE PAGE.
//
// chromedp.Poll's WithPollingTimeout is a timer inside the page: a page that
// does not execute JavaScript never fires its own timeout, so the call hangs
// indefinitely — outside the DeadlinePolicy without looking like it. Phase 6
// spent 24 minutes there, and phase 4C hit the same family through
// requestAnimationFrame, which does not fire in a backgrounded headless target
// at all, so the predicate was never evaluated once.
//
// ARMADILHAS entry 19 and section 7 of the Definition of Done: no wait in this
// module may keep its clock in the page.
var pollingHelpers = map[string]bool{
	"Poll":            true,
	"PollFunction":    true,
	"WaitStableRAF":   true,
	"WaitRepaint":     true,
	"WithPollingTime": true,
}

func TestNoWaitKeepsItsClockInThePage(t *testing.T) {
	var offenders []string

	for _, path := range goFiles(t, false) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := sel.Sel.Name
			if !pollingHelpers[name] && !strings.HasPrefix(name, "WithPolling") {
				return true
			}
			offenders = append(offenders, fset.Position(call.Pos()).String()+": "+name)
			return true
		})
	}

	if len(offenders) > 0 {
		t.Fatalf("a wait was implemented with its clock inside the page. A page "+
			"that stops executing JavaScript never fires it, and the wait leaves "+
			"the DeadlinePolicy without looking like it (ARMADILHAS 19). Use a "+
			"Go-side loop under Runner.Do:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// PrimeTab must never run inside Runner.Do.
//
// Do cancels its bounded child when it returns; a target created under that
// child dies with it, and every later operation on the tab fails instantly with
// "context canceled" — which reads like a dead browser and is a dead context.
// This is the one rule about PrimeTab that a single call cannot demonstrate, so
// it is checked here.
func TestPrimingNeverHappensInsideABoundedOperation(t *testing.T) {
	var offenders []string

	for _, path := range goFiles(t, true) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Do" {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				offenders = append(offenders, callsTo(fset, lit, "PrimeTab")...)
			}
			return true
		})
	}

	if len(offenders) > 0 {
		t.Fatalf("PrimeTab was called inside a bounded operation. The target it "+
			"creates dies when Do returns, and every later operation on that tab "+
			"fails with \"context canceled\" instead of a deadline:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func calleeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

// callsTo lists the positions where name is called anywhere inside n.
func callsTo(fset *token.FileSet, n ast.Node, name string) []string {
	var found []string
	ast.Inspect(n, func(inner ast.Node) bool {
		c, ok := inner.(*ast.CallExpr)
		if !ok {
			return true
		}
		if calleeName(c.Fun) == name {
			found = append(found, fset.Position(c.Pos()).String())
		}
		return true
	})
	return found
}

// spaDir is the package that translates Meta's objects into ours.
const spaDir = "spa"

// untypedFieldTypes are the shapes an SPA object takes when it is passed
// through instead of translated.
var untypedFieldTypes = map[string]bool{
	"any":                    true,
	"interface{}":            true,
	"json.RawMessage":        true,
	"map[string]any":         true,
	"map[string]interface{}": true,
}

// The data rule of this initiative, as a gate rather than as a paragraph.
//
// NEVER: `type Foo = <the object the SPA returned>`. The translation has to be
//
//	META INTERNAL OBJECT -> small internal DTO -> HEADLESS DOMAIN
//
// so that a change on Meta's side is confined to one package. The failure mode
// is not usually someone writing that alias on purpose — it is an untyped blob
// escaping: a field typed `any`, `map[string]any` or `json.RawMessage` on an
// EXPORTED type, which hands the caller Meta's shape with none of Meta's
// guarantees and no place to notice when it changes.
//
// Unexported types are fine: they cannot cross the boundary. Errors and
// functions are fine. The rule is about what spa/ PUBLISHES.
//
// Written now, while the boundary is still clean, for the same reason the other
// gates were: a rule that arrives after the code it governs arrives too late.
func TestTheSPABoundaryPublishesNoUntypedObjects(t *testing.T) {
	var offenders []string

	for _, path := range goFiles(t, false) {
		if filepath.Base(filepath.Dir(path)) != spaDir {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || !spec.Name.IsExported() {
				return true
			}
			offenders = append(offenders, untypedFieldsOf(fset, spec)...)
			return true
		})
	}

	if len(offenders) > 0 {
		t.Fatalf("an exported type in %s/ publishes an untyped object. That hands the "+
			"caller Meta's shape with none of Meta's guarantees, and there is then no "+
			"single place that notices when Meta changes it. Translate into a named "+
			"type instead:\n  %s", spaDir, strings.Join(offenders, "\n  "))
	}
}

func fieldName(f *ast.Field) string {
	if len(f.Names) == 0 {
		return "<embedded>"
	}
	return f.Names[0].Name
}

// typeExprString renders the shapes this gate cares about. Anything it does not
// recognise comes back as "" and is therefore not flagged — the gate is narrow
// on purpose: a false positive here would push someone to route around it.
func typeExprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.InterfaceType:
		if v.Methods == nil || len(v.Methods.List) == 0 {
			return "interface{}"
		}
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok {
			return pkg.Name + "." + v.Sel.Name
		}
	case *ast.MapType:
		return "map[" + typeExprString(v.Key) + "]" + typeExprString(v.Value)
	}
	return ""
}

// untypedFieldsOf lists the exported fields of an exported struct whose type is
// an untyped passthrough.
func untypedFieldsOf(fset *token.FileSet, spec *ast.TypeSpec) []string {
	st, ok := spec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	var found []string
	for _, field := range st.Fields.List {
		// An unexported field cannot be read by a caller, so it cannot carry
		// Meta's shape across the boundary.
		if len(field.Names) > 0 && !field.Names[0].IsExported() {
			continue
		}
		name := typeExprString(field.Type)
		if !untypedFieldTypes[name] {
			continue
		}
		found = append(found, fmt.Sprintf("%s: %s.%s is %s",
			fset.Position(field.Pos()), spec.Name.Name, fieldName(field), name))
	}
	return found
}

// pageTextReaders are the ways a script can pull PAGE CONTENT into this
// process.
//
// They are named here rather than checked ad hoc because that is the whole
// point of this gate: the H6 finding was a text capture guarded by ONE
// selector, and its fix removed the capture entirely. What the fix did not do
// is stop the NEXT reader from being written — every capability that reads the
// page today has its own test, and a capability added tomorrow would have none.
var pageTextReaders = []string{
	".innerText",
	".textContent",
	".innerHTML",
	"__x_body",
}

// countingSuffixes are reads that produce a NUMBER rather than content, and are
// therefore allowed.
//
// The distinction is not a loophole, it is the actual rule. `innerText.length`
// tells you how much text a page is showing; `innerText` hands you the text. On
// a paired account that text begins with the chat list — contact names and
// message previews — and the number does not. spa.PageSnapshot.TextLength is
// built on exactly this, and it is what lets the classifier tell a loading page
// from a rendered one without ever holding a word of it.
//
// A first, cruder version of this gate flagged that line, which is how the
// distinction came to be written down instead of assumed.
var countingSuffixes = []string{".length"}

// allowedTextReaderFiles are files permitted to mention those readers in a
// STRING, with the reason.
//
// An allow-list rather than a directory exemption: a whole directory going
// quiet is how a guard rots. Every entry is a decision someone made once, and
// adding to it should feel like the change it is.
var allowedTextReaderFiles = map[string]string{
	// markerScript reads body text INTO A PAGE VARIABLE and returns only which
	// of a closed list of known markers matched — never the text. That is the
	// H6 fix itself, option 2 of the corrections its entry proposed: redact at
	// the boundary so the field carries matches instead of prose.
	"spa/probe.go": "markerScript matches a closed marker set; the text never leaves the page",
}

// exemptionGuards names the tests that make each exemption safe.
//
// An exemption BY FILE is broader than the thing being excused: it would let
// any future .innerText into spa/probe.go unnoticed, which is how an
// allow-list turns into a hole. Tying it to the tests that constrain the
// excused code means the exemption DECAYS if its justification is deleted —
// remove the guard and the gate stops accepting the file.
var exemptionGuards = map[string][]string{
	"spa/probe.go": {
		"TestProbeDiscardsMarkersItNeverAskedAbout",
		"TestMarkerScriptNeverReturnsPageText",
	},
}

// TestNoProductionCodeReadsPageText is invariant 12 and §C6 enforced across the
// whole module instead of capability by capability.
//
// It scans STRING LITERALS ONLY, via the AST. Two reasons, both learned by
// getting it wrong first: page reading happens inside JavaScript, which lives
// in Go strings, so that is where the risk actually is; and a comment that
// NAMES the hazard is the opposite of the hazard — the first version of this
// test flagged spa/page.go's own explanation of H6, which would have taught
// people to stop documenting it.
//
// Scoped to production files. Tests read page text on purpose: the shape probes
// in realspa_test.go exist to measure what a page exposes, and they are the
// reason this module knows what to keep out.
func TestNoProductionCodeReadsPageText(t *testing.T) {
	var offenders []string

	for _, path := range goFiles(t, false) {
		rel := strings.TrimPrefix(filepath.ToSlash(path), "./")
		var allowed bool
		for suffix := range allowedTextReaderFiles {
			if strings.HasSuffix(rel, suffix) {
				allowed = true
				break
			}
		}
		if allowed {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text := lit.Value
			// Remove the counting forms before looking for the content forms,
			// so `innerText.length` does not read as `innerText`.
			for _, reader := range pageTextReaders {
				for _, suffix := range countingSuffixes {
					text = strings.ReplaceAll(text, reader+suffix, "")
				}
			}
			for _, reader := range pageTextReaders {
				if strings.Contains(text, reader) {
					offenders = append(offenders, fmt.Sprintf("%s:%d has %q inside a string",
						rel, fset.Position(lit.Pos()).Line, reader))
				}
			}
			return true
		})
	}

	// An exemption is only as good as the test that bounds it.
	for file, guards := range exemptionGuards {
		for _, guard := range guards {
			if !testExistsInModule(t, guard) {
				t.Errorf("%s is exempt from the page-text gate because %q bounds it, and "+
					"that test no longer exists. The exemption is now unbounded: either "+
					"restore the guard or remove the exemption",
					file, guard)
			}
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("production code reads PAGE CONTENT, which invariant 12 and §C6 forbid "+
			"(WaMessageMeta carries no body, and a raw waJid in a log is a blocker). H6 was "+
			"exactly this: a capture guarded by one selector, against a paired account whose "+
			"body text is the chat list — contact names and message previews. If a reader is "+
			"genuinely needed, add the file to allowedTextReaderFiles with the reason, so the "+
			"decision is visible instead of implied:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// testExistsInModule reports whether a test function of that name is declared
// anywhere in the module's test sources.
func testExistsInModule(t *testing.T, name string) bool {
	t.Helper()
	needle := "func " + name + "("
	for _, path := range goFiles(t, true) {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(body), needle) {
			return true
		}
	}
	return false
}

// housekeepPath is the module's incidental-findings log.
const housekeepPath = "HOUSEKEEP.md"

// openStatusTokens and closedStatusTokens are how an entry's status line is
// classified. Both lists come from MEASURING the file, not from deciding what
// people should have written: the vocabulary already in use is what a reader
// has to cope with.
var (
	openStatusTokens = []string{
		"não corrigido", "nao corrigido", "aberto",
		"guarda presente e não testada", "não verificado",
		// PARTIAL counts as open: it means work remains, and classifying it as
		// closed is exactly the reading that made H5 look finished when its
		// item 3 was still a human decision.
		"parcialmente corrigido", "parcialmente corrigida",
		// PARCIALMENTE ENTREGUE conta como ABERTO pelo mesmo motivo, e entrou
		// deliberadamente: uma capacidade cuja metade não pôde ser PROVADA não
		// está fechada, mesmo que o código exista. Classificá-la como entregue
		// faria uma varredura contar como pronto algo que ninguém verificou —
		// que é precisamente a confusão que este gate impede.
		"parcialmente entregue", "parcialmente entregues",
		// NÃO ENTREGUE conta como ABERTO, e é distinta de "parcialmente
		// entregue" de propósito: aquela diz que metade foi PROVADA, esta diz
		// que a capacidade não funciona de ponta a ponta, mesmo que exista
		// código e testes cobrindo o que foi medido. Colapsar as duas faria uma
		// varredura ler "tem código" como "tem função" — que é a distinção que
		// custou a H57 e a H58. Entrou deliberadamente.
		"não entregue", "nao entregue", "não entregues", "nao entregues",
		// ABANDONADO é um desfecho legítimo e conta como ABERTO: o achado não
		// foi resolvido, foi deixado de lado com o motivo escrito. Classificá-lo
		// como fechado apagaria a diferença entre "resolvido" e "decidi não
		// resolver", que é exatamente a distinção que o registro existe para
		// preservar.
		"abandonado", "abandonada",
		// MEDIDO conta como ABERTO: significa que a medição existe e a
		// CONSEQUÊNCIA dela ainda não foi aplicada. Classificá-lo como fechado
		// deixaria um achado cuja ação pendente ninguém veria — que é a forma
		// exata do problema que o H29 registra.
		"medido", "medida",
	}
	closedStatusTokens = []string{
		"corrigido", "corrigida", "fechado", "fechada",
		"verificado", "confirmado", "decidido",
		// ENTREGUE conta como FECHADO, e é palavra distinta de "corrigido" de
		// propósito: uma capacidade NOVA não conserta nada, e escrever
		// "corrigido" nela diria que havia defeito onde havia ausência. Entrou
		// deliberadamente, que é o que este gate exige de qualquer palavra
		// nova — a alternativa seria torcer a entrada para caber no
		// vocabulário, e aí o registro passa a mentir para agradar a ferramenta.
		"entregue", "entregues",
	}
)

// strikethrough matches a ~~...~~ span, which in this file means SUPERSEDED
// text kept for history.
var strikethrough = regexp.MustCompile(`(?s)~~.*?~~`)

// TestHousekeepEntriesAreMachineReadable makes the findings log readable by a
// tool, because reading it by eye produced three wrong reports in one day.
//
// THE DEFECT THIS EXISTS FOR. Statuses in this file are superseded by striking
// the old one through and writing the new one after it — good for a human, and
// a trap for a scan: a naive grep finds the STRUCK text first and reports a
// closed finding as open. On 2026-08-19 that made H2, H5 and H6 read as open
// when all three were closed, and two of them were reported that way before the
// mistake was caught by actually opening the entries.
//
// So this test enforces the two properties that make a scan trustworthy:
// every entry HAS an authoritative status, and every authoritative status
// begins with a word from the vocabulary the file already uses. It does not
// try to be clever about entries carrying several statuses — those get listed
// for a human to read, which is honest about what a tool can settle.
// ANCHORED AT THE START OF A LINE, AND FORBIDDEN TO CROSS ONE. Both halves
// were paid for: the earlier pattern matched the word "Status" wherever it
// appeared — including inside receiveTextStatusEnabled and getTextStatus —
// and its [^*:]* ran through newlines until some distant colon, so it
// reported a fragment of test output three paragraphs away as an entry's
// status. That happened twice in one day, and the workaround was to wrap
// identifiers in emphasis so the "*" would stop the scan, which taxed
// everyone who wrote "Status" in an entry.
//
//	^[ \t>*·-]*   a status line may be indented, quoted or bulleted
//	\*{0,2}Status  and bolded, which is how every real one is written
//	[^*:\n]*       but the label cannot run onto another line
//
// The separator class after the colon is consumed BEFORE capturing, not
// trimmed after: stripping a struck-through status leaves "· " or "— "
// behind, and a capture that stops at the next "*" would grab only the
// separator and report it as the status.
var housekeepStatusLine = regexp.MustCompile(`(?m)^[ \t>*·-]*\*{0,2}Status[^*:\n]*\*{0,2}:[\s*·—–-]*([^\n*]{3,60})`)

func TestHousekeepEntriesAreMachineReadable(t *testing.T) {
	body, err := os.ReadFile(housekeepPath)
	if err != nil {
		t.Fatalf("read %s: %v", housekeepPath, err)
	}

	entryHeading := regexp.MustCompile(`(?m)^## (H\d+)\b`)
	statusLine := housekeepStatusLine

	src := string(body)
	locs := entryHeading.FindAllStringSubmatchIndex(src, -1)
	if len(locs) == 0 {
		t.Fatalf("%s has no ## H entries; this test is guarding nothing", housekeepPath)
	}

	var missing, unknown, multi []string
	for i, loc := range locs {
		name := src[loc[2]:loc[3]]
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		// Authoritative text only: superseded statuses are struck through, and
		// reading them is precisely the bug.
		entry := strikethrough.ReplaceAllString(src[loc[0]:end], "")

		found := statusLine.FindAllStringSubmatch(entry, -1)
		if len(found) == 0 {
			missing = append(missing, name)
			continue
		}
		if len(found) > 1 {
			multi = append(multi, fmt.Sprintf("%s (%d statuses)", name, len(found)))
		}
		for _, m := range found {
			// Stripping a struck-through status leaves its separator behind —
			// "**Status**: ~~aberto~~ · **DECIDIDO**" becomes "**Status**: ·
			// **DECIDIDO**". Trimming the leftovers is what lets the surviving
			// status be read instead of the punctuation in front of it.
			s := strings.ToLower(strings.Trim(strings.TrimSpace(m[1]), " ·*—-–:"))
			var known bool
			for _, tok := range append(append([]string{}, openStatusTokens...), closedStatusTokens...) {
				if strings.HasPrefix(s, tok) {
					known = true
					break
				}
			}
			if !known {
				unknown = append(unknown, fmt.Sprintf("%s: %q", name, m[1]))
			}
		}
	}

	if len(missing) > 0 {
		t.Errorf("these entries carry NO authoritative status, so nothing can tell whether "+
			"they are open — H22 was written that way and went unnoticed: %s",
			strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		t.Errorf("these statuses start with a word outside the vocabulary the file already "+
			"uses, so a scan cannot classify them. Either use an existing word or add the "+
			"new one to openStatusTokens/closedStatusTokens deliberately:\n  %s",
			strings.Join(unknown, "\n  "))
	}
	if len(multi) > 0 {
		// NOT a failure: an entry resolved in stages legitimately carries a
		// status per stage (H5 has one for the whole finding and one for its
		// item 3). Logged so a reader knows which entries a tool cannot settle
		// on its own.
		t.Logf("entries with several authoritative statuses — read these by hand: %s",
			strings.Join(multi, ", "))
	}
}
