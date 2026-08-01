package bootstrap

import (
	"encoding/json"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	"wa-api/pkg/infra/db"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// -update regenerates the golden files instead of comparing against them —
// the same flag name `go test -update` uses across the Go ecosystem.
var updateGolden = flag.Bool("update", false, "update golden files in testdata/golden")

// goldenTestDeps builds Deps against a real, freshly migrated in-memory
// sqlite database — not a zero-value one. With no matching token, AuthAlice
// queries succeed and return zero rows (a clean 401), instead of every
// route producing the same undifferentiated "panic on nil *sql.DB,
// recovered into 500" response. CustomHandlers stays at its zero value:
// every route this test drives is rejected by auth before any handler
// method is invoked, so unwired use cases inside the handlers are never
// reached.
func goldenTestDeps(t *testing.T) Deps {
	t.Helper()

	sqlDB, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.InitializeSchema(sqlDB); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	return Deps{
		DB:             sqlDB,
		UserCache:      cache.New(0, 0),
		AdminToken:     "golden-test-admin-token",
		StartClient:    noopStartClient,
		CustomHandlers: emptyCustomHandlers(),
		Log:            zerolog.Nop(),
	}
}

// pathParamValues supplies a stand-in for every {param} this router
// registers (verified against wiring_routes.go: only {id} and {jid}).
var pathParamValues = map[string]string{
	"id":  "golden-test-id",
	"jid": "5511999999999_s.whatsapp.net",
}

var pathParamPattern = regexp.MustCompile(`\{([a-zA-Z]+)\}`)

func fillPathParams(path string) string {
	return pathParamPattern.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := pathParamValues[name]; ok {
			return v
		}
		return "unmapped-path-param"
	})
}

var goldenNameReplacer = strings.NewReplacer("/", "_", "{", "", "}", "")

func goldenFileName(method, path string) string {
	return method + goldenNameReplacer.Replace(path) + ".json"
}

// goldenEntry is the on-disk shape of one captured response: status code
// plus the raw JSON body, so a diff shows exactly what changed.
type goldenEntry struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// TestGolden captures the error contract of every registered route: hit
// with no credentials, does the response shape stay the same? This is the
// mitigation the Fase 4a plan requires to run BEFORE any RespondJSON
// envelope change — see recover_test.go's commit and this file's commit,
// both landing before the envelope commit that follows.
//
// Scope, deliberately: this covers the error path (~all of these routes
// reject with 401/404 before reaching a handler), not the success
// envelope — see plano-correcao-wa-api.md §1.4 for why success is a
// separate, later follow-up.
func TestGolden(t *testing.T) {
	d := goldenTestDeps(t)
	router := NewRouter(d)

	routes := Routes(d)
	if len(routes) < 87 {
		t.Fatalf("Routes() returned %d routes, want at least 87 — golden coverage would silently shrink", len(routes))
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Path < routes[j].Path })

	goldenDir := filepath.Join("testdata", "golden")
	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
	}

	seen := make(map[string]bool)

	for _, route := range routes {
		for _, method := range route.Methods {
			method, route := method, route
			name := goldenFileName(method, route.Path)
			if seen[name] {
				t.Fatalf("duplicate golden file name %q for %s %s — sanitization collided with another route", name, method, route.Path)
			}
			seen[name] = true

			goldenPath := filepath.Join(goldenDir, name)
			t.Run(name, func(t *testing.T) {
				runGoldenCase(t, router, method, route.Path, goldenPath)
			})
		}
	}
}

// runGoldenCase drives one request through router and either records the
// response as the golden file (when -update is set) or compares it against
// the existing one.
func runGoldenCase(t *testing.T, router *mux.Router, method, path, goldenPath string) {
	t.Helper()

	target := fillPathParams(path)
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body json.RawMessage
	if rec.Body.Len() > 0 {
		body = json.RawMessage(rec.Body.Bytes())
	}
	got := goldenEntry{Status: rec.Code, Body: body}

	if *updateGolden {
		writeGoldenCase(t, goldenPath, got)
		return
	}

	want := readGoldenCase(t, goldenPath, method, path)
	if got.Status != want.Status {
		t.Errorf("status = %d, want %d", got.Status, want.Status)
	}
	if !jsonEqual(got.Body, want.Body) {
		t.Errorf("body =\n%s\nwant\n%s", got.Body, want.Body)
	}
}

func writeGoldenCase(t *testing.T, goldenPath string, entry goldenEntry) {
	t.Helper()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden entry: %v", err)
	}
	if err := os.WriteFile(goldenPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden file: %v", err)
	}
}

func readGoldenCase(t *testing.T, goldenPath, method, path string) goldenEntry {
	t.Helper()
	wantData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file missing for %s %s (run: go test ./pkg/bootstrap/ -run TestGolden -update): %v", method, path, err)
	}
	var want goldenEntry
	if err := json.Unmarshal(wantData, &want); err != nil {
		t.Fatalf("unmarshal golden file: %v", err)
	}
	return want
}

// jsonEqual compares two raw JSON values structurally, not byte-for-byte —
// key order in a map isn't semantically meaningful.
func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var av, bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	aCanon, _ := json.Marshal(av)
	bCanon, _ := json.Marshal(bv)
	return string(aCanon) == string(bCanon)
}
