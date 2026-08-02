package bootstrap

import (
	"context"
	"time"

	"wa-api/pkg/infra/constants"
	"wa-api/pkg/infra/db"
	wahistory "wa-api/pkg/infra/history"
	"wa-api/pkg/infra/media"
	"wa-api/pkg/infra/messaging"
	stdiopkg "wa-api/pkg/infra/stdio"
	"wa-api/pkg/infra/storage"
	intwhatsmeow "wa-api/pkg/infra/whatsmeow"
	mwpkg "wa-api/pkg/presentation/http/middleware"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ── DB ──
type DatabaseConfig = db.DatabaseConfig
type HistoryMessage = db.HistoryMessage

var InitializeDatabase = db.InitializeDatabase
var getDatabaseConfig = db.GetDatabaseConfig
var saveMessageToHistory = db.SaveMessageToHistory
var trimMessageHistory = db.TrimMessageHistory

// ── Constants ──
var supportedEventTypes = constants.SupportedEventTypes

// ── Middleware ──
type Values = mwpkg.Values

var authAdmin = mwpkg.AuthAdmin
var authAlice = mwpkg.AuthAlice

// ── Clients ──
type ClientManager = intwhatsmeow.ClientManager

var NewClientManager = intwhatsmeow.NewClientManager

// ── Media ──
const (
	downloadTimeoutImage    = 2 * time.Minute
	downloadTimeoutAudio    = 5 * time.Minute
	downloadTimeoutDocument = 10 * time.Minute
	downloadTimeoutVideo    = 10 * time.Minute
	downloadTimeoutSticker  = 1 * time.Minute
)

type mediaS3Config struct {
	Enabled       string
	MediaDelivery string
}

// GetUserID / GetWAClient make *MyClient satisfy media.MyClient (= whatsmeow.MyClient),
// the interface pkg/infra/media.ProcessMedia consumes.
func (mycli *MyClient) GetUserID() string              { return mycli.UserID }
func (mycli *MyClient) GetWAClient() *whatsmeow.Client { return mycli.WAClient }

var _ media.MyClient = (*MyClient)(nil)

// s3MediaUploader adapts storage.S3Manager to media.S3Manager, preserving the
// lazy per-user client init that the previous media path performed via EnsureS3.
type s3MediaUploader struct{}

func (s3MediaUploader) ProcessMediaForS3(
	ctx context.Context, userID, chatJID, messageID string,
	data []byte, mimeType, fileName string, isIncoming bool,
) (map[string]interface{}, error) {
	storage.EnsureS3ClientForUser(userID)
	return storage.GetS3Manager().ProcessMediaForS3(
		ctx, userID, chatJID, messageID, data, mimeType, fileName, isIncoming)
}

func init() {
	media.SetProcessMediaHandler(&media.ProcessMediaHandler{
		S3Manager:        s3MediaUploader{},
		FileToBase64Func: media.FileToBase64,
	})
}

func (mycli *MyClient) processMedia(
	msg whatsmeow.DownloadableMessage, mimeType, fallbackExt string, timeout time.Duration,
	isIncoming bool, chatJID, messageID string, s3cfg mediaS3Config,
	postmap map[string]interface{}, extraKeys map[string]interface{},
) {
	if mycli.WAClient == nil {
		log.Warn().
			Str("userID", mycli.UserID).
			Str("messageID", messageID).
			Msg("media processing skipped: WhatsApp client not configured")
		return
	}
	media.ProcessMedia(mycli, msg, mimeType, fallbackExt, timeout,
		isIncoming, chatJID, messageID,
		media.MediaS3Config{Enabled: s3cfg.Enabled, MediaDelivery: s3cfg.MediaDelivery},
		postmap, extraKeys)
}

// ── RabbitMQ ──
type WebhookFileErrorPayload = messaging.WebhookFileErrorPayload
type WebhookErrorPayload = messaging.WebhookErrorPayload

var PublishToRabbit = messaging.PublishToRabbit
var PublishFileErrorToQueue = messaging.PublishFileErrorToQueue
var PublishDataErrorToQueue = messaging.PublishDataErrorToQueue

func InitRabbitMQ() {
	messaging.SetupDependencies(appCtx.UserInfoCache, webhookErrorQueueName)
	messaging.InitRabbitMQ()
}
func sendToGlobalRabbit(jsonData []byte, token, userID string, queueName ...string) {
	messaging.SendToGlobalRabbit(jsonData, token, userID, queueName...)
}

// ── Stdio ──
type stdioServer = stdiopkg.Server

func NewStdioServer(s *server) *stdioServer { return stdiopkg.NewServer(s.Router) }
func (s *server) SendNotification(method string, params map[string]interface{}) {
	stdiopkg.SendNotification(method, params)
}

// ── History ──
func syncHistoryForChat(ctx context.Context, db *sqlx.DB, userID string, chatJID types.JID, count int) error {
	return wahistory.SyncHistoryForChat(ctx, db, wahistory.SyncDeps{
		GetWA: func(uid string) interface{} { return clientManager.GetWhatsmeowClient(uid) },
		GetMC: func(uid string) interface{} { return clientManager.GetMyClient(uid) },
	}, userID, chatJID, count)
}
