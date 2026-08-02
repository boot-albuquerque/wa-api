package db

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type Migration struct {
	ID      int
	Name    string
	UpSQL   string
	DownSQL string
}

var migrations = []Migration{
	{
		ID:    1,
		Name:  "initial_schema",
		UpSQL: initialSchemaSQL,
	},
	{
		ID:   2,
		Name: "add_proxy_url",
		UpSQL: `
            -- PostgreSQL version
            DO $$
            BEGIN
                IF NOT EXISTS (
                    SELECT 1 FROM information_schema.columns
                    WHERE table_name = 'users' AND column_name = 'proxy_url'
                ) THEN
                    ALTER TABLE users ADD COLUMN proxy_url TEXT DEFAULT '';
                END IF;
            END $$;

            -- SQLite version (handled in code)
            `,
	},
	{
		ID:    3,
		Name:  "change_id_to_string",
		UpSQL: changeIDToStringSQL,
	},
	{
		ID:    4,
		Name:  "add_s3_support",
		UpSQL: addS3SupportSQL,
	},
	{
		ID:    5,
		Name:  "add_message_history",
		UpSQL: addMessageHistorySQL,
	},
	{
		ID:    6,
		Name:  "add_quoted_message_id",
		UpSQL: addQuotedMessageIDSQL,
	},
	{
		ID:    7,
		Name:  "add_hmac_key",
		UpSQL: addHmacKeySQL,
	},
	{
		ID:    8,
		Name:  "add_data_json",
		UpSQL: addDataJsonSQL,
	},
	{
		ID:    9,
		Name:  "add_whatsmeow_message_secrets_message_id_idx",
		UpSQL: addWhatsmeowMessageSecretsMessageIDIndexSQL,
	},
	{
		ID:    10,
		Name:  "add_webhook_use_proxy",
		UpSQL: addWebhookUseProxySQL,
	},
	{
		ID:   11,
		Name: "add_token_hash",
		// UpSQL fica vazio de propósito: esta migração precisa calcular
		// SHA-256 em Go (Postgres exige pgcrypto e o driver SQLite não expõe
		// sha256 nenhum), então applyMigration a roteia para
		// applyTokenHashMigration em vez de tx.Exec(UpSQL).
		DownSQL: addTokenHashDownSQL,
	},
}

// addTokenHashDownSQL desfaz a migração 11. Primeiro DownSQL preenchido no
// repositório: o runner ainda não executa down steps (dados/F1), mas sem o SQL
// escrito o rollback teria que ser reconstruído sob pressão em incidente.
const addTokenHashDownSQL = `
DROP INDEX IF EXISTS idx_users_token_hash;
ALTER TABLE users DROP COLUMN token_hash;
`

const changeIDToStringSQL = `
-- Migration to change ID from integer to random string
DO $$
BEGIN
    -- Only execute if the column is currently integer type
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'users' AND column_name = 'id' AND data_type = 'integer'
    ) THEN
        -- For PostgreSQL
        ALTER TABLE users ADD COLUMN new_id TEXT;
		UPDATE users SET new_id = md5(random()::text || id::text || clock_timestamp()::text);
		ALTER TABLE users DROP CONSTRAINT users_pkey;
        ALTER TABLE users DROP COLUMN id CASCADE;
        ALTER TABLE users RENAME COLUMN new_id TO id;
        ALTER TABLE users ALTER COLUMN id SET NOT NULL;
        ALTER TABLE users ADD PRIMARY KEY (id);
    END IF;
END $$;
`

const initialSchemaSQL = `
-- PostgreSQL version
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
        CREATE TABLE users (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            token TEXT NOT NULL,
            webhook TEXT NOT NULL DEFAULT '',
            jid TEXT NOT NULL DEFAULT '',
            qrcode TEXT NOT NULL DEFAULT '',
            connected INTEGER,
            expiration INTEGER,
            events TEXT NOT NULL DEFAULT '',
            proxy_url TEXT DEFAULT ''
        );
    END IF;
END $$;

-- SQLite version (handled in code)
`

const addS3SupportSQL = `
-- PostgreSQL version
DO $$
BEGIN
    -- Add S3 configuration columns if they don't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_enabled') THEN
        ALTER TABLE users ADD COLUMN s3_enabled BOOLEAN DEFAULT FALSE;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_endpoint') THEN
        ALTER TABLE users ADD COLUMN s3_endpoint TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_region') THEN
        ALTER TABLE users ADD COLUMN s3_region TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_bucket') THEN
        ALTER TABLE users ADD COLUMN s3_bucket TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_access_key') THEN
        ALTER TABLE users ADD COLUMN s3_access_key TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_secret_key') THEN
        ALTER TABLE users ADD COLUMN s3_secret_key TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_path_style') THEN
        ALTER TABLE users ADD COLUMN s3_path_style BOOLEAN DEFAULT TRUE;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_public_url') THEN
        ALTER TABLE users ADD COLUMN s3_public_url TEXT DEFAULT '';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'media_delivery') THEN
        ALTER TABLE users ADD COLUMN media_delivery TEXT DEFAULT 'base64';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 's3_retention_days') THEN
        ALTER TABLE users ADD COLUMN s3_retention_days INTEGER DEFAULT 30;
    END IF;
END $$;
`

const addMessageHistorySQL = `
-- PostgreSQL version
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'message_history') THEN
        CREATE TABLE message_history (
            id SERIAL PRIMARY KEY,
            user_id TEXT NOT NULL,
            chat_jid TEXT NOT NULL,
            sender_jid TEXT NOT NULL,
            message_id TEXT NOT NULL,
            timestamp TIMESTAMP NOT NULL,
            message_type TEXT NOT NULL,
            text_content TEXT,
            media_link TEXT,
            UNIQUE(user_id, message_id)
        );
        CREATE INDEX idx_message_history_user_chat_timestamp ON message_history (user_id, chat_jid, timestamp DESC);
    END IF;

    -- Add history column to users table if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'history') THEN
        ALTER TABLE users ADD COLUMN history INTEGER DEFAULT 0;
    END IF;
END $$;
`

const addQuotedMessageIDSQL = `
-- PostgreSQL version
DO $$
BEGIN
    -- Add quoted_message_id column to message_history table if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'message_history' AND column_name = 'quoted_message_id') THEN
        ALTER TABLE message_history ADD COLUMN quoted_message_id TEXT;
    END IF;
END $$;
`

const addDataJsonSQL = `
-- PostgreSQL version
DO $$
BEGIN
    -- Add dataJson column to message_history table if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'message_history' AND column_name = 'datajson') THEN
        ALTER TABLE message_history ADD COLUMN datajson TEXT;
    END IF;
END $$;

-- SQLite version (handled in code)
`

const addWhatsmeowMessageSecretsMessageIDIndexSQL = `
-- PostgreSQL version
DO $$
BEGIN
	CREATE INDEX IF NOT EXISTS whatsmeow_message_secrets_message_id_idx
	ON whatsmeow_message_secrets (message_id);
END $$;
-- SQLite version (handled in code)
`

const addWebhookUseProxySQL = `
-- PostgreSQL version
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'webhook_use_proxy') THEN
        ALTER TABLE users ADD COLUMN webhook_use_proxy BOOLEAN DEFAULT TRUE;
    END IF;
END $$;

-- SQLite version (handled in code)
`

// GenerateRandomID creates a random string ID
func GenerateRandomID() (string, error) {
	bytes := make([]byte, 16) // 128 bits
	if _, err := rand.Read(bytes); err != nil {
		log.Error().Err(err).Str("source", "crypto/rand").Int("bytes", len(bytes)).
			Msg("failed to generate random ID")
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// InitializeSchema initializes the database with migrations
func InitializeSchema(db *sqlx.DB) error {
	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		log.Error().Err(err).Str("table", "migrations").Str("driver", db.DriverName()).
			Msg("failed to create migrations table")
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get already applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		log.Error().Err(err).Str("table", "migrations").Str("driver", db.DriverName()).
			Msg("failed to get applied migrations")
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Apply missing migrations
	for _, migration := range migrations {
		if _, ok := applied[migration.ID]; !ok {
			if err := applyMigration(db, migration); err != nil {
				log.Error().Err(err).Int("migration_id", migration.ID).
					Str("migration", migration.Name).Str("driver", db.DriverName()).
					Msg("failed to apply migration")
				return fmt.Errorf("failed to apply migration %d: %w", migration.ID, err)
			}
		}
	}

	return nil
}

func createMigrationsTable(db *sqlx.DB) error {
	var tableExists bool
	var err error

	switch db.DriverName() {
	case "postgres":
		err = db.Get(&tableExists, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'migrations'
			)`)
	case "sqlite":
		err = db.Get(&tableExists, `
			SELECT EXISTS (
				SELECT 1 FROM sqlite_master
				WHERE type='table' AND name='migrations'
			)`)
	default:
		log.Error().Str("driver", db.DriverName()).Str("table", "migrations").
			Msg("unsupported database driver")
		return fmt.Errorf("unsupported database driver: %s", db.DriverName())
	}

	if err != nil {
		log.Error().Err(err).Str("driver", db.DriverName()).Str("table", "migrations").
			Msg("failed to check migrations table existence")
		return fmt.Errorf("failed to check migrations table existence: %w", err)
	}

	if tableExists {
		return nil
	}

	_, err = db.Exec(`
		CREATE TABLE migrations (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		log.Error().Err(err).Str("driver", db.DriverName()).Str("table", "migrations").
			Msg("failed to create migrations table")
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

func getAppliedMigrations(db *sqlx.DB) (map[int]struct{}, error) {
	applied := make(map[int]struct{})
	var rows []struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	err := db.Select(&rows, "SELECT id, name FROM migrations ORDER BY id ASC")
	if err != nil {
		log.Error().Err(err).Str("table", "migrations").Str("query", "select_applied_migrations").
			Msg("failed to query applied migrations")
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}

	for _, row := range rows {
		applied[row.ID] = struct{}{}
	}

	return applied, nil
}

func applyMigration(db *sqlx.DB, migration Migration) error {
	tx, err := db.Beginx()
	if err != nil {
		log.Error().Err(err).Int("migration_id", migration.ID).Str("migration", migration.Name).
			Msg("failed to begin migration transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			// Rollback error is intentionally discarded: err (the original
			// failure) is already what this function returns, and the
			// transaction is torn down by the driver on connection close
			// regardless of whether Rollback itself succeeds.
			log.Warn().Err(err).Int("migration_id", migration.ID).
				Str("migration", migration.Name).
				Msg("rolling back migration transaction after failure")
			_ = tx.Rollback()
		}
	}()

	//nolint:staticcheck // QF1003: this if/else-if chain over 10 migration.ID
	// branches spans ~130 lines of schema DDL in the migration runner.
	// Rewriting it as a switch is purely cosmetic and carries real risk of a
	// transcription error in production schema migrations; deliberately left
	// untouched during lint cleanup. Revisit alongside the Fase 5b migration
	// work, where this file is already in scope for careful changes.
	if migration.ID == 1 {
		// Handle initial schema creation differently per database
		if db.DriverName() == "sqlite" {
			err = createTableIfNotExistsSQLite(tx, "users", `
                CREATE TABLE users (
                    id TEXT PRIMARY KEY,
                    name TEXT NOT NULL,
                    token TEXT NOT NULL,
                    webhook TEXT NOT NULL DEFAULT '',
                    jid TEXT NOT NULL DEFAULT '',
                    qrcode TEXT NOT NULL DEFAULT '',
                    connected INTEGER,
                    expiration INTEGER,
                    events TEXT NOT NULL DEFAULT '',
                    proxy_url TEXT DEFAULT ''
                )`)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 2 {
		if db.DriverName() == "sqlite" {
			err = addColumnIfNotExistsSQLite(tx, "users", "proxy_url", "TEXT DEFAULT ''")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 3 {
		if db.DriverName() == "sqlite" {
			err = migrateSQLiteIDToString(tx)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 4 {
		if db.DriverName() == "sqlite" {
			// Handle S3 columns for SQLite
			err = addColumnIfNotExistsSQLite(tx, "users", "s3_enabled", "BOOLEAN DEFAULT 0")
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_endpoint", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_region", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_bucket", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_access_key", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_secret_key", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_path_style", "BOOLEAN DEFAULT 1")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_public_url", "TEXT DEFAULT ''")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "media_delivery", "TEXT DEFAULT 'base64'")
			}
			if err == nil {
				err = addColumnIfNotExistsSQLite(tx, "users", "s3_retention_days", "INTEGER DEFAULT 30")
			}
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 5 {
		if db.DriverName() == "sqlite" {
			// Handle message_history table creation for SQLite
			err = createTableIfNotExistsSQLite(tx, "message_history", `
				CREATE TABLE message_history (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id TEXT NOT NULL,
					chat_jid TEXT NOT NULL,
					sender_jid TEXT NOT NULL,
					message_id TEXT NOT NULL,
					timestamp DATETIME NOT NULL,
					message_type TEXT NOT NULL,
					text_content TEXT,
					media_link TEXT,
					UNIQUE(user_id, message_id)
				)`)
			if err == nil {
				// Create index for SQLite
				_, err = tx.Exec(`
					CREATE INDEX IF NOT EXISTS idx_message_history_user_chat_timestamp
					ON message_history (user_id, chat_jid, timestamp DESC)`)
			}
			if err == nil {
				// Add history column to users table
				err = addColumnIfNotExistsSQLite(tx, "users", "history", "INTEGER DEFAULT 0")
			}
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 6 {
		if db.DriverName() == "sqlite" {
			// Add quoted_message_id column to message_history table for SQLite
			err = addColumnIfNotExistsSQLite(tx, "message_history", "quoted_message_id", "TEXT")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 7 {
		if db.DriverName() == "sqlite" {
			// Add hmac_key column as BLOB for encrypted data in SQLite
			err = addColumnIfNotExistsSQLite(tx, "users", "hmac_key", "BLOB")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 8 {
		if db.DriverName() == "sqlite" {
			// Add dataJson column to message_history table for SQLite
			err = addColumnIfNotExistsSQLite(tx, "message_history", "datajson", "TEXT")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 9 {
		if db.DriverName() == "sqlite" {
			err = nil
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 10 {
		if db.DriverName() == "sqlite" {
			err = addColumnIfNotExistsSQLite(tx, "users", "webhook_use_proxy", "BOOLEAN DEFAULT 1")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 11 {
		err = applyTokenHashMigration(tx, db.DriverName())
	} else {
		_, err = tx.Exec(migration.UpSQL)
	}

	if err != nil {
		log.Error().Err(err).Int("migration_id", migration.ID).Str("migration", migration.Name).
			Str("driver", db.DriverName()).Msg("failed to execute migration SQL")
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record the migration
	if _, err = tx.Exec(`
        INSERT INTO migrations (id, name)
        VALUES ($1, $2)`, migration.ID, migration.Name); err != nil {
		log.Error().Err(err).Int("migration_id", migration.ID).Str("migration", migration.Name).
			Str("table", "migrations").Msg("failed to record migration")
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// commitErr é deliberadamente uma variável nova: atribuir a `err` faria o
	// defer acima disparar um Rollback sobre transação já encerrada.
	if commitErr := tx.Commit(); commitErr != nil {
		log.Error().Err(commitErr).Int("migration_id", migration.ID).
			Str("migration", migration.Name).
			Msg("failed to commit migration transaction")
		return commitErr
	}
	return nil
}

// ErrDuplicateTokens é retornado pela migração 11 quando users.token já contém
// valores repetidos. A checagem roda antes de qualquer DDL para que o operador
// receba esta mensagem em vez de uma violação de constraint vinda do driver no
// meio do ALTER TABLE.
var ErrDuplicateTokens = errors.New(
	"migration 11 (add_token_hash) aborted: users.token contains duplicate values; " +
		"resolve them before applying the UNIQUE constraint on token_hash")

func applyTokenHashMigration(tx *sqlx.Tx, driver string) error {
	var duplicates int
	if err := tx.Get(&duplicates, `
        SELECT COUNT(*) FROM (
            SELECT token FROM users GROUP BY token HAVING COUNT(*) > 1
        ) AS duplicated_tokens`); err != nil {
		log.Error().Err(err).Str("table", "users").Str("query", "count_duplicate_tokens").
			Str("driver", driver).Msg("failed to check for duplicate tokens")
		return fmt.Errorf("failed to check for duplicate tokens: %w", err)
	}
	if duplicates > 0 {
		log.Error().Str("table", "users").Str("column", "token").Int("duplicates", duplicates).
			Msg("migration 11 aborted: users.token contains duplicate values")
		return fmt.Errorf("%w (%d duplicated token value(s) found)", ErrDuplicateTokens, duplicates)
	}

	if driver == "sqlite" {
		if err := addColumnIfNotExistsSQLite(tx, "users", "token_hash", "TEXT"); err != nil {
			log.Error().Err(err).Str("table", "users").Str("column", "token_hash").
				Str("driver", driver).Msg("failed to add token_hash column")
			return err
		}
	} else {
		if _, err := tx.Exec(`
            DO $$
            BEGIN
                IF NOT EXISTS (
                    SELECT 1 FROM information_schema.columns
                    WHERE table_name = 'users' AND column_name = 'token_hash'
                ) THEN
                    ALTER TABLE users ADD COLUMN token_hash TEXT;
                END IF;
            END $$;`); err != nil {
			log.Error().Err(err).Str("table", "users").Str("column", "token_hash").
				Str("driver", driver).Msg("failed to add token_hash column")
			return fmt.Errorf("failed to add token_hash column: %w", err)
		}
	}

	// O hash é calculado em Go, não em SQL: Postgres só expõe sha256() com
	// pgcrypto instalado e o driver SQLite não expõe nenhuma, e o valor precisa
	// bater byte a byte com o que a autenticação calcula em domain.HashToken.
	var existing []struct {
		ID    string `db:"id"`
		Token string `db:"token"`
	}
	if err := tx.Select(&existing, "SELECT id, token FROM users"); err != nil {
		log.Error().Err(err).Str("table", "users").Str("query", "select_tokens_for_hashing").
			Msg("failed to read tokens for hashing")
		return fmt.Errorf("failed to read tokens for hashing: %w", err)
	}
	updateSQL := tx.Rebind("UPDATE users SET token_hash = ? WHERE id = ?")
	for _, row := range existing {
		if _, err := tx.Exec(updateSQL, domain.HashToken(row.Token), row.ID); err != nil {
			log.Error().Err(err).Str("table", "users").Str("user_id", row.ID).
				Str("column", "token_hash").Msg("failed to populate token_hash")
			return fmt.Errorf("failed to populate token_hash for user %s: %w", row.ID, err)
		}
	}

	if _, err := tx.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_users_token_hash ON users (token_hash)"); err != nil {
		log.Error().Err(err).Str("table", "users").Str("index", "idx_users_token_hash").
			Msg("failed to create unique index on token_hash")
		return fmt.Errorf("failed to create unique index on token_hash: %w", err)
	}

	return nil
}

func createTableIfNotExistsSQLite(tx *sqlx.Tx, tableName, createSQL string) error {
	var exists int
	err := tx.Get(&exists, `
        SELECT COUNT(*) FROM sqlite_master
        WHERE type='table' AND name=?`, tableName)
	if err != nil {
		log.Error().Err(err).Str("table", tableName).Str("query", "sqlite_master_table_exists").
			Msg("failed to check table existence")
		return err
	}

	if exists == 0 {
		if _, err := tx.Exec(createSQL); err != nil {
			log.Error().Err(err).Str("table", tableName).Msg("failed to create table")
			return err
		}
	}
	return nil
}
func migrateSQLiteIDToString(tx *sqlx.Tx) error {
	// 1. Check if we need to do the migration
	var currentType string
	err := tx.QueryRow(`
        SELECT type FROM pragma_table_info('users')
        WHERE name = 'id'`).Scan(&currentType)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("column", "id").
			Str("query", "pragma_table_info").Msg("failed to check column type")
		return fmt.Errorf("failed to check column type: %w", err)
	}

	if currentType != "INTEGER" {
		// No migration needed
		return nil
	}

	// 2. Create new table with string ID
	_, err = tx.Exec(`
        CREATE TABLE users_new (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            token TEXT NOT NULL,
            webhook TEXT NOT NULL DEFAULT '',
            jid TEXT NOT NULL DEFAULT '',
            qrcode TEXT NOT NULL DEFAULT '',
            connected INTEGER,
            expiration INTEGER,
            events TEXT NOT NULL DEFAULT '',
            proxy_url TEXT DEFAULT ''
        )`)
	if err != nil {
		log.Error().Err(err).Str("table", "users_new").Msg("failed to create new table")
		return fmt.Errorf("failed to create new table: %w", err)
	}

	// 3. Copy data with new UUIDs
	_, err = tx.Exec(`
        INSERT INTO users_new
        SELECT
            hex(randomblob(16)),
            name, token, webhook, jid, qrcode,
            connected, expiration, events, proxy_url
        FROM users`)
	if err != nil {
		log.Error().Err(err).Str("table", "users_new").Str("source_table", "users").
			Msg("failed to copy data")
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// 4. Drop old table
	_, err = tx.Exec(`DROP TABLE users`)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Msg("failed to drop old table")
		return fmt.Errorf("failed to drop old table: %w", err)
	}

	// 5. Rename new table
	_, err = tx.Exec(`ALTER TABLE users_new RENAME TO users`)
	if err != nil {
		log.Error().Err(err).Str("table", "users_new").Msg("failed to rename table")
		return fmt.Errorf("failed to rename table: %w", err)
	}

	return nil
}

func addColumnIfNotExistsSQLite(tx *sqlx.Tx, tableName, columnName, columnDef string) error {
	var exists int
	err := tx.Get(&exists, `
        SELECT COUNT(*) FROM pragma_table_info(?)
        WHERE name = ?`, tableName, columnName)
	if err != nil {
		log.Error().Err(err).Str("table", tableName).Str("column", columnName).
			Str("query", "pragma_table_info").Msg("failed to check column existence")
		return fmt.Errorf("failed to check column existence: %w", err)
	}

	if exists == 0 {
		_, err = tx.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s %s",
			tableName, columnName, columnDef))
		if err != nil {
			log.Error().Err(err).Str("table", tableName).Str("column", columnName).
				Msg("failed to add column")
			return fmt.Errorf("failed to add column: %w", err)
		}
	}
	return nil
}

const addHmacKeySQL = `
-- PostgreSQL version - Add encrypted HMAC key column
DO $$
BEGIN
    -- Add hmac_key column as BYTEA for encrypted data
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'hmac_key') THEN
        ALTER TABLE users ADD COLUMN hmac_key BYTEA;
    END IF;
END $$;

-- SQLite version (handled in code)
`
