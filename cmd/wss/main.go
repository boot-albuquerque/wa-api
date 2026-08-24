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
	os.Args = withStdioMode(os.Args)
	bootstrap.Main()
}

// modeStdio is the flag this command exists to force. It is a constant because
// the string appears in the appending and in the duplicate check, and a literal
// repeated across a producer and its own guard is the same bug waiting to
// diverge (ADR-0004).
const modeStdio = "--mode=stdio"

// withStdioMode is what makes this command different from cmd/core: same
// bootstrap, one flag.
//
// It is a function rather than an inline append so the two properties that
// matter can be asserted. The first is that the caller's arguments SURVIVE — an
// implementation that replaced os.Args instead of extending it would drop the
// program name and every flag the operator passed, and would do so only when
// run for real. The second is that asking for stdio twice does not produce it
// twice, because a flag parser given a repeated flag is entitled to reject it.
func withStdioMode(args []string) []string {
	for _, a := range args {
		if a == modeStdio {
			return args
		}
	}
	return append(args, modeStdio)
}
