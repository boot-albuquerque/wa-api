package bootstrap

import (
	"flag"
	"time"

	"github.com/patrickmn/go-cache"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

// Replace the global variables
var (
	address = flag.String("address", "0.0.0.0", "Bind IP Address")
	port    = flag.String("port", "8080", "Listen Port")
	waDebug = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType = flag.String("logtype", "console", "Type of log output (console or json)")
	// logLevel defaults to info deliberately. Before this flag existed there
	// was no SetGlobalLevel call anywhere in the repo, so zerolog's implicit
	// TraceLevel default left DEBUG on in production. Restore the old volume
	// with -loglevel=debug or WA_API_LOG_LEVEL=debug.
	logLevel            = flag.String("loglevel", "info", "Log level (debug, info, warn, error)")
	skipMedia           = flag.Bool("skipmedia", false, "Do not attempt to download media in messages")
	osName              = flag.String("osname", "Mac OS 10", "Connection OSName in Whatsapp")
	platformType        = flag.String("platformtype", "DESKTOP", "Device platform type (DESKTOP, IPAD, ANDROID_TABLET, IOS_PHONE, ANDROID_PHONE, etc.)")
	colorOutput         = flag.Bool("color", false, "Enable colored output for console logs")
	sslcert             = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey          = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	adminToken          = flag.String("admintoken", "", "Security Token to authorize admin actions (list/create/remove users)")
	globalEncryptionKey = flag.String("globalencryptionkey", "", "Encryption key for sensitive data (32 bytes)")
	globalHMACKey       = flag.String("globalhmackey", "", "Global HMAC key for webhook signing")
	globalWebhook       = flag.String("globalwebhook", "", "Global webhook URL to receive all events from all users")
	versionFlag         = flag.Bool("version", false, "Display version information and exit")
	mode                = flag.String("mode", "http", "Server mode: http or stdio")
	dataDir             = flag.String("datadir", "", "Data directory for database and session files (defaults to executable directory)")

	globalHMACKeyEncrypted []byte

	webhookRetryEnabled      = flag.Bool("webhookretry", true, "Enable webhook retry mechanism")
	webhookRetryCount        = flag.Int("retrycount", 5, "Number of times to retry failed webhooks")
	webhookRetryDelaySeconds = flag.Int("retrydelay", 30, "Delay in seconds between webhook retries")
	webhookErrorQueueName    = flag.String("errorqueue", "webhook_errors", "RabbitMQ queue name for failed webhooks")
	globalWebhookUseProxy    = flag.Bool("webhookuseproxy", true, "Route webhook deliveries through the per-user proxy when configured")

	container     *sqlstore.Container
	clientManager = NewClientManager()
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
)

// appCtx bundles runtime configuration previously spread across global vars.
// Functions in wmiau.go, handlers.go, and helpers.go access it for webhook
// dispatch, caching, and session lifecycle orchestration.
var appCtx = NewAppContext()

// Kill channel helpers delegate to appCtx.KillChannel.
func setKillChannel(uid string, ch chan bool)     { appCtx.KillChannel.Set(uid, ch) }
func getKillChannel(uid string) (chan bool, bool) { return appCtx.KillChannel.Get(uid) }
func deleteKillChannel(uid string, ch chan bool)  { appCtx.KillChannel.Delete(uid, ch) }
func signalKill(uid string)                       { appCtx.KillChannel.Signal(uid) }
