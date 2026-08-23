package main

import (
	"sort"
	"strings"
	"testing"

	"wa-api/pkg/bootstrap"
)

// TestRenderLinesIsSortedAndStable guards the property the golden-file harness
// depends on and that nothing else asserts.
//
// The harness diffs this command's output against a stored file. Go randomises
// map iteration by design, so a rendering that followed registration order
// would produce diffs that mean nothing — and a diff that is noisy half the
// time is a diff people stop reading.
func TestRenderLinesIsSortedAndStable(t *testing.T) {
	in := []bootstrap.RouteInfo{
		{Path: "/zeta", Methods: []string{"GET"}},
		{Path: "/alpha", Methods: []string{"POST", "GET"}},
		{Path: "/beta", Methods: []string{"DELETE"}},
	}
	got := renderLines(in)

	want := []string{"DELETE /beta", "GET /alpha", "GET /zeta", "POST /alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("the output is not sorted: %v", got)
	}
}

// TestEveryMethodOfEveryRouteIsEmitted: the count is what
// `go run ./cmd/listroutes | wc -l` measures, and a route with two methods must
// contribute two lines. Emitting one per ROUTE instead of one per METHOD would
// silently shrink that count.
func TestEveryMethodOfEveryRouteIsEmitted(t *testing.T) {
	in := []bootstrap.RouteInfo{
		{Path: "/a", Methods: []string{"GET", "POST", "PUT"}},
		{Path: "/b", Methods: []string{"GET"}},
	}
	if got := renderLines(in); len(got) != 4 {
		t.Fatalf("got %d lines for 4 method/path pairs: %v", len(got), got)
	}
}

// TestTheRealRouteTableIsNotEmpty is the smoke test proper: this command exists
// so other tooling can enumerate routes without booting a server, and an empty
// answer would make every consumer report "no routes" as success.
func TestTheRealRouteTableIsNotEmpty(t *testing.T) {
	lines := renderLines(bootstrap.Routes(bootstrap.Deps{}))
	if len(lines) == 0 {
		t.Fatal("the real route table rendered zero lines; every consumer of this " +
			"command would read that as a healthy empty API")
	}
	for _, l := range lines {
		method, path, ok := strings.Cut(l, " ")
		if !ok || method == "" || !strings.HasPrefix(path, "/") {
			t.Fatalf("line %q is not \"METHOD /path\", which is the documented format", l)
		}
	}
	t.Logf("%d method/path pairs registered", len(lines))
}
