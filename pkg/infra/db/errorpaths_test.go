package db

// Caminhos de erro do adaptador de persistência.
//
// Nenhum destes testes mocka o driver: todos moldam o SCHEMA para que o
// SQLite real recuse a operação — tabela ausente, coluna ausente, colisão de
// nome de objeto, trigger que aborta, transação já encerrada. É a mesma via de
// migrations_test.go e message_history_test.go (modernc.org/sqlite em
// t.TempDir(), sem CGO e sem Docker), estendida para o lado negativo.
//
// O que fica descoberto por construção, e por quê:
//   - os dois `sqlx.Open` de connection.go: database/sql só falha em Open com
//     driver não registrado, e ambos os drivers estão registrados por import;
//   - `rand.Read` em GenerateRandomID: crypto/rand não é injetável;
//   - `RowsAffected` em CreateUser/DeleteUser: modernc.org/sqlite nunca falha;
//   - `rows.Err()` em ListUsers: exigiria derrubar a conexão no meio da
//     iteração, o que não é determinístico;
//   - o ramo `isUniqueViolation` de CreateUser: é código morto, porque o
//     `ON CONFLICT DO NOTHING` sem alvo absorve a violação antes que o driver
//     devolva erro (ver TestUserRepositoryCreateUser_DuplicateTokenIsSwallowedByOnConflict).
//
// Os dois caminhos exclusivos de PostgreSQL são o risco R4 do plano: cobertura
// por SQLite não é evidência sobre `lib/pq`, e isso é follow-up nomeado.

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// deadTx devolve uma transação já revertida. Toda operação sobre ela falha com
// sql.ErrTxDone — é o jeito mais barato de exercitar os ramos de erro dos
// helpers que recebem *sqlx.Tx sem inventar um driver falso.
func deadTx(t *testing.T, db *sqlx.DB) *sqlx.Tx {
	t.Helper()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	return tx
}

// liveTx devolve uma transação aberta, revertida no fim do teste.
func liveTx(t *testing.T, db *sqlx.DB) *sqlx.Tx {
	t.Helper()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func mustExec(t *testing.T, db *sqlx.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// -------------------------------------------------------------------------
// connection.go
// -------------------------------------------------------------------------

func TestInitializeDatabase_SQLiteCreatesUsableHandle(t *testing.T) {
	for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT", "DB_SSLMODE"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()

	db, err := InitializeDatabase(dir, "")
	if err != nil {
		t.Fatalf("InitializeDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Ping(); err != nil {
		t.Errorf("ping: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dbdata", "users.db")); err != nil {
		t.Errorf("users.db not created: %v", err)
	}
}

func TestInitializeDatabase_SQLiteFailsWhenDataDirIsAFile(t *testing.T) {
	for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	// "dbdata" já existe como ARQUIVO: o MkdirAll de initializeSQLite não tem
	// como criar o diretório por cima dele.
	blocker := filepath.Join(dir, "dbdata")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	db, err := InitializeDatabase(dir, "")
	if err == nil {
		_ = db.Close()
		t.Fatal("InitializeDatabase succeeded with dbdata occupied by a file")
	}
	if !strings.Contains(err.Error(), "could not create dbdata directory") {
		t.Errorf("error = %v, want it to mention the dbdata directory", err)
	}
}

func TestInitializeDatabase_SQLitePingFailsWhenDBPathIsADirectory(t *testing.T) {
	for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	// users.db existe como DIRETÓRIO: MkdirAll e Open passam (Open é lazy), e
	// quem recusa é o Ping, que é o primeiro a tocar o arquivo de verdade.
	if err := os.MkdirAll(filepath.Join(dir, "dbdata", "users.db"), 0o751); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db, err := InitializeDatabase(dir, "")
	if err == nil {
		_ = db.Close()
		t.Fatal("InitializeDatabase succeeded with users.db occupied by a directory")
	}
	if !strings.Contains(err.Error(), "failed to ping sqlite database") {
		t.Errorf("error = %v, want a sqlite ping failure", err)
	}
}

func TestInitializeDatabase_UsesDataDirFlagOverExecPath(t *testing.T) {
	for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT"} {
		t.Setenv(k, "")
	}
	exPath, dataDir := t.TempDir(), t.TempDir()

	db, err := InitializeDatabase(exPath, dataDir)
	if err != nil {
		t.Fatalf("InitializeDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(filepath.Join(dataDir, "dbdata", "users.db")); err != nil {
		t.Errorf("db not created under the datadir flag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exPath, "dbdata")); !os.IsNotExist(err) {
		t.Error("db was created under exPath despite the datadir flag")
	}
}

// TestInitializeDatabase_PostgresPingFailsWithoutServer cobre o ramo postgres
// de ponta a ponta sem exigir Docker: o DSN é sintaticamente válido, a porta é
// fechada, e quem recusa é o Ping.
func TestInitializeDatabase_PostgresPingFailsWithoutServer(t *testing.T) {
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "d")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	t.Setenv("DB_SSLMODE", "false")

	db, err := InitializeDatabase(t.TempDir(), "")
	if err == nil {
		_ = db.Close()
		t.Fatal("InitializeDatabase succeeded against a closed postgres port")
	}
	if !strings.Contains(err.Error(), "failed to ping postgres database") {
		t.Errorf("error = %v, want a postgres ping failure", err)
	}
}

func TestGetDatabaseConfig_SSLModeRequireWhenTrue(t *testing.T) {
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "d")
	t.Setenv("DB_HOST", "h")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_SSLMODE", "verify-full")

	cfg := GetDatabaseConfig("/ex", "")
	if cfg.Type != "postgres" {
		t.Fatalf("Type = %q, want postgres", cfg.Type)
	}
	if cfg.SSLMode != "verify-full" {
		t.Errorf("SSLMode = %q, want it passed through verbatim", cfg.SSLMode)
	}
}

// TestGetDatabaseConfig_PartialPostgresEnvFallsBackToSQLite fixa o
// comportamento que o Warn novo denuncia: env de Postgres pela metade não
// promove a conexão, ela cai para SQLite.
func TestGetDatabaseConfig_PartialPostgresEnvFallsBackToSQLite(t *testing.T) {
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "d")
	t.Setenv("DB_HOST", "h")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_SSLMODE", "")

	cfg := GetDatabaseConfig("/ex", "")
	if cfg.Type != "sqlite" {
		t.Fatalf("Type = %q, want sqlite when DB_PASSWORD is missing", cfg.Type)
	}
	if cfg.Path != filepath.Join("/ex", "dbdata") {
		t.Errorf("Path = %q, want it under exPath", cfg.Path)
	}
}

func TestSetDisconnectedState_ReportsErrorWhenTableIsMissing(t *testing.T) {
	for _, clearEvents := range []bool{false, true} {
		db := openTestDB(t)
		if err := SetDisconnectedState(db, "u1", clearEvents); err == nil {
			t.Errorf("clearEvents=%v: succeeded without a users table", clearEvents)
		}
	}
}

// -------------------------------------------------------------------------
// message_history.go
// -------------------------------------------------------------------------

func TestSaveMessageToHistory_ReportsErrorWhenTableIsMissing(t *testing.T) {
	db := newHistoryDB(t)
	mustExec(t, db, "DROP TABLE message_history")

	err := SaveMessageToHistory(db, "u1", "c", "s", "M1", "text", "hi", "", "", "{}")
	if err == nil {
		t.Fatal("SaveMessageToHistory succeeded without message_history")
	}
	if !strings.Contains(err.Error(), "failed to save message to history") {
		t.Errorf("error = %v, want it wrapped with the save context", err)
	}
}

func TestTrimMessageHistory_ReportsSecretsFailureFirst(t *testing.T) {
	db := newHistoryDB(t)
	mustExec(t, db, "DROP TABLE whatsmeow_message_secrets")

	err := TrimMessageHistory(db, "u1", "c", 5)
	if err == nil {
		t.Fatal("TrimMessageHistory succeeded without whatsmeow_message_secrets")
	}
	if !strings.Contains(err.Error(), "failed to trim message secrets") {
		t.Errorf("error = %v, want the secrets failure", err)
	}
}

func TestTrimMessageHistory_ReportsHistoryFailure(t *testing.T) {
	db := newHistoryDB(t)
	// message_history precisa existir — o DELETE dos segredos faz um subselect
	// nela, então derrubá-la faria a falha cair no primeiro passo. O trigger
	// deixa a leitura passar e aborta só o DELETE, que é o ramo sob teste.
	mustExec(t, db, `CREATE TRIGGER block_history_delete BEFORE DELETE ON message_history
		BEGIN SELECT RAISE(ABORT, 'deletes are blocked'); END`)
	if err := SaveMessageToHistory(db, "u1", "c", "s", "M1", "text", "hi", "", "", "{}"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// limit 0 faz a linha semeada entrar no subselect, então o DELETE de fato
	// tenta remover algo e o trigger dispara.
	err := TrimMessageHistory(db, "u1", "c", 0)
	if err == nil {
		t.Fatal("TrimMessageHistory succeeded against an aborting delete trigger")
	}
	if !strings.Contains(err.Error(), "failed to trim message history") {
		t.Errorf("error = %v, want the history failure", err)
	}
}

// TestTrimMessageHistory_BuildsPostgresQueriesForPostgresDriver cobre o ramo
// postgres da seleção de SQL sem servidor: sqlx.NewDb aceita um nome de driver
// arbitrário, então DriverName() reporta "postgres" enquanto o backend real
// continua sendo o SQLite. O SQL do ramo postgres usa OFFSET sem LIMIT, que o
// SQLite recusa — e é exatamente essa recusa que prova qual ramo rodou.
func TestTrimMessageHistory_BuildsPostgresQueriesForPostgresDriver(t *testing.T) {
	raw := newHistoryDB(t)
	pg := sqlx.NewDb(raw.DB, "postgres")

	if got := pg.DriverName(); got != "postgres" {
		t.Fatalf("DriverName = %q, want postgres", got)
	}
	err := TrimMessageHistory(pg, "u1", "c", 5)
	if err == nil {
		t.Fatal("postgres-shaped query ran on sqlite; the driver branch was not taken")
	}
	if !strings.Contains(err.Error(), "failed to trim message secrets") {
		t.Errorf("error = %v, want the secrets step to fail first", err)
	}
}

// -------------------------------------------------------------------------
// migrations.go — createMigrationsTable / getAppliedMigrations
// -------------------------------------------------------------------------

func TestCreateMigrationsTable_RejectsUnsupportedDriver(t *testing.T) {
	raw := openTestDB(t)
	weird := sqlx.NewDb(raw.DB, "mysql")

	err := createMigrationsTable(weird)
	if err == nil {
		t.Fatal("createMigrationsTable accepted an unsupported driver")
	}
	if !strings.Contains(err.Error(), "unsupported database driver: mysql") {
		t.Errorf("error = %v, want it to name the driver", err)
	}
}

func TestCreateMigrationsTable_ReportsExistenceCheckFailure(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	err := createMigrationsTable(db)
	if err == nil {
		t.Fatal("createMigrationsTable succeeded on a closed database")
	}
	if !strings.Contains(err.Error(), "failed to check migrations table existence") {
		t.Errorf("error = %v, want the existence-check failure", err)
	}
}

// TestCreateMigrationsTable_ReportsCreateFailure usa uma VIEW chamada
// "migrations": a checagem de existência filtra type='table' e não a vê, então
// o CREATE TABLE roda e colide com o nome já ocupado.
func TestCreateMigrationsTable_ReportsCreateFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE seed (x TEXT)")
	mustExec(t, db, "CREATE VIEW migrations AS SELECT x FROM seed")

	err := createMigrationsTable(db)
	if err == nil {
		t.Fatal("createMigrationsTable succeeded with the name already taken by a view")
	}
	if !strings.Contains(err.Error(), "failed to create migrations table") {
		t.Errorf("error = %v, want the create failure", err)
	}
}

// TestCreateMigrationsTable_PostgresBranchQueriesInformationSchema cobre o
// `case "postgres"` da seleção de dialeto: information_schema não existe no
// SQLite, então a recusa é a prova de qual consulta foi montada.
func TestCreateMigrationsTable_PostgresBranchQueriesInformationSchema(t *testing.T) {
	raw := openTestDB(t)
	pg := sqlx.NewDb(raw.DB, "postgres")

	err := createMigrationsTable(pg)
	if err == nil {
		t.Fatal("the information_schema query ran on sqlite; the postgres branch was not taken")
	}
	if !strings.Contains(err.Error(), "failed to check migrations table existence") {
		t.Errorf("error = %v, want the existence-check failure", err)
	}
}

func TestCreateMigrationsTable_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestGetAppliedMigrations_ReportsQueryFailure(t *testing.T) {
	db := openTestDB(t)
	// A tabela existe, mas sem as colunas que o SELECT pede.
	mustExec(t, db, "CREATE TABLE migrations (unrelated TEXT)")

	_, err := getAppliedMigrations(db)
	if err == nil {
		t.Fatal("getAppliedMigrations succeeded against a mis-shaped table")
	}
	if !strings.Contains(err.Error(), "failed to query applied migrations") {
		t.Errorf("error = %v, want the query failure", err)
	}
}

func TestGetAppliedMigrations_ReturnsAppliedIDs(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 11)

	applied, err := getAppliedMigrations(db)
	if err != nil {
		t.Fatalf("getAppliedMigrations: %v", err)
	}
	for id := 1; id <= 11; id++ {
		if _, ok := applied[id]; !ok {
			t.Errorf("migration %d missing from the applied set", id)
		}
	}
}

// -------------------------------------------------------------------------
// migrations.go — InitializeSchema
// -------------------------------------------------------------------------

func TestInitializeSchema_ReportsCreateMigrationsTableFailure(t *testing.T) {
	raw := openTestDB(t)
	weird := sqlx.NewDb(raw.DB, "mysql")

	err := InitializeSchema(weird)
	if err == nil {
		t.Fatal("InitializeSchema succeeded on an unsupported driver")
	}
	if !strings.Contains(err.Error(), "failed to create migrations table") {
		t.Errorf("error = %v, want the migrations-table failure", err)
	}
}

func TestInitializeSchema_ReportsAppliedLookupFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE migrations (unrelated TEXT)")

	err := InitializeSchema(db)
	if err == nil {
		t.Fatal("InitializeSchema succeeded against a mis-shaped migrations table")
	}
	if !strings.Contains(err.Error(), "failed to get applied migrations") {
		t.Errorf("error = %v, want the applied-lookup failure", err)
	}
}

// TestInitializeSchema_ReportsMigrationFailure ocupa o nome users_new antes da
// migração 3, que precisa criá-lo para reescrever o id de INTEGER para TEXT.
func TestInitializeSchema_ReportsMigrationFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, `CREATE TABLE users (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL, token TEXT NOT NULL,
		webhook TEXT NOT NULL DEFAULT '', jid TEXT NOT NULL DEFAULT '',
		qrcode TEXT NOT NULL DEFAULT '', connected INTEGER, expiration INTEGER,
		events TEXT NOT NULL DEFAULT '', proxy_url TEXT DEFAULT '')`)
	mustExec(t, db, "CREATE TABLE users_new (occupied TEXT)")

	err := InitializeSchema(db)
	if err == nil {
		t.Fatal("InitializeSchema succeeded with users_new already occupied")
	}
	if !strings.Contains(err.Error(), "failed to apply migration 3") {
		t.Errorf("error = %v, want migration 3 to be named", err)
	}
}

func TestInitializeSchema_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("second run: %v", err)
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM migrations"); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if n != len(migrations) {
		t.Errorf("recorded migrations = %d, want %d", n, len(migrations))
	}
}

// -------------------------------------------------------------------------
// migrations.go — applyMigration
// -------------------------------------------------------------------------

func TestApplyMigration_ReportsBeginFailure(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	err := applyMigration(db, migrations[0])
	if err == nil {
		t.Fatal("applyMigration succeeded on a closed database")
	}
	if !strings.Contains(err.Error(), "failed to begin transaction") {
		t.Errorf("error = %v, want the begin failure", err)
	}
}

// TestApplyMigration_ReportsRecordFailure aplica a mesma migração duas vezes:
// a segunda passa pelo DDL (idempotente) e morre no INSERT em migrations, cuja
// chave primária já está ocupada.
func TestApplyMigration_ReportsRecordFailure(t *testing.T) {
	db := openTestDB(t)
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	if err := applyMigration(db, migrations[0]); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	err := applyMigration(db, migrations[0])
	if err == nil {
		t.Fatal("applyMigration recorded the same migration twice")
	}
	if !strings.Contains(err.Error(), "failed to record migration") {
		t.Errorf("error = %v, want the record failure", err)
	}
}

// TestApplyMigration_PostgresBranchExecutesUpSQL prova que um driver não-sqlite
// cai no tx.Exec(UpSQL) de cada migração: o SQL é PL/pgSQL, que o SQLite
// recusa, então o erro é a evidência do ramo.
func TestApplyMigration_PostgresBranchExecutesUpSQL(t *testing.T) {
	raw := openTestDB(t)
	if err := createMigrationsTable(raw); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	pg := sqlx.NewDb(raw.DB, "postgres")

	for _, m := range migrations {
		if m.ID == 11 {
			continue // roteada para applyTokenHashMigration, coberta à parte
		}
		err := applyMigration(pg, m)
		if err == nil {
			t.Errorf("migration %d: PL/pgSQL body ran on sqlite", m.ID)
			continue
		}
		if !strings.Contains(err.Error(), "failed to execute migration SQL") {
			t.Errorf("migration %d error = %v, want the exec failure", m.ID, err)
		}
	}

	// O defer de rollback precisa ter deixado o banco limpo: nenhuma migração
	// registrada como aplicada.
	var n int
	if err := raw.Get(&n, "SELECT COUNT(*) FROM migrations"); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if n != 0 {
		t.Errorf("migrations recorded = %d, want 0 after every attempt failed", n)
	}
}

// TestApplyMigration_ReportsCommitFailure usa `PRAGMA defer_foreign_keys=ON`:
// com ele o SQLite adia a checagem de chave estrangeira para o COMMIT, então o
// DDL e o INSERT em migrations passam e a transação só é recusada no fim. É o
// único jeito determinístico de exercitar o ramo de commit sem derrubar a
// conexão no meio da operação.
func TestApplyMigration_ReportsCommitFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "PRAGMA foreign_keys=ON")
	mustExec(t, db, "CREATE TABLE parent (id INTEGER PRIMARY KEY)")
	mustExec(t, db, "CREATE TABLE child (p INTEGER REFERENCES parent(id))")
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	m := Migration{
		ID:    998,
		Name:  "orphan_child",
		UpSQL: "PRAGMA defer_foreign_keys=ON; INSERT INTO child VALUES (42);",
	}
	err := applyMigration(db, m)
	if err == nil {
		t.Fatal("applyMigration committed a transaction with a violated foreign key")
	}
	if !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Errorf("error = %v, want the deferred constraint failure from COMMIT", err)
	}

	// Deliberadamente NÃO se afirma nada sobre o que sobrou no banco: quando o
	// COMMIT do SQLite falha por constraint adiada, a transação continua
	// ABERTA em vez de ser desfeita, e o que é visível depois depende de como
	// o driver devolve a conexão ao pool. O defer de rollback de applyMigration
	// só dispara para falhas ANTERIORES ao commit (é o que a variável separada
	// `commitErr` preserva), então este caminho não tem contrato de limpeza
	// para testar — o contrato é apenas propagar o erro, que é o que se afirma
	// acima.
}

func TestApplyMigration_UnknownIDFallsBackToUpSQL(t *testing.T) {
	db := openTestDB(t)
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	m := Migration{ID: 999, Name: "sentinel", UpSQL: "CREATE TABLE sentinel (x TEXT)"}
	if err := applyMigration(db, m); err != nil {
		t.Fatalf("applyMigration: %v", err)
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sentinel'"); err != nil {
		t.Fatalf("query sentinel: %v", err)
	}
	if n != 1 {
		t.Error("UpSQL of an unknown migration ID was not executed")
	}
}

// TestMigrateSQLiteIDToString_RewritesLegacyIntegerIDs exercita a migração 3
// pelo caminho feliz, que só roda em bancos vindos da versão antiga.
func TestMigrateSQLiteIDToString_RewritesLegacyIntegerIDs(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, `CREATE TABLE users (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL, token TEXT NOT NULL,
		webhook TEXT NOT NULL DEFAULT '', jid TEXT NOT NULL DEFAULT '',
		qrcode TEXT NOT NULL DEFAULT '', connected INTEGER, expiration INTEGER,
		events TEXT NOT NULL DEFAULT '', proxy_url TEXT DEFAULT '')`)
	mustExec(t, db, "INSERT INTO users (id, name, token) VALUES (7, 'legacy', 'tok-legacy')")

	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}

	var id, name string
	if err := db.QueryRow("SELECT id, name FROM users").Scan(&id, &name); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if name != "legacy" {
		t.Errorf("name = %q, want the row preserved", name)
	}
	if id == "7" || len(id) != 32 {
		t.Errorf("id = %q, want a 32-char random hex string, not the old integer", id)
	}
}

func TestMigrateSQLiteIDToString_SkipsWhenIDIsAlreadyText(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 2)

	if err := migrateSQLiteIDToString(liveTx(t, db)); err != nil {
		t.Fatalf("migrateSQLiteIDToString on a TEXT id: %v", err)
	}
}

func TestMigrateSQLiteIDToString_ReportsColumnTypeCheckFailure(t *testing.T) {
	db := openTestDB(t)

	err := migrateSQLiteIDToString(liveTx(t, db))
	if err == nil {
		t.Fatal("migrateSQLiteIDToString succeeded without a users table")
	}
	if !strings.Contains(err.Error(), "failed to check column type") {
		t.Errorf("error = %v, want the column-type check failure", err)
	}
}

func TestMigrateSQLiteIDToString_ReportsCreateFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE users (id INTEGER PRIMARY KEY)")
	mustExec(t, db, "CREATE TABLE users_new (occupied TEXT)")

	err := migrateSQLiteIDToString(liveTx(t, db))
	if err == nil {
		t.Fatal("migrateSQLiteIDToString succeeded with users_new occupied")
	}
	if !strings.Contains(err.Error(), "failed to create new table") {
		t.Errorf("error = %v, want the create failure", err)
	}
}

// TestMigrateSQLiteIDToString_ReportsCopyFailure dá à tabela legada apenas o
// id: o SELECT da cópia referencia name/token/..., que não existem.
func TestMigrateSQLiteIDToString_ReportsCopyFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE users (id INTEGER PRIMARY KEY)")

	err := migrateSQLiteIDToString(liveTx(t, db))
	if err == nil {
		t.Fatal("migrateSQLiteIDToString succeeded copying from a table without the columns")
	}
	if !strings.Contains(err.Error(), "failed to copy data") {
		t.Errorf("error = %v, want the copy failure", err)
	}
}

// TestMigrateSQLiteIDToString_ReportsDropFailure usa uma chave estrangeira com
// linha filha: com foreign_keys ligado, o DROP TABLE users é recusado.
func TestMigrateSQLiteIDToString_ReportsDropFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "PRAGMA foreign_keys=ON")
	mustExec(t, db, `CREATE TABLE users (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL, token TEXT NOT NULL,
		webhook TEXT NOT NULL DEFAULT '', jid TEXT NOT NULL DEFAULT '',
		qrcode TEXT NOT NULL DEFAULT '', connected INTEGER, expiration INTEGER,
		events TEXT NOT NULL DEFAULT '', proxy_url TEXT DEFAULT '')`)
	mustExec(t, db, "CREATE TABLE dependent (owner INTEGER REFERENCES users(id))")
	mustExec(t, db, "INSERT INTO users (id, name, token) VALUES (1, 'a', 't')")
	mustExec(t, db, "INSERT INTO dependent (owner) VALUES (1)")

	tx := liveTx(t, db)
	if _, err := tx.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys on tx: %v", err)
	}

	err := migrateSQLiteIDToString(tx)
	if err == nil {
		t.Fatal("migrateSQLiteIDToString dropped a table with live foreign key children")
	}
	if !strings.Contains(err.Error(), "failed to drop old table") {
		t.Errorf("error = %v, want the drop failure", err)
	}
}

// TestMigrateSQLiteIDToString_ReportsRenameFailure ocupa o nome "users" com um
// ÍNDICE: a tabela some no DROP, mas o namespace de objetos continua tomado,
// então o RENAME TO users falha.
func TestMigrateSQLiteIDToString_ReportsRenameFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, `CREATE TABLE users (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL, token TEXT NOT NULL,
		webhook TEXT NOT NULL DEFAULT '', jid TEXT NOT NULL DEFAULT '',
		qrcode TEXT NOT NULL DEFAULT '', connected INTEGER, expiration INTEGER,
		events TEXT NOT NULL DEFAULT '', proxy_url TEXT DEFAULT '')`)
	mustExec(t, db, "CREATE TABLE decoy (c TEXT)")
	mustExec(t, db, "CREATE INDEX users_ix_placeholder ON decoy (c)")
	// O índice é renomeado por baixo para ocupar exatamente o nome "users".
	mustExec(t, db, "PRAGMA writable_schema=ON")
	mustExec(t, db, "UPDATE sqlite_master SET name='users' WHERE name='users_ix_placeholder'")
	mustExec(t, db, "PRAGMA writable_schema=OFF")

	err := migrateSQLiteIDToString(liveTx(t, db))
	if err == nil {
		t.Fatal("migrateSQLiteIDToString renamed onto an occupied object name")
	}
	if !strings.Contains(err.Error(), "failed to rename table") {
		t.Errorf("error = %v, want the rename failure", err)
	}
}

// -------------------------------------------------------------------------
// migrations.go — helpers SQLite
// -------------------------------------------------------------------------

func TestCreateTableIfNotExistsSQLite_ReportsExistenceCheckFailure(t *testing.T) {
	db := openTestDB(t)

	err := createTableIfNotExistsSQLite(deadTx(t, db), "whatever", "CREATE TABLE whatever (x TEXT)")
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("error = %v, want sql.ErrTxDone from the finished transaction", err)
	}
}

func TestCreateTableIfNotExistsSQLite_ReportsCreateFailure(t *testing.T) {
	db := openTestDB(t)

	err := createTableIfNotExistsSQLite(liveTx(t, db), "broken", "CREATE TABLE broken (")
	if err == nil {
		t.Fatal("createTableIfNotExistsSQLite accepted malformed DDL")
	}
}

func TestCreateTableIfNotExistsSQLite_SkipsWhenTableExists(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE already (x TEXT)")

	// O createSQL é sintaticamente inválido de propósito: se o helper tentasse
	// executá-lo, o teste falharia — a existência prévia tem de curto-circuitar.
	if err := createTableIfNotExistsSQLite(liveTx(t, db), "already", "NOT SQL AT ALL"); err != nil {
		t.Fatalf("createTableIfNotExistsSQLite on an existing table: %v", err)
	}
}

func TestAddColumnIfNotExistsSQLite_ReportsExistenceCheckFailure(t *testing.T) {
	db := openTestDB(t)

	err := addColumnIfNotExistsSQLite(deadTx(t, db), "users", "c", "TEXT")
	if err == nil {
		t.Fatal("addColumnIfNotExistsSQLite succeeded on a finished transaction")
	}
	if !strings.Contains(err.Error(), "failed to check column existence") {
		t.Errorf("error = %v, want the existence-check failure", err)
	}
}

// TestAddColumnIfNotExistsSQLite_ReportsAddFailure pede a coluna em caixa alta:
// a checagem de existência compara o nome literalmente e não acha, mas o
// ALTER TABLE do SQLite compara sem distinguir caixa e recusa a duplicata.
func TestAddColumnIfNotExistsSQLite_ReportsAddFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE users (id TEXT, name TEXT)")

	err := addColumnIfNotExistsSQLite(liveTx(t, db), "users", "NAME", "TEXT")
	if err == nil {
		t.Fatal("addColumnIfNotExistsSQLite added a column that already exists")
	}
	if !strings.Contains(err.Error(), "failed to add column") {
		t.Errorf("error = %v, want the add failure", err)
	}
}

func TestAddColumnIfNotExistsSQLite_SkipsWhenColumnExists(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE users (id TEXT, name TEXT)")

	if err := addColumnIfNotExistsSQLite(liveTx(t, db), "users", "name", "TEXT"); err != nil {
		t.Fatalf("addColumnIfNotExistsSQLite on an existing column: %v", err)
	}
}

// -------------------------------------------------------------------------
// migrations.go — applyTokenHashMigration
// -------------------------------------------------------------------------

func TestApplyTokenHashMigration_ReportsDuplicateCheckFailure(t *testing.T) {
	db := openTestDB(t)

	err := applyTokenHashMigration(liveTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration succeeded without a users table")
	}
	if !strings.Contains(err.Error(), "failed to check for duplicate tokens") {
		t.Errorf("error = %v, want the duplicate-check failure", err)
	}
}

func TestApplyTokenHashMigration_ReportsDuplicateCheckFailureOnDeadTx(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)

	err := applyTokenHashMigration(deadTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration succeeded on a finished transaction")
	}
}

// TestApplyTokenHashMigration_ReportsAddColumnFailure explora a assimetria de
// caixa entre as duas checagens: pragma_table_info compara o nome com `=`, que
// distingue maiúsculas, então "TOKEN_HASH" não casa com 'token_hash' e o
// helper decide adicionar; já o ALTER TABLE do SQLite compara sem distinguir
// caixa e recusa a coluna duplicada.
func TestApplyTokenHashMigration_ReportsAddColumnFailure(t *testing.T) {
	db := openTestDB(t)
	mustExec(t, db, "CREATE TABLE users (id TEXT, token TEXT, TOKEN_HASH TEXT)")
	mustExec(t, db, "INSERT INTO users (id, token) VALUES ('u1', 'tok')")

	err := applyTokenHashMigration(liveTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration added token_hash on top of an existing TOKEN_HASH")
	}
	if !strings.Contains(err.Error(), "failed to add column") {
		t.Errorf("error = %v, want the add-column failure", err)
	}
}

// TestApplyTokenHashMigration_PostgresBranchAddsColumnViaDDL cobre o ramo
// não-sqlite: o corpo é PL/pgSQL, que o SQLite recusa.
func TestApplyTokenHashMigration_PostgresBranchAddsColumnViaDDL(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)

	err := applyTokenHashMigration(liveTx(t, db), "postgres")
	if err == nil {
		t.Fatal("PL/pgSQL body ran on sqlite; the postgres branch was not taken")
	}
	if !strings.Contains(err.Error(), "failed to add token_hash column") {
		t.Errorf("error = %v, want the add-column failure", err)
	}
}

// TestApplyTokenHashMigration_ReportsTokenReadFailure remove a coluna token
// depois que a guarda de duplicatas já rodou — o SELECT de leitura para hashing
// é o próximo a tocá-la.
func TestApplyTokenHashMigration_ReportsTokenReadFailure(t *testing.T) {
	db := openTestDB(t)
	// users tem token (a guarda de duplicatas passa) mas o SELECT de leitura
	// pede id também, que aqui não existe.
	mustExec(t, db, "CREATE TABLE users (token TEXT)")

	err := applyTokenHashMigration(liveTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration succeeded without users.id")
	}
	if !strings.Contains(err.Error(), "failed to read tokens for hashing") {
		t.Errorf("error = %v, want the token-read failure", err)
	}
}

// TestApplyTokenHashMigration_ReportsPopulateFailure instala um trigger que
// aborta todo UPDATE em users: o preenchimento do token_hash morre linha a
// linha, que é o ramo com o user_id no log.
func TestApplyTokenHashMigration_ReportsPopulateFailure(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)
	insertUser(t, db, "u1", "alice", "token-alice")
	mustExec(t, db, `CREATE TRIGGER block_update BEFORE UPDATE ON users
		BEGIN SELECT RAISE(ABORT, 'updates are blocked'); END`)

	err := applyTokenHashMigration(liveTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration succeeded against an aborting trigger")
	}
	if !strings.Contains(err.Error(), "failed to populate token_hash for user u1") {
		t.Errorf("error = %v, want the populate failure naming the user", err)
	}
}

// TestApplyTokenHashMigration_ReportsIndexFailure ocupa o nome do índice com
// uma TABELA: o IF NOT EXISTS do CREATE INDEX não salva de uma colisão de
// namespace com outro tipo de objeto.
func TestApplyTokenHashMigration_ReportsIndexFailure(t *testing.T) {
	db := openTestDB(t)
	migrateUpTo(t, db, 10)
	insertUser(t, db, "u1", "alice", "token-alice")
	mustExec(t, db, "CREATE TABLE idx_users_token_hash (occupied TEXT)")

	err := applyTokenHashMigration(liveTx(t, db), "sqlite")
	if err == nil {
		t.Fatal("applyTokenHashMigration succeeded with the index name already taken")
	}
	if !strings.Contains(err.Error(), "failed to create unique index on token_hash") {
		t.Errorf("error = %v, want the index failure", err)
	}
}

// -------------------------------------------------------------------------
// migrations.go — GenerateRandomID
// -------------------------------------------------------------------------

func TestGenerateRandomID_ReturnsDistinct32CharHex(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for range 64 {
		id, err := GenerateRandomID()
		if err != nil {
			t.Fatalf("GenerateRandomID: %v", err)
		}
		if len(id) != 32 {
			t.Fatalf("id = %q, want 32 hex chars", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("GenerateRandomID returned %q twice", id)
		}
		seen[id] = struct{}{}
	}
}

// -------------------------------------------------------------------------
// user_repository.go
// -------------------------------------------------------------------------

func newRepoDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return db
}

func TestIsUniqueViolation_ClassifiesByDriverMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sqlite", errors.New("constraint failed: UNIQUE constraint failed: users.token_hash"), true},
		{"postgres", errors.New(`pq: duplicate key value violates unique constraint "idx_users_token_hash"`), true},
		{"unrelated", errors.New("no such table: users"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUniqueViolation(tc.err); got != tc.want {
				t.Errorf("isUniqueViolation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserRepositoryCreateUser_InsertsAndReportsAffected(t *testing.T) {
	r := NewUserRepository(newRepoDB(t))
	ctx := context.Background()

	created, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "tok-1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if !created {
		t.Error("created = false on a fresh insert")
	}
}

// TestUserRepositoryCreateUser_ConflictOnIDReportsNotCreated exercita o
// ON CONFLICT DO NOTHING: o mesmo id não gera erro, só zero linhas afetadas.
func TestUserRepositoryCreateUser_ConflictOnIDReportsNotCreated(t *testing.T) {
	r := NewUserRepository(newRepoDB(t))
	ctx := context.Background()

	if _, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "tok-1"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	created, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice again", Token: "tok-2"})
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if created {
		t.Error("created = true for a row that conflicted on the primary key")
	}
}

// TestUserRepositoryCreateUser_DuplicateTokenIsSwallowedByOnConflict fixa o
// contrato REAL, que não é o que o corpo de CreateUser sugere: o
// `ON CONFLICT DO NOTHING` não tem alvo, então ele absorve TODA violação de
// unicidade — inclusive a de token_hash, não só a da chave primária. O driver
// nunca devolve erro, e por isso o ramo `isUniqueViolation` de CreateUser é
// inalcançável: quem sinaliza duplicidade é `created == false`.
//
// É desse `false` que AddUserUseCase deriva ErrDuplicateToken (ver
// TestAddUserRejectsDuplicateToken em user_repository_test.go), então o
// comportamento observável pela API está correto — o que não existe é o
// caminho de erro interno. Registrado como achado, não corrigido aqui:
// remover o ramo morto mudaria a assinatura de erro de CreateUser.
func TestUserRepositoryCreateUser_DuplicateTokenIsSwallowedByOnConflict(t *testing.T) {
	db := newRepoDB(t)
	r := NewUserRepository(db)
	ctx := context.Background()

	if _, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "shared"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	created, err := r.CreateUser(ctx, domain.UserRecord{ID: "u2", Name: "mallory", Token: "shared"})
	if err != nil {
		t.Fatalf("second insert error = %v, want nil: ON CONFLICT DO NOTHING absorbs it", err)
	}
	if created {
		t.Fatal("created = true for a duplicate token; the UNIQUE index is not enforcing")
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM users WHERE token_hash = ?", domain.HashToken("shared")); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("rows with the shared token = %d, want 1", n)
	}
}

func TestUserRepositoryCreateUser_PropagatesNonUniqueFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, "DROP TABLE users")
	r := NewUserRepository(db)

	_, err := r.CreateUser(context.Background(), domain.UserRecord{ID: "u1", Name: "a", Token: "t"})
	if err == nil {
		t.Fatal("CreateUser succeeded without a users table")
	}
	if errors.Is(err, domain.ErrDuplicateToken) {
		t.Errorf("error = %v, want the raw driver failure, not ErrDuplicateToken", err)
	}
}

func TestUserRepositoryUserExists_ReportsPresenceAndAbsence(t *testing.T) {
	db := newRepoDB(t)
	r := NewUserRepository(db)
	ctx := context.Background()

	if _, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "t"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := r.UserExists(ctx, "u1")
	if err != nil {
		t.Fatalf("UserExists(u1): %v", err)
	}
	if !got {
		t.Error("UserExists(u1) = false, want true")
	}

	got, err = r.UserExists(ctx, "ghost")
	if err != nil {
		t.Fatalf("UserExists(ghost): %v", err)
	}
	if got {
		t.Error("UserExists(ghost) = true, want false")
	}
}

func TestUserRepositoryUserExists_PropagatesQueryFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, "DROP TABLE users")

	if _, err := NewUserRepository(db).UserExists(context.Background(), "u1"); err == nil {
		t.Fatal("UserExists succeeded without a users table")
	}
}

func TestUserRepositoryUpdateUser_AppliesEveryField(t *testing.T) {
	db := newRepoDB(t)
	r := NewUserRepository(db)
	ctx := context.Background()
	if _, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "t"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	name, token, webhook, events := "renamed", "new-token", "https://hook.example", "Message"
	proxy := "http://proxy.example"
	expiration, history := 99, 25
	useProxy := true
	upd := domain.UserUpdate{
		Name: &name, Token: &token, Webhook: &webhook, Events: &events,
		Expiration: &expiration, History: &history,
		ProxyURL: &proxy, WebhookUseProxy: &useProxy,
		S3: &domain.S3Config{
			Enabled: true, Endpoint: "https://s3.example", Region: "us-east-1",
			Bucket: "b", AccessKey: "ak", SecretKey: "sk", PathStyle: true,
			PublicURL: "https://cdn.example", MediaDelivery: "s3", RetentionDays: 7,
		},
	}
	if err := r.UpdateUser(ctx, "u1", upd); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	var got struct {
		Name      string `db:"name"`
		Token     string `db:"token"`
		TokenHash string `db:"token_hash"`
		Bucket    string `db:"s3_bucket"`
		History   int    `db:"history"`
	}
	if err := db.Get(&got,
		"SELECT name, token, token_hash, s3_bucket, history FROM users WHERE id = ?", "u1"); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Name != "renamed" || got.Token != "new-token" || got.Bucket != "b" || got.History != 25 {
		t.Errorf("row = %+v, want every field applied", got)
	}
	if want := domain.HashToken("new-token"); got.TokenHash != want {
		t.Errorf("token_hash = %q, want %q", got.TokenHash, want)
	}
}

func TestUserRepositoryUpdateUser_RejectsEmptyUpdate(t *testing.T) {
	r := NewUserRepository(newRepoDB(t))

	err := r.UpdateUser(context.Background(), "u1", domain.UserUpdate{})
	if !errors.Is(err, domain.ErrNoFieldsToUpdate) {
		t.Fatalf("error = %v, want domain.ErrNoFieldsToUpdate", err)
	}
}

func TestUserRepositoryUpdateUser_PropagatesNonUniqueFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, "DROP TABLE users")
	name := "x"

	err := NewUserRepository(db).UpdateUser(context.Background(), "u1", domain.UserUpdate{Name: &name})
	if err == nil {
		t.Fatal("UpdateUser succeeded without a users table")
	}
	if errors.Is(err, domain.ErrDuplicateToken) {
		t.Errorf("error = %v, want the raw driver failure", err)
	}
}

func TestUserRepositoryListUsers_ReturnsAllAndFiltersByID(t *testing.T) {
	db := newRepoDB(t)
	r := NewUserRepository(db)
	ctx := context.Background()
	for _, id := range []string{"u1", "u2"} {
		if _, err := r.CreateUser(ctx, domain.UserRecord{
			ID: id, Name: id, Token: "tok-" + id, ProxyURL: "http://p." + id,
			S3: domain.S3Config{Enabled: true, Bucket: "bucket-" + id, MediaDelivery: "s3"},
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	all, err := r.ListUsers(ctx, "")
	if err != nil {
		t.Fatalf("ListUsers(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("users = %d, want 2", len(all))
	}
	if !all[0].HasProxyURL {
		t.Error("HasProxyURL = false for a user with a proxy set")
	}
	if all[0].S3.Bucket != "bucket-u1" {
		t.Errorf("S3.Bucket = %q, want the second query to have run", all[0].S3.Bucket)
	}

	one, err := r.ListUsers(ctx, "u2")
	if err != nil {
		t.Fatalf("ListUsers(u2): %v", err)
	}
	if len(one) != 1 || one[0].ID != "u2" {
		t.Errorf("filtered result = %+v, want exactly u2", one)
	}
}

func TestUserRepositoryListUsers_PropagatesQueryFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, "DROP TABLE users")

	if _, err := NewUserRepository(db).ListUsers(context.Background(), ""); err == nil {
		t.Fatal("ListUsers succeeded without a users table")
	}
}

// TestUserRepositoryListUsers_PropagatesScanFailure põe texto na coluna
// connected, que o destino sql.NullBool não consegue converter.
func TestUserRepositoryListUsers_PropagatesScanFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, `INSERT INTO users (id, name, token, token_hash, webhook, jid, qrcode, events, connected)
		VALUES ('u1', 'alice', 't', 'h', '', '', '', '', 'not-a-bool')`)

	if _, err := NewUserRepository(db).ListUsers(context.Background(), ""); err == nil {
		t.Fatal("ListUsers succeeded scanning a non-boolean into connected")
	}
}

func TestUserRepositoryUserS3Config_ReportsMissingUser(t *testing.T) {
	r := NewUserRepository(newRepoDB(t))

	_, err := r.userS3Config(context.Background(), "ghost")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestUserRepositoryDeleteUser_RemovesAndReportsMiss(t *testing.T) {
	db := newRepoDB(t)
	r := NewUserRepository(db)
	ctx := context.Background()
	if _, err := r.CreateUser(ctx, domain.UserRecord{ID: "u1", Name: "alice", Token: "t"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	deleted, err := r.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !deleted {
		t.Error("deleted = false for an existing row")
	}

	deleted, err = r.DeleteUser(ctx, "u1")
	if err != nil {
		t.Fatalf("second DeleteUser: %v", err)
	}
	if deleted {
		t.Error("deleted = true for a row that was already gone")
	}
}

func TestUserRepositoryDeleteUser_PropagatesQueryFailure(t *testing.T) {
	db := newRepoDB(t)
	mustExec(t, db, "DROP TABLE users")

	if _, err := NewUserRepository(db).DeleteUser(context.Background(), "u1"); err == nil {
		t.Fatal("DeleteUser succeeded without a users table")
	}
}
