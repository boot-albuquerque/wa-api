package bootstrap

import (
	"context"

	"flag"
	"fmt"
	"math/rand"

	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	dbmig "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
)

// ServerMode represents the server operating mode
type ServerMode int

const (
	HTTP ServerMode = iota
	Stdio
)

type server struct {
	DB     *sqlx.DB
	Router *mux.Router
	ExPath string
	Mode   ServerMode
}

const version = Version

// killchannel helpers now delegate to appCtx.KillChannel (internal/app).
// The raw sync.Mutex and map have been migrated to KillChannel struct.
func Main() {
	InitPrivateIPBlocks()
	err := godotenv.Load()
	if err != nil {
		log.Warn().Err(err).Msg("It was not possible to load the .env file (it may not exist).")
	}

	flag.Parse()

	// Check for address in environment variable if flag is default or empty
	if *address == "0.0.0.0" || *address == "" {
		if v := os.Getenv("WA_API_ADDRESS"); v != "" {
			*address = v
			log.Info().Str("address", v).Msg("Address configured from environment variable")
		}
	}

	// Check for port in environment variable if flag is default or empty
	if *port == "8080" || *port == "" {
		if v := os.Getenv("WA_API_PORT"); v != "" {
			*port = v
			log.Info().Str("port", v).Msg("Port configured from environment variable")
		}
	}

	if v := os.Getenv("WEBHOOK_RETRY_ENABLED"); v != "" {
		*webhookRetryEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("WEBHOOK_RETRY_COUNT"); v != "" {
		if count, err := strconv.Atoi(v); err == nil {
			*webhookRetryCount = count
		}
	}
	if v := os.Getenv("WEBHOOK_RETRY_DELAY_SECONDS"); v != "" {
		if delay, err := strconv.Atoi(v); err == nil {
			*webhookRetryDelaySeconds = delay
		}
	}
	if v := os.Getenv("WEBHOOK_ERROR_QUEUE_NAME"); v != "" {
		*webhookErrorQueueName = v
	}
	if v := os.Getenv("WA_API_WEBHOOK_USE_PROXY"); v != "" {
		*globalWebhookUseProxy = strings.ToLower(v) == "true" || v == "1"
	}

	log.Info().
		Bool("use_proxy", *globalWebhookUseProxy).
		Msg("Webhook Proxy Configured")

	log.Info().
		Bool("enabled", *webhookRetryEnabled).
		Int("count", *webhookRetryCount).
		Int("delay", *webhookRetryDelaySeconds).
		Str("queue", *webhookErrorQueueName).
		Msg("Webhook Retry Configured")

	// Novo bloco para sobrescrever o osName pelo ENV, se existir
	if v := os.Getenv("SESSION_DEVICE_NAME"); v != "" {
		*osName = v
	}

	// Override platformType from environment variable if set
	if v := os.Getenv("SESSION_PLATFORM_TYPE"); v != "" {
		*platformType = v
	}

	if *versionFlag {
		fmt.Printf("WuzAPI version %s\n", version)
		os.Exit(0)
	}

	// In stdio mode, always log to stderr to avoid interfering with JSON responses on stdout
	logOutput := os.Stdout
	if *mode == "stdio" {
		logOutput = os.Stderr
	}

	if *logType == "json" {
		log.Logger = zerolog.New(logOutput).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{
			Out:        logOutput,
			TimeFormat: "2006-01-02 15:04:05 -07:00",
			NoColor:    !*colorOutput,
		}

		output.FormatLevel = func(i interface{}) string {
			if i == nil {
				return ""
			}
			lvl := strings.ToUpper(i.(string))
			switch lvl {
			case "DEBUG":
				return "\x1b[34m" + lvl + "\x1b[0m"
			case "INFO":
				return "\x1b[32m" + lvl + "\x1b[0m"
			case "WARN":
				return "\x1b[33m" + lvl + "\x1b[0m"
			case "ERROR", "FATAL", "PANIC":
				return "\x1b[31m" + lvl + "\x1b[0m"
			default:
				return lvl
			}
		}

		log.Logger = zerolog.New(output).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Logger()
	}

	// Setup timezone (after logger is configured)
	tz := os.Getenv("TZ")
	if tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			log.Warn().Err(err).Msgf("It was not possible to define TZ=%q, using UTC", tz)
		} else {
			time.Local = loc
			log.Info().Str("TZ", tz).Msg("Timezone defined")
		}
	}

	if *adminToken == "" {
		if v := os.Getenv("WA_API_ADMIN_TOKEN"); v != "" {
			*adminToken = v
		} else {
			// Generate a random token if none provided
			const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			b := make([]byte, 32)
			for i := range b {
				b[i] = charset[rand.Intn(len(charset))]
			}
			*adminToken = string(b)
			log.Warn().Str("admin_token", *adminToken).Msg("No admin token provided, generated a random one")
		}
	}

	if *globalEncryptionKey == "" {
		if v := os.Getenv("WA_API_GLOBAL_ENCRYPTION_KEY"); v != "" {
			*globalEncryptionKey = v
			log.Info().Msg("Encryption key loaded from environment variable")
		} else {
			// Generate a random key if none provided
			const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			b := make([]byte, 32)
			for i := range b {
				b[i] = charset[rand.Intn(len(charset))]
			}
			*globalEncryptionKey = string(b)
			log.Warn().Str("global_encryption_key", *globalEncryptionKey).Msg("No WA_API_GLOBAL_ENCRYPTION_KEY provided, generated a random one. " +
				"SAVE THIS KEY TO YOUR .ENV FILE OR ALL ENCRYPTED DATA WILL BE LOST ON RESTART!")
		}
	}

	// Check for global webhook in environment variable
	if *globalWebhook == "" {
		if v := os.Getenv("WA_API_GLOBAL_WEBHOOK"); v != "" {
			*globalWebhook = v
			log.Info().Str("global_webhook", v).Msg("Global webhook configured from environment variable")
		}
	} else {
		log.Info().Str("global_webhook", *globalWebhook).Msg("Global webhook configured from command line")
	}

	// Check for global HMAC key in environment variable
	if *globalHMACKey == "" {
		if v := os.Getenv("WA_API_GLOBAL_HMAC_KEY"); v != "" {
			*globalHMACKey = v
			log.Info().Msg("Global HMAC key configured from environment variable")
		} else {
			// Generate a random key if none provided
			const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			b := make([]byte, 32)
			for i := range b {
				b[i] = charset[rand.Intn(len(charset))]
			}
			*globalHMACKey = string(b)
			log.Warn().Str("global_hmac_key", *globalHMACKey).Msg("No WA_API_GLOBAL_HMAC_KEY provided, generated a random one")
		}

	} else {
		log.Info().Msg("Global HMAC key configured from command line")
	}

	globalHMACKeyEncrypted, err = encryptHMACKey(*globalHMACKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt global HMAC key")
	} else {
		log.Info().Msg("Global HMAC key encrypted successfully")
	}

	InitRabbitMQ()

	// Seed the AppContext with runtime config so functions in wmiau.go
	// and helpers.go can access global state without raw globals.
	appCtx.GlobalWebhook = *globalWebhook
	appCtx.GlobalHMACKeyEncrypted = globalHMACKeyEncrypted
	appCtx.GlobalWebhookUseProxy = *globalWebhookUseProxy
	appCtx.GlobalEncryptionKey = *globalEncryptionKey
	appCtx.WebhookRetryEnabled = *webhookRetryEnabled
	appCtx.WebhookRetryCount = *webhookRetryCount
	appCtx.WebhookRetryDelaySeconds = *webhookRetryDelaySeconds
	appCtx.WebhookErrorQueueName = *webhookErrorQueueName

	ex, err := os.Executable()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get executable path")
		panic(err)
	}
	exPath := filepath.Dir(ex)

	db, err := InitializeDatabase(exPath, *dataDir)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
		os.Exit(1)
	}
	// Defer cleanup of the database connection
	defer func() {
		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close database connection")
		}
	}()

	// Set DB reference in S3Manager for lazy client initialization
	storage.GetS3Manager().SetDB(db)

	var dbLog waLog.Logger
	if *waDebug != "" {
		dbLog = waLog.Stdout("Database", *waDebug, *colorOutput)
	}

	// Get database configuration
	config := getDatabaseConfig(exPath, *dataDir)
	var storeConnStr string
	if config.Type == "postgres" {
		storeConnStr = fmt.Sprintf(
			"user=%s password=%s dbname=%s host=%s port=%s sslmode=%s",
			config.User, config.Password, config.Name, config.Host, config.Port, config.SSLMode,
		)
		container, err = sqlstore.New(context.Background(), "postgres", storeConnStr, dbLog)
	} else {
		storeConnStr = "file:" + filepath.ToSlash(filepath.Join(config.Path, "main.db")) + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
		container, err = sqlstore.New(context.Background(), "sqlite", storeConnStr, dbLog)
	}

	if err != nil {
		log.Fatal().Err(err).Msg("Error creating sqlstore")
		os.Exit(1)
	}

	// Initialize the schema
	if err = dbmig.InitializeSchema(db); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize schema")
		// Perform cleanup before exiting
		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close database connection during cleanup")
		}
		os.Exit(1)
	}

	serverMode := HTTP
	if *mode == "stdio" {
		serverMode = Stdio
	}

	s := &server{
		Router: mux.NewRouter(),
		DB:     db,
		ExPath: exPath,
		Mode:   serverMode,
	}
	initCustomHandlers(s)
	s.routes()

	s.connectOnStartup()

	if serverMode == Stdio {
		startStdioMode(s)
	} else {
		startHTTPMode(s)
	}
}

func startHTTPMode(s *server) {
	srv := &http.Server{
		Addr:              *address + ":" + *port,
		Handler:           s.Router,
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       180 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	var once sync.Once

	// Wait for signals in a separate goroutine
	go func() {
		for {
			<-done
			once.Do(func() {
				log.Warn().Msg("Stopping server...")

				// Graceful shutdown logic
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := srv.Shutdown(ctx); err != nil {
					log.Error().Err(err).Msg("Failed to stop server")
					os.Exit(1)
				}

				log.Info().Msg("Server Exited Properly")
				os.Exit(0)
			})
		}
	}()

	go func() {
		if *sslcert != "" {

			if *sslcert != "" && *sslprivkey != "" {
				if _, err := os.Stat(*sslcert); os.IsNotExist(err) {
					log.Fatal().Err(err).Msg("SSL certificate file does not exist")
				}
				if _, err := os.Stat(*sslprivkey); os.IsNotExist(err) {
					log.Fatal().Err(err).Msg("SSL private key file does not exist")
				}
			}
			if err := srv.ListenAndServeTLS(*sslcert, *sslprivkey); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("HTTPS server failed to start")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("HTTP server failed to start")
			}
		}
	}()
	log.Info().Str("address", *address).Str("port", *port).Msg("Server started. Waiting for connections...")
	select {}
}

func startStdioMode(s *server) {
	stdioServer := NewStdioServer(s)
	if err := stdioServer.Start(); err != nil {
		log.Error().Err(err).Msg("Stdio server error")
		os.Exit(1)
	}
	log.Info().Msg("Stdio server exited properly")
}
