package bootstrap

import (
	"context"

	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"wa-api/internal/wa-noise/persistence/store/sqlstore"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	appsession "wa-api/pkg/application/session"
	dbmig "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
	"wa-api/pkg/infra/wa-noise/observability/walog"
)

// ServerMode represents the server operating mode
type ServerMode int

const (
	HTTP ServerMode = iota
	Stdio
)

type server struct {
	DB                  *sqlx.DB
	Router              *mux.Router
	ExPath              string
	Mode                ServerMode
	SessionOrchestrator *appsession.Orchestrator

	// Leases e' nil no modo `single`, e isso NAO e' omissao: com um processo
	// so' nao ha posse a coordenar, e a trava de instancia do D1 ja' garante
	// exclusividade. Ver buildLeaseManager.
	Leases *leaseManager
}

const version = Version

// resolveLogLevel traduz o valor de -loglevel para o nivel global do zerolog,
// e diz se ele foi reconhecido. Comportamento identico ao bloco que vivia
// inline em Main(); virou funcao para que a correcao da Fase 4b passe a ter
// teste, que era o unico jeito de torna-la permanente.
//
// Ate esta regra existir, nao havia SetGlobalLevel em lugar nenhum do repo, e
// o default implicito do zerolog (TraceLevel) mandava todo Debug para
// producao — a causa raiz do desequilibrio Debug-vs-Error da auditoria da
// Fase 4b.
//
// O detalhe nao-obvio: zerolog.ParseLevel("") devolve NoLevel com erro NIL.
// Tratar so o erro deixaria o valor vazio DESLIGAR a filtragem inteira —
// exatamente o defeito que esta regra existe para consertar. Por isso o vazio
// cai no fallback junto com o valor invalido.
func resolveLogLevel(raw string) (zerolog.Level, bool) {
	if raw == "" {
		return zerolog.InfoLevel, false
	}
	lvl, err := zerolog.ParseLevel(strings.ToLower(raw))
	if err != nil {
		return zerolog.InfoLevel, false
	}
	return lvl, true
}

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

	// Check for log level in environment variable if flag is default or empty
	if *logLevel == "info" || *logLevel == "" {
		if v := os.Getenv("WA_API_LOG_LEVEL"); v != "" {
			*logLevel = v
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

	// Global log level. Ver resolveLogLevel para a regra e o porquê dela.
	lvl, recognized := resolveLogLevel(*logLevel)
	if !recognized {
		log.Warn().Str("loglevel", *logLevel).Msg("Unrecognized log level, falling back to info")
	}
	zerolog.SetGlobalLevel(lvl)

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

	// Global encryption key: flag, else environment, else the process REFUSES to
	// start. Why it is not generated — and why silencing the old log line alone
	// would have made things worse — lives in startup_secrets.go.
	//
	// The admin token is resolved further down, once the data directory is
	// known: a generated token is written to a file inside it.
	resolvedEncryptionKey, _, err := resolveGlobalEncryptionKey(*globalEncryptionKey, os.Getenv(envGlobalEncryptionKey))
	if err != nil {
		log.Fatal().Err(err).Msg("could not resolve the global encryption key")
	}
	*globalEncryptionKey = resolvedEncryptionKey

	// Check for global webhook in environment variable
	if *globalWebhook == "" {
		if v := os.Getenv("WA_API_GLOBAL_WEBHOOK"); v != "" {
			*globalWebhook = v
			log.Info().Str("global_webhook", v).Msg("Global webhook configured from environment variable")
		}
	} else {
		log.Info().Str("global_webhook", *globalWebhook).Msg("Global webhook configured from command line")
	}

	// Global HMAC key: flag, else environment, else generated. The rules — and
	// why the value never reaches a log line — live in global_hmac_key.go.
	resolvedHMACKey, _, errHMAC := resolveGlobalHMACKey(*globalHMACKey, os.Getenv(envGlobalHMACKey))
	if errHMAC != nil {
		log.Fatal().Err(errHMAC).
			Msg("could not resolve the global HMAC key: the entropy source failed")
	}
	*globalHMACKey = resolvedHMACKey

	// Seed the AppContext with runtime config so functions in wmiau.go
	// and helpers.go can access global state without raw globals.
	// GlobalEncryptionKey must be set before encryptHMACKey below, which reads it via appCtx.
	appCtx.GlobalWebhook = *globalWebhook
	appCtx.GlobalWebhookUseProxy = *globalWebhookUseProxy
	appCtx.GlobalEncryptionKey = *globalEncryptionKey
	appCtx.WebhookRetryEnabled = *webhookRetryEnabled
	appCtx.WebhookRetryCount = *webhookRetryCount
	appCtx.WebhookRetryDelaySeconds = *webhookRetryDelaySeconds
	appCtx.WebhookErrorQueueName = *webhookErrorQueueName

	// Falha aqui é FATAL, e não um Error que se ignora (F67 item 2).
	//
	// Com a chave vazia, callHookWithHmac pula a assinatura em silêncio —
	// `if len(encryptedHmacKey) > 0` (dispatch_callhook.go:80) — e todo
	// webhook global sai SEM assinatura. O operador que configurou uma chave
	// HMAC fez isso justamente para que fossem assinados; entregar sem
	// assinatura acreditando que estão assinados é rebaixamento silencioso de
	// segurança, e o receptor não tem como perceber a diferença.
	//
	// Não há caso legítimo de seguir adiante: *globalHMACKey nunca chega aqui
	// vazio (resolveGlobalHMACKey gera uma quando nenhuma é fornecida),
	// então a assinatura é sempre pretendida. A única falha possível é
	// WA_API_GLOBAL_ENCRYPTION_KEY inválida — configuração, corrigível, e que
	// o operador precisa ver antes de o serviço atender requisição.
	//
	// Mesmo tratamento que os.Executable() logo abaixo já recebia.
	globalHMACKeyEncrypted, err = encryptHMACKey(*globalHMACKey)
	if err != nil {
		log.Fatal().Err(err).
			Msg("não foi possível cifrar a chave HMAC global: webhooks sairiam SEM assinatura. " +
				"Verifique WA_API_GLOBAL_ENCRYPTION_KEY — o AES aceita 16, 24 ou 32 bytes")
	}
	log.Info().Msg("Global HMAC key encrypted successfully")
	appCtx.GlobalHMACKeyEncrypted = globalHMACKeyEncrypted

	InitRabbitMQ()

	ex, err := os.Executable()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get executable path")
		panic(err)
	}
	exPath := filepath.Dir(ex)

	// Modo de cluster ANTES de abrir o banco (ADR-0005, D1): uma configuração
	// impossível — `multi` sem Postgres, ou um segundo processo sobre o mesmo
	// diretório de dados — tem de morrer sem ter tocado em estado nenhum.
	dirDados := exPath
	if *dataDir != "" {
		dirDados = *dataDir
	}
	tipoBanco := getDatabaseConfig(exPath, *dataDir).Type
	modoCluster, liberarCluster, err := prepareCluster(dirDados, tipoBanco)
	if err != nil {
		log.Fatal().Err(err).Msg("configuracao de cluster invalida")
		os.Exit(1)
	}
	defer liberarCluster()

	// Admin token: flag, else environment, else generated. It is resolved HERE,
	// and not next to the encryption key above, because a generated token is
	// written to a file inside the data directory — and dirDados is only known
	// after the cluster block resolved it. See startup_secrets.go for why the
	// token goes to a 0600 file instead of a log line.
	resolvedAdminToken, _, errAdmin := resolveAdminToken(*adminToken, os.Getenv(envAdminToken), dirDados)
	if errAdmin != nil {
		log.Fatal().Err(errAdmin).Msg("could not resolve the admin token")
	}
	*adminToken = resolvedAdminToken

	// Relatório de capacidades (ADR-0005 D7, F104): declara o que este processo
	// pode e não pode fazer, em UMA linha, antes de qualquer outra coisa
	// acontecer. O que ele existe para evitar é o operador ter de DERIVAR
	// "isto não aguenta dois pods" a partir de uma variável de ambiente
	// ausente e de um tipo de banco que ninguém declarou.
	publishCapabilities(modoCluster, tipoBanco)

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
	// And the key that unwraps the stored S3 secret (ADR-0009). Without it the
	// lazy initialization fails closed instead of using the envelope as a
	// credential.
	storage.GetS3Manager().SetEncryptionKey(appCtx.GlobalEncryptionKey)

	// Nunca nil e nunca um logger nulo: Warn e Error do sqlstore saem sempre.
	// --wadebug apenas baixa o piso (ver walog.ParseLevel).
	dbLog := walog.New(log.Logger, walog.ModuleDatabase, walog.ParseLevel(*waDebug))

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
	s.SessionOrchestrator = newSessionOrchestrator(s)
	initCustomHandlers(s)
	s.routes()

	// A posse tem de existir ANTES do connectOnStartup: e' ela que decide
	// quais sessoes este processo pode assumir (ADR-0005 D2).
	setupSessionOwnership(s)

	s.connectOnStartup()

	// O heartbeat so' comeca DEPOIS do connectOnStartup: antes disso nao ha
	// posse registrada para renovar, e um tick vazio no meio da subida so'
	// produziria consulta inutil.
	leaseCtx, pararLeases := context.WithCancel(context.Background())
	defer pararLeases()
	startLeaseHeartbeat(leaseCtx, s.Leases)

	// Varredura do outbox (ADR-0005 D3). Depois do connectOnStartup pelo mesmo
	// motivo do heartbeat: antes disso nao ha cliente HTTP provisionado para
	// nenhuma sessao, e a retomada so encontraria entregas que nao tem como
	// entregar — gastando tentativas do orcamento a toa.
	//
	// NAO existe caminho separado de "carregar pendentes na subida": uma linha
	// deixada por um processo morto ja esta com o prazo vencido, entao a
	// primeira varredura a pega. Dois mecanismos para o mesmo trabalho e o
	// dobro das chances de divergir.
	setupWebhookOutbox(s)
	startOutboxSweeper(leaseCtx)

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

				// AQUI, e nunca num `defer`: os.Exit abaixo nao executa
				// funcoes adiadas. Um release adiado nunca rodaria, e o
				// sintoma seria "as vezes a sessao demora 15s para voltar" —
				// creditado a' rede, e nao a um defer que nao disparou.
				releaseLeasesOnShutdown(s.Leases)

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
