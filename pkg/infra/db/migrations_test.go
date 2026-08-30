package db

import (
	"errors"
	"path/filepath"
	"testing"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// openTestDB opens a file-backed SQLite (not :memory:) because the
// database/sql pool may open multiple connections, each of which would see a
// separate in-memory database. SQLitePragmas matches production — see
// connection.go.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db")+SQLitePragmas)
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

// engineOf lê a coluna crua, sem passar pelo repositório — o mesmo motivo do
// helper homônimo em user_engine_test.go (package db_test, inacessível
// daqui): o teste da migração tem de ver o que está GRAVADO.
func engineOf(t *testing.T, db *sqlx.DB, id string) string {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT engine FROM users WHERE id = ?`, id); err != nil {
		t.Fatalf("read engine of %q: %v", id, err)
	}
	return got
}

func migration22(t *testing.T) Migration {
	t.Helper()
	for _, m := range migrations {
		if m.ID == migrationIDEngineWireRename {
			return m
		}
	}
	t.Fatal("migration 22 (rename_users_engine_wire_values) not found")
	return Migration{}
}

// TestMigrationEngineWireRenameRewritesExistingRows: F385's clean cutover
// (HOUSEKEEP) claims every row written under the old wire values
// (`wa_noise`/`wa_headless`) is rewritten to the new ones (`noise`/
// `headless`) — not just that new writes use the new values. The
// measurement that matters is a row that ALREADY HELD the old value before
// this migration ran, exactly what a database migrated from before
// 2026-08-29 looks like.
func TestMigrationEngineWireRenameRewritesExistingRows(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 21)

	insertUser(t, db, "u1", "alice", "token-alice")
	insertUser(t, db, "u2", "bob", "token-bob")
	insertUser(t, db, "u3", "carol", "token-carol")

	// Simula o estado ANTES do corte: linhas gravadas pela API enquanto
	// domain.EngineNoise/EngineHeadless ainda valiam "wa_noise"/"wa_headless".
	if _, err := db.Exec(`UPDATE users SET engine = 'wa_noise' WHERE id = 'u1'`); err != nil {
		t.Fatalf("seed u1 as wa_noise: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET engine = 'wa_headless' WHERE id = 'u2'`); err != nil {
		t.Fatalf("seed u2 as wa_headless: %v", err)
	}
	// u3 fica em legacy_unknown (o default da migração 19) — controle
	// negativo: uma linha que NUNCA teve engine definido não pode virar
	// "noise" nem "headless" só porque a migração 22 rodou por perto.

	if err := applyMigration(db, migration22(t)); err != nil {
		t.Fatalf("migration 22 failed: %v", err)
	}

	for _, tc := range []struct{ id, want string }{
		{"u1", "noise"},
		{"u2", "headless"},
		{"u3", string(domain.EngineLegacyUnknown)},
	} {
		got := engineOf(t, db, tc.id)
		if got != tc.want {
			t.Errorf("engine of %s after migration 22 = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// TestMigrationEngineWireRenameIsRecordedAndIdempotent: rodar a migração
// duas vezes (o que InitializeSchema faz em todo arranque, verificando a
// tabela migrations) não pode reescrever uma linha já correta nem falhar.
func TestMigrationEngineWireRenameIsRecordedAndIdempotent(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 21)

	insertUser(t, db, "u1", "alice", "token-alice")
	if _, err := db.Exec(`UPDATE users SET engine = 'wa_noise' WHERE id = 'u1'`); err != nil {
		t.Fatalf("seed u1 as wa_noise: %v", err)
	}

	m22 := migration22(t)
	if err := applyMigration(db, m22); err != nil {
		t.Fatalf("first apply of migration 22: %v", err)
	}
	if got := engineOf(t, db, "u1"); got != "noise" {
		t.Fatalf("engine after first apply = %q, want %q", got, "noise")
	}

	// UpSQL sozinho (sem passar por applyMigration, que recusaria reinserir
	// o id 22 na tabela migrations) — prova que o UPDATE em si é idempotente,
	// não só que InitializeSchema não o roda duas vezes.
	if _, err := db.Exec(m22.UpSQL); err != nil {
		t.Fatalf("re-running migration 22 UpSQL: %v", err)
	}
	if got := engineOf(t, db, "u1"); got != "noise" {
		t.Errorf("engine after re-running UpSQL = %q, want %q (idempotency broken)", got, "noise")
	}

	var applied int
	if err := db.Get(&applied, "SELECT COUNT(*) FROM migrations WHERE id = ?", migrationIDEngineWireRename); err != nil {
		t.Fatalf("query migrations: %v", err)
	}
	if applied != 1 {
		t.Errorf("migrations table has %d row(s) for id=22, want 1", applied)
	}
}
