// Package main provides the stdio (JSON-RPC 2.0) entry point for wa-api.
// This is a thin wrapper that delegates to bootstrap.MainStdio().
//
// Usage: go build ./cmd/wss && ./wss < input.json
package main

import (
	"os"

	"wa-api/pkg/bootstrap"
)

func main() {
	os.Args = append(os.Args, "--mode=stdio")
	bootstrap.Main()
}
