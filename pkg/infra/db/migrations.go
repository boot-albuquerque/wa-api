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
		Name:  "add_wanoise_message_secrets_message_id_idx",
		UpSQL: addWaNoiseMessageSecretsMessageIDIndexSQL,
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
	{
		ID:      12,
		Name:    "rename_message_secrets_index",
		UpSQL:   renameMessageSecretsIndexSQL,
		DownSQL: renameMessageSecretsIndexDownSQL,
	},
	{
		ID:      13,
		Name:    "add_message_history_sender_push_name",
		UpSQL:   addSenderPushNameSQL,
		DownSQL: addSenderPushNameDownSQL,
	},
	{
		ID:      migrationIDSessionLeases,
		Name:    "add_session_leases",
		UpSQL:   addSessionLeasesSQL,
		DownSQL: addSessionLeasesDownSQL,
	},
	{
		ID:      migrationIDWebhookOutbox,
		Name:    "add_webhook_outbox",
		UpSQL:   addWebhookOutboxSQL,
		DownSQL: addWebhookOutboxDownSQL,
	},
	{
		ID:   migrationIDBlankPlaintextToken,
		Name: "blank_plaintext_token",
		// UpSQL vazio: esta migração roda em Go (ver
		// applyBlankPlaintextTokenMigration). O hash precisa bater byte a byte
		// com domain.HashToken, e nem SQLite nem Postgres calculam SHA-256 sem
		// extensão — mesmo motivo da migração 11.
		UpSQL:   "",
		DownSQL: "",
	},
	{
		ID:      migrationIDLeaseOwnerAddr,
		Name:    "add_lease_owner_addr",
		UpSQL:   addLeaseOwnerAddrSQL,
		DownSQL: addLeaseOwnerAddrDownSQL,
	},
	{
		ID:      migrationIDLabels,
		Name:    "add_labels",
		UpSQL:   addLabelsSQL,
		DownSQL: addLabelsDownSQL,
	},
	{
		ID:      migrationIDUsersEngine,
		Name:    migrationNameUsersEngine,
		UpSQL:   addUsersEngineSQL,
		DownSQL: addUsersEngineDownSQL,
	},
	{
		ID:      migrationIDAccountOwnership,
		Name:    migrationNameAccountOwnership,
		UpSQL:   accountOwnershipSQL,
		DownSQL: accountOwnershipDownSQL,
	},
}

// migrationIDBlankPlaintextToken apaga o token em texto claro das linhas
// existentes (F97 etapa 2).
const migrationIDBlankPlaintextToken = 16

// migrationIDLeaseOwnerAddr acrescenta o endereço do dono à tabela de posse
// (ADR-0007, decisão 1).
const migrationIDLeaseOwnerAddr = 17

// migrationIDLabels acompanha a F191: os três eventos de etiqueta que a
// biblioteca emite (LabelEdit, LabelAssociationChat, LabelAssociationMessage)
// chegavam e eram deitados fora. Estas tabelas são onde eles passam a parar.
const migrationIDLabels = 18

// migrationIDWebhookOutbox identifica a migração do outbox de webhook, pelo
// mesmo motivo da constante acima: três lugares a referenciam.
const migrationIDWebhookOutbox = 15

// addWebhookOutboxSQL cria o outbox de entrega de webhook (ADR-0005, D3).
//
// Isto AMENDA a F88, que registrou que retry durável exigiria RabbitMQ e um
// consumidor. Está errado: durabilidade exige armazenamento TRANSACIONAL, e
// SQLite é um. Hoje o retry vive só em memória (`time.AfterFunc` em
// dispatch_retry.go), então todo reinício do processo perde silenciosamente o
// que estava pendente — e sob k8s reinício é rotina, não exceção.
//
// `due_at` é quando a linha volta a ser elegível, e é também o mecanismo de
// posse: reivindicar empurra `due_at` para frente, de modo que outra réplica
// não pegue a mesma linha enquanto esta trabalha. Um só campo faz as duas
// coisas, e não há estado "em processamento" que possa ficar preso se o
// processo morrer no meio — o prazo vence sozinho.
//
// NÃO guarda a chave HMAC. Ela já vive em `users.hmac_key` e é relida por
// user_id na retomada: duplicar segredo em outra tabela multiplica a
// superfície de vazamento sem comprar nada.
//
// `hmac_scope` existe porque a chave tem DUAS origens: o webhook do usuário
// assina com `users.hmac_key`, e o webhook global assina com a chave global do
// processo. Sem o discriminador, a retomada teria de adivinhar comparando a URL
// com a configuração ATUAL — e uma mudança de configuração faria entregas
// antigas serem assinadas com a chave errada, silenciosamente. Não é segredo:
// é qual segredo usar.
//
// A tabela é criada nos DOIS dialetos, como a de posse, para que uma
// instalação que migre de `single` para `multi` não precise de migração
// retroativa.
const addWebhookOutboxSQL = `
CREATE TABLE IF NOT EXISTS webhook_outbox (
    id         TEXT PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    url        TEXT        NOT NULL,
    payload    TEXT        NOT NULL,
    attempt    INTEGER     NOT NULL DEFAULT 0,
    due_at     TIMESTAMPTZ NOT NULL,
    hmac_scope TEXT        NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_webhook_outbox_due_at ON webhook_outbox (due_at);
`

// addWebhookOutboxSQLiteSQL é a mesma tabela sem TIMESTAMPTZ, que o SQLite não
// conhece. Aqui, ao contrário da tabela de posse, ela NÃO é inerte: o cenário
// catastrófico do ADR — um pod com SQLite — é exatamente onde durabilidade de
// entrega precisa funcionar.
const addWebhookOutboxSQLiteSQL = `
CREATE TABLE IF NOT EXISTS webhook_outbox (
    id         TEXT PRIMARY KEY,
    user_id    TEXT      NOT NULL,
    url        TEXT      NOT NULL,
    payload    TEXT      NOT NULL,
    attempt    INTEGER   NOT NULL DEFAULT 0,
    due_at     TIMESTAMP NOT NULL,
    hmac_scope TEXT      NOT NULL DEFAULT 'user',
    created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_webhook_outbox_due_at ON webhook_outbox (due_at);
`

const addWebhookOutboxDownSQL = `DROP TABLE IF EXISTS webhook_outbox;`

// migrationIDSessionLeases identifica a migração da tabela de posse. Nomeada
// porque três lugares a referenciam — a lista, o roteamento por dialeto e o
// teste que cobre o ramo — e literal repetido é divergência esperando.
const migrationIDSessionLeases = 14

// addSessionLeasesSQL cria a tabela de posse de sessão (ADR-0005, D2).
//
// Uma sessão do WhatsApp é STATEFUL: o socket vive dentro de um processo. Duas
// réplicas assumindo a mesma sessão não brigam em laço, como se supunha —
// medido na F89, o WhatsApp manda UM `StreamReplaced` e o perdedor fica com a
// sessão morta para sempre, sem nunca reconectar. Esta tabela é quem decide,
// antes de conectar, qual processo tem o direito.
//
// A tabela é criada nos DOIS dialetos mesmo sendo inerte em `single`: migração
// condicional por modo produziria esquemas divergentes, e uma instalação que
// migrasse de `single` para `multi` precisaria de migração retroativa.
//
// `expires_at` com fuso no Postgres (TIMESTAMPTZ) porque é comparado com
// `now()` para decidir posse — comparação de instante em fuso ambíguo aqui
// significaria duas réplicas se achando donas.
const addSessionLeasesSQL = `
CREATE TABLE IF NOT EXISTS session_leases (
    user_id    TEXT PRIMARY KEY,
    owner_id   TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_leases_expires_at ON session_leases (expires_at);
`

// addSessionLeasesSQLiteSQL é a mesma tabela sem TIMESTAMPTZ, que o SQLite não
// conhece. Fica inerte: em `single` não há posse a coordenar.
const addSessionLeasesSQLiteSQL = `
CREATE TABLE IF NOT EXISTS session_leases (
    user_id    TEXT PRIMARY KEY,
    owner_id   TEXT      NOT NULL,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_leases_expires_at ON session_leases (expires_at);
`

const addSessionLeasesDownSQL = `DROP TABLE IF EXISTS session_leases;`

// addLabelsSQL cria as três tabelas de etiqueta (F191).
//
// POR QUE TRÊS E NÃO UMA: a etiqueta em si tem nome e cor; a associação a uma
// CONVERSA e a associação a uma MENSAGEM são coisas diferentes, com chaves
// diferentes, e o WhatsApp emite um evento distinto para cada. Enfiá-las numa
// tabela só obrigaria a colunas nulas que só valem para metade das linhas.
//
// `labeled` é BOOLEAN e não "a linha existe": o evento de DESetiquetar chega
// como `labeled=false`, e apagar a linha perderia o instante em que isso
// aconteceu — que é o que distingue "nunca teve" de "tinha e tiraram".
const addLabelsSQL = `
CREATE TABLE IF NOT EXISTS wa_labels (
    user_id       TEXT        NOT NULL,
    label_id      TEXT        NOT NULL,
    name          TEXT        NOT NULL DEFAULT '',
    color         INTEGER     NOT NULL DEFAULT 0,
    predefined_id INTEGER     NOT NULL DEFAULT 0,
    deleted       BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, label_id)
);
CREATE TABLE IF NOT EXISTS wa_label_chats (
    user_id    TEXT        NOT NULL,
    label_id   TEXT        NOT NULL,
    chat_jid   TEXT        NOT NULL,
    labeled    BOOLEAN     NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, label_id, chat_jid)
);
CREATE TABLE IF NOT EXISTS wa_label_messages (
    user_id    TEXT        NOT NULL,
    label_id   TEXT        NOT NULL,
    chat_jid   TEXT        NOT NULL,
    message_id TEXT        NOT NULL,
    labeled    BOOLEAN     NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, label_id, chat_jid, message_id)
);
CREATE INDEX IF NOT EXISTS idx_wa_label_chats_user_chat ON wa_label_chats (user_id, chat_jid);
`

// addLabelsSQLiteSQL é a mesma coisa sem TIMESTAMPTZ, que o SQLite não conhece
// — mesmo motivo de addSessionLeasesSQLiteSQL.
const addLabelsSQLiteSQL = `
CREATE TABLE IF NOT EXISTS wa_labels (
    user_id       TEXT      NOT NULL,
    label_id      TEXT      NOT NULL,
    name          TEXT      NOT NULL DEFAULT '',
    color         INTEGER   NOT NULL DEFAULT 0,
    predefined_id INTEGER   NOT NULL DEFAULT 0,
    deleted       BOOLEAN   NOT NULL DEFAULT 0,
    updated_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, label_id)
);
CREATE TABLE IF NOT EXISTS wa_label_chats (
    user_id    TEXT      NOT NULL,
    label_id   TEXT      NOT NULL,
    chat_jid   TEXT      NOT NULL,
    labeled    BOOLEAN   NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, label_id, chat_jid)
);
CREATE TABLE IF NOT EXISTS wa_label_messages (
    user_id    TEXT      NOT NULL,
    label_id   TEXT      NOT NULL,
    chat_jid   TEXT      NOT NULL,
    message_id TEXT      NOT NULL,
    labeled    BOOLEAN   NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, label_id, chat_jid, message_id)
);
CREATE INDEX IF NOT EXISTS idx_wa_label_chats_user_chat ON wa_label_chats (user_id, chat_jid);
`

const addLabelsDownSQL = `
DROP TABLE IF EXISTS wa_label_messages;
DROP TABLE IF EXISTS wa_label_chats;
DROP TABLE IF EXISTS wa_labels;
`

// addSenderPushNameSQL acompanha a F84: o pushName que o WhatsApp manda em
// cada mensagem passa a ter coluna própria.
//
// Antes ele só existia dentro de `datajson`, e a lista de conversas teria de
// desserializar um JSON por linha para lê-lo — sobre 40 mil mensagens, num
// caminho de leitura. A coluna é o que torna a junção barata.
//
// NULLABLE de propósito: as linhas já gravadas não têm o nome (ele foi
// perdido na escrita, ver F84), e inventar string vazia para elas apagaria a
// distinção entre "não sabemos" e "sabemos que não tem".
const addSenderPushNameSQL = `
-- PostgreSQL version
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'message_history' AND column_name = 'sender_push_name') THEN
        ALTER TABLE message_history ADD COLUMN sender_push_name TEXT;
    END IF;
END $$;

-- SQLite version (handled in code, via addColumnIfNotExists)
`

const addSenderPushNameDownSQL = `
ALTER TABLE message_history DROP COLUMN sender_push_name;
`

// addLeaseOwnerAddrSQL guarda o ENDEREÇO do dono junto com a posse
// (ADR-0007, decisão 1).
//
// # Por que na mesma linha, e não descoberto por fora
//
// Quem for rotear precisa de duas respostas: quem é o dono, e onde ele está.
// Descobrir a segunda por DNS (StatefulSet) ou pela API do k8s são as duas
// alternativas recusadas no ADR — a primeira amarra a arquitetura a uma
// topologia, a segunda põe o plano de controle do cluster no caminho quente de
// toda requisição.
//
// Guardando aqui, as duas respostas vêm da MESMA linha, na MESMA consulta,
// escritas pela MESMA transação. Não existe caminho em que divirjam.
//
// # O endereço envelhece, e isso é tratado
//
// IP de pod é reciclado, então uma linha velha pode apontar para outro
// processo. Quem receber confere se o `owner_id` é o seu e recusa se não for;
// o chamador relê a linha. O erro é DETECTÁVEL e transitório — que é a
// propriedade que faltava nas duas alternativas.
//
// `DEFAULT ”` porque a coluna é NOT NULL e a tabela pode já ter linhas: em
// `single` ela é inerte (a posse nem é reivindicada), e vazio significa
// "não roteável", que é a leitura correta para uma linha escrita antes desta
// migração existir.
const addLeaseOwnerAddrSQL = `
ALTER TABLE session_leases ADD COLUMN owner_addr TEXT NOT NULL DEFAULT '';
`

const addLeaseOwnerAddrDownSQL = `
ALTER TABLE session_leases DROP COLUMN owner_addr;
`

// renameMessageSecretsIndexSQL acompanha a renomeação das tabelas do módulo de
// protocolo (ver renameLegacyTables em
// internal/wa-noise/persistence/store/sqlstore/legacy_rename.go). Renomear a
// tabela não renomeia seus índices, e este índice em particular é criado por
// esta camada (migração 9), não pelo módulo — por isso o rename mora aqui.
//
// É rename puro, guardado por existência: não depende de a tabela existir, só
// do índice. Em instalação nova o índice ainda não foi criado e o bloco é
// no-op. No SQLite a migração 9 nunca criou índice nenhum (applyMigration a
// trata como no-op), então esta também é no-op lá.
const renameMessageSecretsIndexSQL = `
-- PostgreSQL version
DO $$
BEGIN
	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'whatsmeow_message_secrets_message_id_idx') THEN
		ALTER INDEX whatsmeow_message_secrets_message_id_idx RENAME TO wanoise_message_secrets_message_id_idx;
	END IF;
END $$;
-- SQLite version (handled in code)
`

const renameMessageSecretsIndexDownSQL = `
DO $$
BEGIN
	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'wanoise_message_secrets_message_id_idx') THEN
		ALTER INDEX wanoise_message_secrets_message_id_idx RENAME TO whatsmeow_message_secrets_message_id_idx;
	END IF;
END $$;
`

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

const addWaNoiseMessageSecretsMessageIDIndexSQL = `
-- PostgreSQL version
DO $$
BEGIN
	CREATE INDEX IF NOT EXISTS wanoise_message_secrets_message_id_idx
	ON wanoise_message_secrets (message_id);
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
	} else if migration.ID == migrationIDBlankPlaintextToken {
		err = applyBlankPlaintextTokenMigration(tx)
	} else if migration.ID == 12 {
		if db.DriverName() == "sqlite" {
			// A migração 9 nunca criou o índice no SQLite, então não há o que
			// renomear aqui.
			err = nil
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == 13 {
		if db.DriverName() == "sqlite" {
			// Sem DEFAULT: NULL distingue "linha antiga, nao sabemos o nome"
			// de "sabemos que nao tem" (F84).
			err = addColumnIfNotExistsSQLite(tx, "message_history", "sender_push_name", "TEXT")
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == migrationIDSessionLeases {
		if db.DriverName() == "sqlite" {
			_, err = tx.Exec(addSessionLeasesSQLiteSQL)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == migrationIDLabels {
		if db.DriverName() == "sqlite" {
			_, err = tx.Exec(addLabelsSQLiteSQL)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == migrationIDWebhookOutbox {
		if db.DriverName() == "sqlite" {
			_, err = tx.Exec(addWebhookOutboxSQLiteSQL)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
	} else if migration.ID == migrationIDAccountOwnership {
		if db.DriverName() == "sqlite" {
			_, err = tx.Exec(accountOwnershipSQLiteSQL)
		} else {
			_, err = tx.Exec(migration.UpSQL)
		}
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

// ErrTokenHashBackfillIncomplete é devolvido pela migração 16 quando alguma
// linha continuaria sem `token_hash` depois do preenchimento.
//
// Apagar o texto claro dessas linhas tiraria delas a ÚNICA forma de autenticar,
// e o valor não existe em nenhum outro lugar — não há como desfazer. Abortar
// deixa o processo sem subir, com esta mensagem; continuar deixaria um usuário
// sem acesso e sem diagnóstico.
var ErrTokenHashBackfillIncomplete = errors.New(
	"migration 16 (blank_plaintext_token) aborted: some users would be left without token_hash; " +
		"blanking the plaintext token would remove their only way to authenticate")

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

// applyBlankPlaintextTokenMigration apaga o token em texto claro das linhas
// existentes (F97 etapa 2).
//
// A F97 etapa 1 fez o INSERT parar de gravar o texto claro, mas isso só vale
// para linhas NOVAS. As antigas continuam com a credencial legível em disco —
// e é ela que aparece num backup, numa réplica ou num dump de suporte.
//
// # A ordem, que é o que torna isto seguro
//
// Preenche o hash que faltar, VERIFICA que não sobrou ninguém sem hash, e só
// então apaga. A verificação não é zelo: apagar o texto claro de uma linha sem
// hash tira dela a única forma de autenticar, e não há como desfazer — o valor
// não existe em nenhum outro lugar.
//
// Por isso a migração ABORTA em vez de continuar. Abortar deixa o processo sem
// subir, com mensagem; continuar deixaria um usuário sem acesso e sem
// diagnóstico.
//
// # O que NÃO é feito aqui
//
// A coluna não é dropada. Ela é NOT NULL, dropar exige reconstruir a tabela no
// SQLite, e o ganho é cosmético: com todo valor vazio, o segredo já saiu do
// disco. Fica como limpeza posterior, não como parte desta mudança.
func applyBlankPlaintextTokenMigration(tx *sqlx.Tx) error {
	var pendentes []struct {
		ID    string `db:"id"`
		Token string `db:"token"`
	}
	if err := tx.Select(&pendentes,
		`SELECT id, token FROM users WHERE token <> '' AND (token_hash IS NULL OR token_hash = '')`); err != nil {
		log.Error().Err(err).Str("table", "users").Str("query", "select_rows_without_hash").
			Msg("failed to read rows missing token_hash")
		return fmt.Errorf("failed to read rows missing token_hash: %w", err)
	}

	updateSQL := tx.Rebind("UPDATE users SET token_hash = ? WHERE id = ?")
	for _, linha := range pendentes {
		if _, err := tx.Exec(updateSQL, domain.HashToken(linha.Token), linha.ID); err != nil {
			log.Error().Err(err).Str("table", "users").Str("user_id", linha.ID).
				Msg("failed to backfill token_hash")
			return fmt.Errorf("failed to backfill token_hash for user %s: %w", linha.ID, err)
		}
	}

	// Relê do BANCO em vez de confiar no laço acima: o que importa é o estado
	// que vai ser destruído, não o que o código acha que fez.
	//
	// HOJE esta verificação é inalcançável, e isso foi MEDIDO, não suposto: o
	// único jeito conhecido de uma linha sobreviver ao preenchimento é o UPDATE
	// falhar, e aí a função já retornou erro acima. Desligar esta checagem não
	// muda o resultado de nenhum teste — o controle negativo confirmou.
	//
	// Fica assim mesmo. É defesa em profundidade sobre uma operação
	// IRREVERSÍVEL: o dia em que o preenchimento ganhar um caminho que engole
	// falha por linha (um `continue` num laço, por exemplo), esta é a única
	// coisa entre isso e um usuário sem acesso.
	var semHash int
	if err := tx.Get(&semHash,
		`SELECT COUNT(*) FROM users WHERE token <> '' AND (token_hash IS NULL OR token_hash = '')`); err != nil {
		log.Error().Err(err).Str("table", "users").Str("query", "verify_no_row_without_hash").
			Msg("failed to verify token_hash backfill")
		return fmt.Errorf("failed to verify token_hash backfill: %w", err)
	}
	if semHash > 0 {
		log.Error().Str("table", "users").Int("rows", semHash).
			Msg("migration 16 aborted: rows would lose their only way to authenticate")
		return fmt.Errorf("%w (%d row(s) still without token_hash)", ErrTokenHashBackfillIncomplete, semHash)
	}

	res, err := tx.Exec(`UPDATE users SET token = '' WHERE token <> ''`)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("column", "token").
			Msg("failed to blank the plaintext token")
		return fmt.Errorf("failed to blank the plaintext token: %w", err)
	}
	if apagadas, err := res.RowsAffected(); err == nil {
		log.Info().Str("table", "users").Int64("rows", apagadas).
			Msg("plaintext API tokens removed from storage")
	}

	return nil
}
