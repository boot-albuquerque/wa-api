package media

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	wamgr "wuzapi/internal/infrastructure/whatsmeow"
)

const (
	DownloadTimeoutImage    = 2 * time.Minute
	DownloadTimeoutAudio    = 5 * time.Minute
	DownloadTimeoutDocument = 10 * time.Minute
	DownloadTimeoutVideo    = 10 * time.Minute
	DownloadTimeoutSticker  = 1 * time.Minute
)

type MediaS3Config struct {
	Enabled       string
	MediaDelivery string
}

// MyClient is an alias to the whatsmeow package's MyClient interface
type MyClient = wamgr.MyClient

// S3Manager interface for S3 operations
type S3Manager interface {
	ProcessMediaForS3(ctx context.Context, userID, chatJID, messageID string, data []byte, mimeType, fileName string, isIncoming bool) (map[string]interface{}, error)
}

// ProcessMediaHandler holds the dependencies for media processing
type ProcessMediaHandler struct {
	S3Manager        S3Manager
	FileToBase64Func func(filepath string) (string, string, error)
}

var defaultHandler *ProcessMediaHandler

// SetProcessMediaHandler sets the global handler for media processing
func SetProcessMediaHandler(handler *ProcessMediaHandler) {
	defaultHandler = handler
}

func ProcessMedia(
	mycli MyClient,
	msg whatsmeow.DownloadableMessage,
	mimeType string,
	fallbackExt string,
	timeout time.Duration,
	isIncoming bool,
	chatJID string,
	messageID string,
	s3cfg MediaS3Config,
	postmap map[string]interface{},
	extraKeys map[string]interface{},
) {
	tmpDir := filepath.Join("/tmp", "user_"+mycli.GetUserID())
	if err := os.MkdirAll(tmpDir, 0751); err != nil {
		log.Error().Err(err).Msg("Could not create temporary directory")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	data, err := mycli.GetWAClient().Download(ctx, msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to download media")
		return
	}

	ext := fallbackExt
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		ext = exts[0]
	}
	tmpPath := filepath.Join(tmpDir, messageID+ext)

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Error().Err(err).Msg("Failed to save media to temporary file")
		return
	}
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			log.Error().Err(err).Msg("Failed to delete temporary file")
		} else {
			log.Info().Str("path", tmpPath).Msg("Temporary file deleted")
		}
	}()

	if defaultHandler != nil {
		if s3cfg.Enabled == "true" && (s3cfg.MediaDelivery == "s3" || s3cfg.MediaDelivery == "both") {
			if defaultHandler.S3Manager != nil {
				s3Data, err := defaultHandler.S3Manager.ProcessMediaForS3(
					ctx,
					mycli.GetUserID(),
					chatJID,
					messageID,
					data,
					mimeType,
					filepath.Base(tmpPath),
					isIncoming,
				)
				if err != nil {
					log.Error().Err(err).Msg("Failed to upload media to S3")
				} else {
					postmap["s3"] = s3Data
				}
			}
		}

		if s3cfg.MediaDelivery == "base64" || s3cfg.MediaDelivery == "both" {
			if defaultHandler.FileToBase64Func != nil {
				b64, mime_, err := defaultHandler.FileToBase64Func(tmpPath)
				if err != nil {
					log.Error().Err(err).Msg("Failed to convert media to base64")
					return
				}
				postmap["base64"] = b64
				postmap["mimeType"] = mime_
				postmap["fileName"] = filepath.Base(tmpPath)
			}
		}
	}

	for k, v := range extraKeys {
		postmap[k] = v
	}

	log.Info().Str("path", tmpPath).Msg("Media processed")
}
