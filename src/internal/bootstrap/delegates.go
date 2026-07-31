package bootstrap

import (
	"context"
	"io"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waclient "wa-api/internal/infra/whatsapp/client"
	"wa-api/internal/infra/constants"
	"wa-api/internal/infra/db"
	wahistory "wa-api/internal/infra/history"
	"wa-api/internal/infra/messaging"
	stdiopkg "wa-api/internal/infra/stdio"
	intwhatsmeow "wa-api/internal/infra/whatsmeow"
	mwpkg "wa-api/internal/presentation/http/middleware"
)

// ── DB ──
type DatabaseConfig = db.DatabaseConfig
type HistoryMessage = db.HistoryMessage
var InitializeDatabase = db.InitializeDatabase
var getDatabaseConfig = db.GetDatabaseConfig
var saveMessageToHistory = db.SaveMessageToHistory
var trimMessageHistory = db.TrimMessageHistory
var setDisconnectedState = db.SetDisconnectedState

// ── Constants ──
var supportedEventTypes = constants.SupportedEventTypes

// ── Middleware ──
type Values = mwpkg.Values
var respondJSON = mwpkg.RespondJSON
var authAdmin = mwpkg.AuthAdmin
var authAlice = mwpkg.AuthAlice
func resolveConnectEvents(subscribe []string, existing string) (string, bool) {
	return mwpkg.ResolveConnectEvents(Find, supportedEventTypes, subscribe, existing)
}

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
func (mycli *MyClient) processMedia(
	msg whatsmeow.DownloadableMessage, mimeType, fallbackExt string, timeout time.Duration,
	isIncoming bool, chatJID, messageID string, s3cfg mediaS3Config,
	postmap map[string]interface{}, extraKeys map[string]interface{},
) {
	cc := &waclient.MyClient{Client: mycli.WAClient, UserID: mycli.UserID, Token: mycli.Token}
	cc.ProcessMedia(msg, mimeType, fallbackExt, int(timeout.Seconds()),
		isIncoming, chatJID, messageID,
		waclient.MediaConfig{Enabled: s3cfg.Enabled, MediaDelivery: s3cfg.MediaDelivery},
		postmap, extraKeys)
}

// ── RabbitMQ ──
type WebhookFileErrorPayload = messaging.WebhookFileErrorPayload
type WebhookErrorPayload = messaging.WebhookErrorPayload
var PublishToRabbit = messaging.PublishToRabbit
var PublishFileErrorToQueue = messaging.PublishFileErrorToQueue
var PublishDataErrorToQueue = messaging.PublishDataErrorToQueue
func InitRabbitMQ() { messaging.SetupDependencies(appCtx.UserInfoCache, webhookErrorQueueName); messaging.InitRabbitMQ() }
func handleConnectionErrors() { messaging.HandleConnectionErrors() }
func sendToGlobalRabbit(jsonData []byte, token, userID string, queueName ...string) { messaging.SendToGlobalRabbit(jsonData, token, userID, queueName...) }

// ── Stdio ──
type stdioServer = stdiopkg.Server
func NewStdioServer(s *server) *stdioServer { return stdiopkg.NewServer(s.Router) }
func newStdioServerWithIO(s *server, stdin io.Reader, stdout io.Writer) *stdioServer { return stdiopkg.NewServerWithIO(s.Router, stdin, stdout) }
func (s *server) SendNotification(method string, params map[string]interface{}) { stdiopkg.SendNotification(method, params) }

// ── History ──
func syncHistoryForChat(ctx context.Context, db *sqlx.DB, userID string, chatJID types.JID, count int) error {
	return wahistory.SyncHistoryForChat(ctx, db, wahistory.SyncDeps{
		GetWA: func(uid string) interface{} { return clientManager.GetWhatsmeowClient(uid) },
		GetMC: func(uid string) interface{} { return clientManager.GetMyClient(uid) },
	}, userID, chatJID, count)
}
func saveOutgoingMessageToHistory(db *sqlx.DB, userID, chatJID, messageID, messageType, textContent, mediaLink string, historyLimit int) {
	if historyLimit > 0 {
		err := saveMessageToHistory(db, userID, chatJID, "me", messageID, messageType, textContent, mediaLink, "", "")
		if err != nil { log.Error().Err(err).Msg("Failed to save outgoing msg") } else {
			if err := trimMessageHistory(db, userID, chatJID, historyLimit); err != nil { log.Error().Err(err).Msg("Failed to trim history") }
		}
	}
}
