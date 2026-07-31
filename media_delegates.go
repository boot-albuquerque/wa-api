package wuzapi

import (
	"time"

	"go.mau.fi/whatsmeow"
	waclient "wuzapi/internal/whatsapp/client"
)

// mediaS3Config holds S3 settings (used by myEventHandler in wmiau.go).
type mediaS3Config struct {
	Enabled       string
	MediaDelivery string
}

// Media download timeout constants — delegated to internal/whatsapp/client.
const (
	downloadTimeoutImage    = 2 * time.Minute
	downloadTimeoutAudio    = 5 * time.Minute
	downloadTimeoutDocument = 10 * time.Minute
	downloadTimeoutVideo    = 10 * time.Minute
	downloadTimeoutSticker  = 1 * time.Minute
)

// processMedia delegates to client.ProcessMedia.
func (mycli *MyClient) processMedia(
	msg whatsmeow.DownloadableMessage,
	mimeType, fallbackExt string,
	timeout time.Duration,
	isIncoming bool,
	chatJID, messageID string,
	s3cfg mediaS3Config,
	postmap map[string]interface{},
	extraKeys map[string]interface{},
) {
	cc := &waclient.MyClient{
		Client: mycli.WAClient,
		UserID: mycli.userID,
		Token:  mycli.token,
	}
	cc.ProcessMedia(msg, mimeType, fallbackExt, int(timeout.Seconds()),
		isIncoming, chatJID, messageID,
		waclient.MediaConfig{Enabled: s3cfg.Enabled, MediaDelivery: s3cfg.MediaDelivery},
		postmap, extraKeys)
}
