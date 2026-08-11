package waheadless

// Module-wide gates for the foundation.
//
// They live in the facade package because they are statements about the TREE,
// not about any one layer, and because the rule that produced them is written
// at the top of main.go: a rule that does not fail the build is a comment, not
// a rule. internal/wa-noise/ learned that the expensive way and now has
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
	"path/filepath"
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
//	META INTERNAL OBJECT -> small internal DTO -> WA-HEADLESS DOMAIN
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
