package bootstrap

import (
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

func noopStartSession(string, string) {}

// minimalDeps builds a Deps value using only in-memory/zero-value
// dependencies — no flag.Parse(), no os.Executable(), no database
// connection, no package globals. Proves buildRouter is reachable from
// outside pkg/bootstrap's process-effect-laden main().
func minimalDeps() Deps {
	return Deps{
		DB:             &sqlx.DB{},
		UserCache:      cache.New(0, 0),
		AdminToken:     "test-admin-token",
		StaticDir:      "", // empty: don't register the FileServer
		StartSession:   noopStartSession,
		CustomHandlers: emptyCustomHandlers(),
	}
}

func TestNewRouter(t *testing.T) {
	router := NewRouter(minimalDeps())
	if router == nil {
		t.Fatal("NewRouter returned nil")
	}

	// The construction path must not touch the process: no os.Executable(),
	// no flag.Parse() side effects. If it did, this call would either panic
	// (unparsed flags are still their zero value, which is fine) or require
	// a real executable path on disk, which minimalDeps() never provides.
	var count int
	_ = router.Walk(func(_ *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		count++
		return nil
	})
	if count == 0 {
		t.Fatal("router has zero registered routes")
	}
}

func TestNewRouter_PanicsWithoutStartSession(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Deps.StartSession is nil, got none")
		}
	}()
	d := minimalDeps()
	d.StartSession = nil
	NewRouter(d)
}

func TestNewRouter_PanicsWithoutCustomHandlers(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Deps.CustomHandlers is nil, got none")
		}
	}()
	d := minimalDeps()
	d.CustomHandlers = nil
	NewRouter(d)
}

func TestRoutes_ZeroValueDeps(t *testing.T) {
	// Routes() must work with a zero-value Deps — no StartSession, no
	// CustomHandlers, no DB — since route enumeration never invokes a
	// handler's ServeHTTP. This is what cmd/listroutes relies on.
	infos := Routes(Deps{})
	if len(infos) < 87 {
		t.Fatalf("Routes() returned %d routes, want at least 87 (the count fixed at the Fase 1a audit)", len(infos))
	}
}

func TestRoutes_IncludesLivez(t *testing.T) {
	infos := Routes(Deps{})
	found := false
	for _, info := range infos {
		if info.Path == "/livez" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("/livez route not found in Routes() output")
	}
}
