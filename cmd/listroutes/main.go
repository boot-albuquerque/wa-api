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
	routes := bootstrap.Routes(bootstrap.Deps{})

	lines := make([]string, 0, len(routes)*2)
	for _, r := range routes {
		for _, method := range r.Methods {
			lines = append(lines, method+" "+r.Path)
		}
	}
	sort.Strings(lines)

	for _, line := range lines {
		fmt.Println(line)
	}
}
