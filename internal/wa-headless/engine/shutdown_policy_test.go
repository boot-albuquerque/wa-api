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

// deliversNothing reports whether a call sends signal 0.
//
// Signal 0 delivers no signal at all: it asks the kernel whether a pid exists.
// It cannot stop anything, so flagging it would make the gate refuse a LIVENESS
// PROBE, and the only ways out of that would be to exempt a function that is
// not a shutdown (widening the exemption) or to write the probe some other way
// to dodge the check (routing around it). Both are worse than naming the case.
//
// The match is deliberately syntactic and narrow: the literal 0, spelled out at
// the call site. A variable holding a signal is NOT this — its value is not
// visible here, and "probably zero" is not a thing a gate may assume.
func deliversNothing(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	last := call.Args[len(call.Args)-1]

	// syscall.Signal(0)
	if conv, ok := last.(*ast.CallExpr); ok {
		if sel, ok := conv.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Signal" &&
			len(conv.Args) == 1 && isZeroLiteral(conv.Args[0]) {
			return true
		}
	}
	// Kill(pid, 0)
	return isZeroLiteral(last)
}

func isZeroLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "0"
}

func TestNothingStopsABrowserBySignal(t *testing.T) {
	var offenders []string

	eachProductionFile(t, func(path string, fset *token.FileSet, file *ast.File) {
		exempt := exemptedLines(fset, file)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !signalSendingCalls[sel.Sel.Name] || deliversNothing(call) {
				return true
			}
			pos := fset.Position(call.Pos())
			if exempt[pos.Line] {
				return true
			}
			offenders = append(offenders, pos.String()+": "+sel.Sel.Name)
			return true
		})
	})

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

// signalOwningFuncs are the only functions allowed to carry the exemption.
//
// The rule is that the code permitted to signal is the code whose NAME is the
// signal. Two functions qualify, and they are different jobs:
//
//	CleanStop   decides that the protocol path failed and the fallback is due
//	SignalStop  is the fallback — the mechanism that delivers the signal
//
// An earlier version of this gate demanded exactly ONE marker in the module,
// which was written before the mechanism existed and would have forced the
// launcher's SignalStop to either carry a second marker (failing the gate) or
// route around it. Counting markers was a proxy; the real rule is WHERE they
// may appear, and that is what is checked now.
//
// The count still matters, so it is reported: a module that grows a third
// signalling site has made a decision somebody should see.
var signalOwningFuncs = map[string]bool{
	"CleanStop":  true,
	"SignalStop": true,
}

func TestSignalExemptionsLiveOnlyInShutdownFunctions(t *testing.T) {
	type mark struct{ pos, fn string }
	var marks []mark

	// Each marker is attributed to the function whose body contains it. A
	// marker outside any function body reports an empty name and fails below —
	// which is right: a package-level exemption exempts everything.
	eachProductionFile(t, func(path string, fset *token.FileSet, file *ast.File) {
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if !strings.Contains(c.Text, ablationMarker) {
					continue
				}
				marks = append(marks, mark{
					pos: fset.Position(c.Pos()).String(),
					fn:  enclosingFunc(file, c.Pos()),
				})
			}
		}
	})

	if len(marks) == 0 {
		t.Fatal("no ablation marker anywhere: the dirty fallback vanished, and with it " +
			"the only sanctioned way to stop a browser that refuses to leave")
	}

	var offenders []string
	for _, m := range marks {
		if !signalOwningFuncs[m.fn] {
			where := m.fn
			if where == "" {
				where = "no enclosing function"
			}
			offenders = append(offenders, m.pos+" in "+where)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a signal exemption sits outside CleanStop/SignalStop. Only the code whose "+
			"NAME is the signal may carry one; anywhere else it is the gate being routed "+
			"around:\n  %s", strings.Join(offenders, "\n  "))
	}
	t.Logf("%d signal exemption(s), all inside CleanStop/SignalStop", len(marks))
}

// eachProductionFile parses every non-test .go file under the module and hands
// it to fn.
//
// Shared by both gates below because they ask different questions of the SAME
// set of files: a walk written twice is a filter that can drift, and a gate
// that silently stops covering a directory is a gate that passes for the wrong
// reason.
func eachProductionFile(t *testing.T, fn func(path string, fset *token.FileSet, file *ast.File)) {
	t.Helper()

	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		fn(path, fset, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}
}

// exemptedLines maps the lines carrying the ablation marker in one file.
func exemptedLines(fset *token.FileSet, file *ast.File) map[int]bool {
	out := map[int]bool{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, ablationMarker) {
				out[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	return out
}

// enclosingFunc names the function whose body contains pos, or "" when none
// does. A marker outside any body exempts everything in the file, so "" is a
// failure, not a detail.
func enclosingFunc(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Body.Pos() <= pos && pos <= fn.Body.End() {
			return fn.Name.Name
		}
	}
	return ""
}
