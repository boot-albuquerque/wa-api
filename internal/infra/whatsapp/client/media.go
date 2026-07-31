package client

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
)

// MediaConfig holds per-user S3/media delivery settings.
type MediaConfig struct {
	Enabled       string
	MediaDelivery string
}

// Download timeouts.
const (
	DownloadTimeoutImage    = 60
	DownloadTimeoutAudio    = 60
	DownloadTimeoutDocument = 60
	DownloadTimeoutVideo    = 180
	DownloadTimeoutSticker  = 15
)

// ProcessMedia downloads WhatsApp media and optionally uploads to S3,
// appending results to postmap. Was root media.go:28-106.
func (c *MyClient) ProcessMedia(
	media whatsmeow.DownloadableMessage,
	mimeType, ext string,
	timeoutSec int,
	isIncoming bool,
	chatJID, msgID string,
	s3cfg MediaConfig,
	postmap map[string]interface{},
	extra map[string]interface{},
) {
	if c.Ctx.SkipMedia {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	data, err := c.Client.Download(ctx, media)
	if err != nil {
		log.Error().Err(err).Str("msgID", msgID).Msg("media download failed")
		return
	}

	if s3cfg.Enabled == "true" && (s3cfg.MediaDelivery == "s3" || s3cfg.MediaDelivery == "both") {
		c.Ctx.EnsureS3(c.UserID)
		fn := "file" + ext
		if extra != nil {
			if efn, ok := extra["fileName"]; ok {
				fn = efn.(string)
			}
		}
		s3Data, err := c.Ctx.ProcessS3(ctx, c.UserID, chatJID, msgID, data, mimeType, fn, isIncoming)
		if err != nil {
			log.Error().Err(err).Msg("S3 upload failed")
		} else {
			postmap["s3"] = s3Data
		}
	} else {
		postmap["s3"] = map[string]interface{}{"url": ""}
	}

	if s3cfg.MediaDelivery == "s3" {
		return
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	postmap["mediaBase64"] = "data:" + mimeType + ";base64," + b64
	if extra != nil {
		for k, v := range extra {
			postmap[k] = v
		}
	}
}
