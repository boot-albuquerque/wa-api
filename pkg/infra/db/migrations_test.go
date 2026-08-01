package db

import (
	"errors"
	"path/filepath"
	"testing"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// openTestDB abre um SQLite em arquivo (não :memory:) porque o pool do
// database/sql pode abrir mais de uma conexão, e cada conexão para :memory: vê
// um banco diferente.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return db
}

// migrateUpTo aplica as migrações até maxID inclusive, permitindo semear dados
// no estado de schema anterior à migração sob teste.
func migrateUpTo(t *testing.T, db *sqlx.DB, maxID int) {
	t.Helper()
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	for _, m := range migrations {
		if m.ID > maxID {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			t.Fatalf("apply migration %d (%s): %v", m.ID, m.Name, err)
		}
	}
}

func migration11(t *testing.T) Migration {
	t.Helper()
	for _, m := range migrations {
		if m.ID == 11 {
			return m
		}
	}
	t.Fatal("migration 11 (add_token_hash) not found")
	return Migration{}
}

func insertUser(t *testing.T, db *sqlx.DB, id, name, token string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO users (id, name, token, webhook, jid, qrcode, events) VALUES (?, ?, ?, '', '', '', '')",
		id, name, token); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func columnExists(t *testing.T, db *sqlx.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column); err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	return n > 0
}

func TestMigrationTokenHashPopulatesAndIndexes(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)

	insertUser(t, db, "u1", "alice", "token-alice")
	insertUser(t, db, "u2", "bob", "token-bob")

	if err := applyMigration(db, migration11(t)); err != nil {
		t.Fatalf("migration 11 failed on clean data: %v", err)
	}

	for _, tc := range []struct{ id, token string }{
		{"u1", "token-alice"},
		{"u2", "token-bob"},
	} {
		var got string
		if err := db.Get(&got, "SELECT token_hash FROM users WHERE id = ?", tc.id); err != nil {
			t.Fatalf("read token_hash for %s: %v", tc.id, err)
		}
		if want := domain.HashToken(tc.token); got != want {
			t.Errorf("token_hash for %s = %q, want %q", tc.id, got, want)
		}
	}

	var indexes int
	if err := db.Get(&indexes,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_users_token_hash'"); err != nil {
		t.Fatalf("query index: %v", err)
	}
	if indexes != 1 {
		t.Errorf("idx_users_token_hash count = %d, want 1", indexes)
	}
}

// TestMigrationTokenHashAbortsOnDuplicateTokens é o controle negativo do guard:
// sem ele a migração chegaria ao CREATE UNIQUE INDEX e falharia com o erro do
// driver, sem dizer ao operador o que consertar.
func TestMigrationTokenHashAbortsOnDuplicateTokens(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)

	insertUser(t, db, "u1", "alice", "shared-token")
	insertUser(t, db, "u2", "bob", "shared-token")

	err := applyMigration(db, migration11(t))
	if err == nil {
		t.Fatal("migration 11 succeeded with duplicate tokens; the guard is not load-bearing")
	}
	if !errors.Is(err, ErrDuplicateTokens) {
		t.Fatalf("error = %v, want it to wrap ErrDuplicateTokens", err)
	}

	// A transação do runner precisa ter revertido tudo: nem coluna, nem
	// registro da migração como aplicada.
	if columnExists(t, db, "users", "token_hash") {
		t.Error("token_hash column survived the aborted migration; rollback was not clean")
	}
	var applied int
	if err := db.Get(&applied, "SELECT COUNT(*) FROM migrations WHERE id = 11"); err != nil {
		t.Fatalf("query migrations: %v", err)
	}
	if applied != 0 {
		t.Error("migration 11 recorded as applied despite aborting")
	}
}

// TestMigrationTokenHashUniqueIndexRejectsDuplicate prova que o índice criado
// realmente barra a duplicata — é o que add_user/edit_user passam a confiar no
// lugar do SELECT COUNT(*) fora de transação.
func TestMigrationTokenHashUniqueIndexRejectsDuplicate(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 11)

	hash := domain.HashToken("token-alice")
	if _, err := db.Exec(
		"INSERT INTO users (id, name, token, token_hash, webhook, jid, qrcode, events) VALUES (?, ?, ?, ?, '', '', '', '')",
		"u1", "alice", "token-alice", hash); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err := db.Exec(
		"INSERT INTO users (id, name, token, token_hash, webhook, jid, qrcode, events) VALUES (?, ?, ?, ?, '', '', '', '')",
		"u2", "mallory", "token-alice", hash)
	if err == nil {
		t.Fatal("second insert with the same token_hash succeeded; the UNIQUE index is not enforcing")
	}
}

func TestInitializeSchemaAppliesTokenHashOnFreshDatabase(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if !columnExists(t, db, "users", "token_hash") {
		t.Error("fresh database is missing users.token_hash")
	}
}
