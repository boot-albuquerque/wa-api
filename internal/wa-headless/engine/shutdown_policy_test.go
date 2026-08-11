package engine

// Locks requirement 1 of phase 4C: nothing in this module may stop a browser by
// signal outside the one labelled fallback.
//
// The defect this prevents already happened, and it survived an entire phase
// unseen. Phase 4C measured that SIGTERM corrupts session state, wrote the
// requirement into its report, and the requirement stayed inside the experiment
// that measured it. Five modes of the study went on calling gracefulStop, and
// the symptom resurfaced a phase later as Singleton files left in the profile —
// the signature of a dirty exit. It is F94 in the root HOUSEKEEP.md.
//
// So the test is STATIC, not behavioural. The defect was never "the clean
// shutdown does not work" — CloseBrowserViaCDP always worked. The defect is
// "the call site calls the wrong thing", and that is only visible by looking at
// call sites. A behavioural test over CleanStop passes with every call site
// still broken.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// moduleRoot is where the scan starts: the whole of internal/wa-headless, not
// just this package. The rule is about call sites, and call sites will live in
// core/, runtime/ and capabilities/ long before they live here.
const moduleRoot = ".."

// The exemption is PER CALL, marked on the line itself, never by function name.
//
// The study's first version exempted whole functions, and that nearly cost
// three defects: in one recovery function only ONE of four signal calls was the
// variable under ablation — the other three were error-path cleanup over the
// paired profile, and one of them ran precisely when the baseline failed, which
// is when the credential is already fragile.
//
// A name-based exemption is too wide in a second way: it also covers the code
// that has not been written inside that function yet.
const ablationMarker = "ablation:stop-form"

// signalSendingCalls are the shapes of "make this process die by signal".
// SignalStop is this package's own dirty path; the other two are the ways Go
// reaches a process directly.
var signalSendingCalls = map[string]bool{
	"SignalStop": true,
	"Signal":     true,
	"Kill":       true,
}

func TestNothingStopsABrowserBySignal(t *testing.T) {
	var offenders []string

	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		exemptLines := map[int]bool{}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, ablationMarker) {
					exemptLines[fset.Position(c.Pos()).Line] = true
				}
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !signalSendingCalls[sel.Sel.Name] {
				return true
			}
			pos := fset.Position(call.Pos())
			if exemptLines[pos.Line] {
				return true
			}
			offenders = append(offenders, pos.String()+": "+sel.Sel.Name)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}

	if len(offenders) > 0 {
		t.Fatalf("a browser is stopped by signal, and SIGTERM corrupts session state "+
			"(phase 4C requirement 1, F94). Use CleanStop, or mark the line %q "+
			"if the signal is genuinely the point:\n  %s",
			"//"+ablationMarker, strings.Join(offenders, "\n  "))
	}
}

// Counter-proof for the test above. It only means anything if the clean path
// actually exists and actually goes through the protocol: without this, emptying
// CleanStop would leave the suite green with every path stopping by signal
// underneath — which is the exact shape of the failure it is meant to catch.
func TestCleanStopGoesThroughBrowserClose(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "shutdown.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var found bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "CleanStop" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "CloseBrowserViaCDP" {
					found = true
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("CleanStop does not call CloseBrowserViaCDP: the clean shutdown is gone, " +
			"and the static gate above would still pass")
	}
}

// The exemption must stay rare and must stay here. One marked line is the
// design; a module sprinkled with markers is the gate being routed around, and
// that is a review decision, not something to discover later.
func TestTheSignalExemptionIsUsedExactlyOnce(t *testing.T) {
	var marked []string

	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, ablationMarker) {
					marked = append(marked, fset.Position(c.Pos()).String())
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}

	if len(marked) != 1 {
		t.Fatalf("found %d ablation markers, want exactly 1 (the fallback inside "+
			"CleanStop). Every extra one is a path that stops by signal:\n  %s",
			len(marked), strings.Join(marked, "\n  "))
	}
	if !strings.Contains(marked[0], "shutdown.go") {
		t.Fatalf("the only signal exemption is at %s; it belongs in shutdown.go, "+
			"inside CleanStop", marked[0])
	}
}
