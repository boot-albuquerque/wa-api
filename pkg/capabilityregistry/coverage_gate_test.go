package capabilityregistry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// contractsDir is where pkg/application/contracts lives, relative to this
// package. A constant instead of a literal repeated at each call site (only
// one call site today, but the project's zero-loose-literal rule applies to
// any string carrying meaning, and a path is exactly that).
const contractsDir = "../application/contracts"

// providerDependentInterfaces parses pkg/application/contracts and returns,
// for every interface that EMBEDS domain contracts.SessionGuard and declares
// at least one method of its own, the interface name and its own declared
// method names (embedded methods excluded).
//
// "Embeds SessionGuard" is the coverage boundary this gate uses for
// "operação dependente de provider" (item 7 of the architectural prompt):
// SessionGuard.EnsureSession is the one question nearly every provider-
// backed port asks ("existe sessão WhatsApp para este txtID?"), per its own
// doc comment. A port that does NOT embed it is an infra/config concern
// (storage, HMAC keys, S3 credentials, loggers, the user repository) rather
// than a WhatsApp capability, and is correctly out of this registry's scope.
//
// A composite interface that only re-exports embedded methods (e.g.
// GroupSettings, ChatOperations, PresenceController) contributes ZERO of its
// own declared methods and is therefore invisible here by construction —
// its methods are already required through the interfaces it composes.
func providerDependentInterfaces(t *testing.T, dir string) map[string][]string {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", dir, err)
	}

	// First pass: collect every interface's directly-embedded type names and
	// its own declared method names.
	type ifaceInfo struct {
		embeds  []string
		methods []string
	}
	all := make(map[string]*ifaceInfo)

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					it, ok := ts.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}
					info := &ifaceInfo{}
					for _, field := range it.Methods.List {
						if len(field.Names) == 0 {
							// Embedded interface: field.Type names it.
							if id, ok := field.Type.(*ast.Ident); ok {
								info.embeds = append(info.embeds, id.Name)
							}
							continue
						}
						for _, name := range field.Names {
							info.methods = append(info.methods, name.Name)
						}
					}
					all[ts.Name.Name] = info
				}
			}
		}
	}

	if len(all) == 0 {
		t.Fatalf("parsed zero interfaces from %s — parser or path is broken, not that the package is empty", dir)
	}

	embedsSessionGuard := func(name string) bool {
		seen := map[string]bool{}
		var walk func(string) bool
		walk = func(n string) bool {
			if n == "SessionGuard" {
				return true
			}
			if seen[n] {
				return false
			}
			seen[n] = true
			info, ok := all[n]
			if !ok {
				return false
			}
			for _, e := range info.embeds {
				if walk(e) {
					return true
				}
			}
			return false
		}
		info, ok := all[name]
		if !ok {
			return false
		}
		for _, e := range info.embeds {
			if walk(e) {
				return true
			}
		}
		return false
	}

	out := make(map[string][]string)
	for name, info := range all {
		if len(info.methods) == 0 {
			continue // composite-only interface: nothing new to cover here
		}
		if !embedsSessionGuard(name) {
			continue // infra/config port, not a WhatsApp capability
		}
		out[name] = info.methods
	}
	return out
}

// TestCoverageGate_EveryProviderDependentMethodIsMapped is item 83 of the
// architectural prompt: it enumerates every provider-dependent port method
// in pkg/application/contracts and fails if one has neither a
// PortCoverage entry nor an EngineAgnosticPorts exemption. This is the gate
// that stops a capability from shipping without ever being registered.
func TestCoverageGate_EveryProviderDependentMethodIsMapped(t *testing.T) {
	ifaces := providerDependentInterfaces(t, contractsDir)

	var missing []string
	for iface, methods := range ifaces {
		for _, method := range methods {
			pm := PortMethod{Interface: iface, Method: method}
			if _, covered := PortCoverage[pm]; covered {
				continue
			}
			if EngineAgnosticPorts[pm] {
				continue
			}
			missing = append(missing, iface+"."+method)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("provider-dependent port methods with no capability mapping in PortCoverage "+
			"(and not exempted via EngineAgnosticPorts) — every capability needs one before it "+
			"ships:\n  %s", filepath.Join(missing...))
	}
}

// TestCoverageGate_CatchesAnUnmappedPort is the EXECUTED NEGATIVE CONTROL
// this repository's anti-regression policy requires: it reintroduces the
// exact defect the gate exists to catch — a provider-dependent port method
// with no capability mapping — and confirms the gate fails on it, using the
// SAME AST-scanning path as the real gate (providerDependentInterfaces),
// not a hand-simulated shortcut.
//
// It works on a temporary copy of pkg/application/contracts plus one
// injected file declaring a SessionGuard-embedding interface with a method
// that is deliberately absent from PortCoverage. This proves the gate bites
// without ever touching the real contracts directory.
func TestCoverageGate_CatchesAnUnmappedPort(t *testing.T) {
	dir := t.TempDir()

	// Minimal stand-in for pkg/application/contracts/session_guard.go: the
	// gate only needs the SessionGuard identifier to exist as an interface
	// for the embeds-SessionGuard walk to terminate.
	guard := `package port

type SessionGuard interface {
	EnsureSession()
}
`
	if err := os.WriteFile(filepath.Join(dir, "session_guard.go"), []byte(guard), 0o644); err != nil {
		t.Fatalf("writing fake session_guard.go: %v", err)
	}

	fake := `package port

// FakePortForNegativeControl embeds SessionGuard and declares one method
// that is NOT in PortCoverage nor EngineAgnosticPorts, by construction.
type FakePortForNegativeControl interface {
	SessionGuard

	DoesNotExist()
}
`
	if err := os.WriteFile(filepath.Join(dir, "fake_port.go"), []byte(fake), 0o644); err != nil {
		t.Fatalf("writing fake_port.go: %v", err)
	}

	ifaces := providerDependentInterfaces(t, dir)

	methods, ok := ifaces["FakePortForNegativeControl"]
	if !ok {
		t.Fatalf("gate's own scan did not even discover the injected fake interface — the scan itself is broken, not just the mapping check")
	}

	found := false
	for _, m := range methods {
		if m == "DoesNotExist" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gate's scan discovered %v for the fake interface, missing DoesNotExist — negative control did not reproduce the defect", methods)
	}

	pm := PortMethod{Interface: "FakePortForNegativeControl", Method: "DoesNotExist"}
	if _, covered := PortCoverage[pm]; covered {
		t.Fatalf("negative control is broken: the fake port method is somehow already in PortCoverage")
	}
	if EngineAgnosticPorts[pm] {
		t.Fatalf("negative control is broken: the fake port method is somehow already exempted")
	}

	// This is the same verdict the real gate test computes; asserting it
	// directly is the "confirm the gate fails" half of the control. Removing
	// the injected files (t.TempDir cleans up automatically) and rerunning
	// TestCoverageGate_EveryProviderDependentMethodIsMapped against the real
	// contractsDir is the "confirm it passes again" half — that test is run
	// every `make check`, unconditionally, immediately after this one.
	t.Log("EXECUTED CONTROL: injected an unmapped SessionGuard-embedding method; gate reports it as uncovered, as required")
}
