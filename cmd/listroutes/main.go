// Command listroutes prints every registered route as "METHOD PATH", one
// per line. It exists so the golden-file harness (Fase 4a) and the F3.5
// readiness check can enumerate routes without booting a full server:
//
//	go run ./cmd/listroutes | wc -l
package main

import (
	"fmt"
	"sort"

	"wa-api/pkg/bootstrap"
)

func main() {
	for _, line := range renderLines(bootstrap.Routes(bootstrap.Deps{})) {
		fmt.Println(line)
	}
}

// renderLines is separated from main so the property the golden-file harness
// actually depends on can be asserted: the output is SORTED and stable.
//
// A harness that diffs this output against a golden file cannot tell "a route
// changed" from "the order changed", and map iteration in Go is randomised by
// design — so an unsorted rendering would produce spurious diffs that teach
// people to ignore the diff.
func renderLines(routes []bootstrap.RouteInfo) []string {
	lines := make([]string, 0, len(routes)*2)
	for _, r := range routes {
		for _, method := range r.Methods {
			lines = append(lines, method+" "+r.Path)
		}
	}
	sort.Strings(lines)
	return lines
}
