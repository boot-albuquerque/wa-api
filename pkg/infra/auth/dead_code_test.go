package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuth_NoCacheConstructor prevents reintroduction of the dead
// NewTokenCache constructor (HOUSEKEEP F207).
//
// NewTokenCache existed in cache.go and was only called from tests. The
// production cache is created directly by bootstrap/config.go and
// bootstrap/context.go. A second constructor in the auth package suggests two
// caches that need to stay coherent — exactly the confusion that cost an
// auditor a full round of investigation on the F201.
func TestAuth_NoCacheConstructor(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read package directory: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("cannot read %s: %v", name, err)
		}
		if strings.Contains(string(data), "func NewTokenCache(") {
			t.Fatalf("F207: %s still exports NewTokenCache — this was dead code "+
				"(only called from tests) and its presence suggests a second cache "+
				"that auditors must account for. The production cache is created in "+
				"bootstrap/config.go and bootstrap/context.go.", name)
		}
	}
}
