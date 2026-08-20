package main

import (
	"os"
	"strings"
	"testing"

	"wa-api/pkg/bootstrap"
)

// cmd/core and cmd/wss are the same program with one difference: wss forces
// --mode=stdio into argv before calling bootstrap.Main, and core does not.
// Nothing else in the tree states that, so these tests do.

// TestCoreDoesNotInjectAModeFlag. The injection in cmd/wss happens at run time,
// in main. If it were ever moved into an init() or into a file shared by both
// commands, cmd/core would silently become a stdio process — it would start,
// report healthy, and never serve HTTP.
//
// Importing this package is what runs its package-level initialisation, so the
// check is on argv as observed here.
func TestCoreDoesNotInjectAModeFlag(t *testing.T) {
	for _, a := range os.Args {
		if strings.HasPrefix(a, "--mode=") {
			t.Fatalf("argv carries %q after loading cmd/core; this command must "+
				"leave the mode to configuration, which is the only thing that "+
				"separates it from cmd/wss", a)
		}
	}
}

// TestCoreServesRoutes is the smoke test proper: this binary exists to serve an
// HTTP API, and a route table that came back empty would mean the process
// starts, reports healthy and answers nothing.
//
// It builds the table rather than the server, so nothing is bound and no
// dependency is required — Deps{} is the same zero value cmd/listroutes uses.
func TestCoreServesRoutes(t *testing.T) {
	routes := bootstrap.Routes(bootstrap.Deps{})
	if len(routes) == 0 {
		t.Fatal("bootstrap registered zero routes; a core process built on this " +
			"would come up healthy and serve nothing")
	}
	// A route with NO methods is not automatically a defect: gorilla/mux
	// PathPrefix subrouters are mount points, and their methods live on the
	// children. Measured: exactly one such entry, "/admin", registered at
	// pkg/bootstrap/router.go:268 as PathPrefix("/admin").Subrouter().
	//
	// The first version of this test asserted that every route declares a
	// method and failed on that mount — the TEST was wrong, not the router. So
	// the property asserted here is the one that actually holds: a route with
	// no methods must have something mounted UNDER it, because a method-less
	// leaf is unreachable by any request.
	mounts := 0
	for _, r := range routes {
		if !strings.HasPrefix(r.Path, "/") {
			t.Fatalf("route path %q is not rooted", r.Path)
		}
		if len(r.Methods) > 0 {
			continue
		}
		mounts++
		child := false
		for _, other := range routes {
			if other.Path != r.Path && strings.HasPrefix(other.Path, r.Path) && len(other.Methods) > 0 {
				child = true
				break
			}
		}
		if !child {
			t.Fatalf("route %q registers no method and nothing is mounted under it, "+
				"so no request can ever reach it", r.Path)
		}
	}
	t.Logf("%d routes registered, %d of them mount points", len(routes), mounts)
}
