package db

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// TestSQLitePragmas_AppliedOnOpen verifies that the three production pragmas
// are active on a file-backed database opened with SQLitePragmas. If someone
// removes or renames a pragma in the constant, this test fails — it is the
// negative-control anchor for the constant.
func TestSQLitePragmas_AppliedOnOpen(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "pragmas.db")+SQLitePragmas)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cases := []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"busy_timeout", "10000"},
		{"foreign_keys", "1"},
	}

	for _, tc := range cases {
		var got string
		if err := db.QueryRow("PRAGMA " + tc.pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}
