package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

type DatabaseConfig struct {
	Type     string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	Path     string
	SSLMode  string
}

func InitializeDatabase(exPath, dataDirFlag string) (*sqlx.DB, error) {
	config := GetDatabaseConfig(exPath, dataDirFlag)

	if config.Type == "postgres" {
		db, err := initializePostgres(config)
		if err != nil {
			log.Error().Err(err).Str("db_type", "postgres").Str("db_name", config.Name).
				Str("host", config.Host).Msg("failed to initialize database")
			return nil, err
		}
		return db, nil
	}

	db, err := initializeSQLite(config)
	if err != nil {
		log.Error().Err(err).Str("db_type", "sqlite").Str("path", config.Path).
			Msg("failed to initialize database")
		return nil, err
	}
	return db, nil
}

func GetDatabaseConfig(exPath, dataDirFlag string) DatabaseConfig {
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbSSL := os.Getenv("DB_SSLMODE")

	sslMode := dbSSL
	switch dbSSL {
	case "true":
		sslMode = "require"
	case "false", "":
		sslMode = "disable"
	}

	if dbUser != "" && dbPassword != "" && dbName != "" && dbHost != "" && dbPort != "" {
		return DatabaseConfig{
			Type:     "postgres",
			Host:     dbHost,
			Port:     dbPort,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
			SSLMode:  sslMode,
		}
	}

	// Um subconjunto não vazio das variáveis de Postgres, mas incompleto, é
	// quase sempre erro de configuração: o processo cai silenciosamente para
	// SQLite e o operador só descobre quando os dados não estão onde espera.
	if dbUser != "" || dbPassword != "" || dbName != "" || dbHost != "" || dbPort != "" {
		log.Warn().Str("db_type", "sqlite").Str("reason", "incomplete_postgres_env").
			Msg("falling back to sqlite: DB_USER/DB_PASSWORD/DB_NAME/DB_HOST/DB_PORT partially set")
	}

	// Use datadir flag if provided, otherwise fall back to executable directory
	dataPath := exPath
	if dataDirFlag != "" {
		dataPath = dataDirFlag
	}

	return DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(dataPath, "dbdata"),
	}
}

func initializePostgres(config DatabaseConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=%s",
		config.User, config.Password, config.Name, config.Host, config.Port, config.SSLMode,
	)

	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		log.Error().Err(err).Str("db_type", "postgres").Str("db_name", config.Name).
			Str("host", config.Host).Str("port", config.Port).
			Msg("failed to open postgres connection")
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		log.Error().Err(err).Str("db_type", "postgres").Str("db_name", config.Name).
			Str("host", config.Host).Str("port", config.Port).
			Msg("failed to ping postgres database")
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	return db, nil
}

func initializeSQLite(config DatabaseConfig) (*sqlx.DB, error) {
	if err := os.MkdirAll(config.Path, 0751); err != nil {
		log.Error().Err(err).Str("db_type", "sqlite").Str("path", config.Path).
			Msg("could not create dbdata directory")
		return nil, fmt.Errorf("could not create dbdata directory: %w", err)
	}

	dbPath := filepath.ToSlash(filepath.Join(config.Path, "users.db"))
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)")
	if err != nil {
		log.Error().Err(err).Str("db_type", "sqlite").Str("path", dbPath).
			Msg("failed to open sqlite database")
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		log.Error().Err(err).Str("db_type", "sqlite").Str("path", dbPath).
			Msg("failed to ping sqlite database")
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	return db, nil
}

type HistoryMessage struct {
	ID              int       `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	ChatJID         string    `json:"chat_jid" db:"chat_jid"`
	SenderJID       string    `json:"sender_jid" db:"sender_jid"`
	MessageID       string    `json:"message_id" db:"message_id"`
	Timestamp       time.Time `json:"timestamp" db:"timestamp"`
	MessageType     string    `json:"message_type" db:"message_type"`
	TextContent     string    `json:"text_content" db:"text_content"`
	MediaLink       string    `json:"media_link" db:"media_link"`
	QuotedMessageID string    `json:"quoted_message_id,omitempty" db:"quoted_message_id"`
	DataJson        string    `json:"data_json" db:"datajson"`
}

// SetDisconnectedState marks a user disconnected. Events kept by default;
// reset only when clearEvents is true (issue #305).
func SetDisconnectedState(db *sqlx.DB, txtid string, clearEvents bool) error {
	if clearEvents {
		if _, err := db.Exec("UPDATE users SET connected=0,events=$1 WHERE id=$2", "", txtid); err != nil {
			log.Error().Err(err).Str("table", "users").Str("user_id", txtid).
				Bool("clear_events", true).Msg("failed to set disconnected state")
			return err
		}
		return nil
	}
	if _, err := db.Exec("UPDATE users SET connected=0 WHERE id=$1", txtid); err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", txtid).
			Bool("clear_events", false).Msg("failed to set disconnected state")
		return err
	}
	return nil
}
