package status

import (
	"context"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const fetchVideoMaxBytes int64 = 100 * 1024 * 1024

const dataPrefix = "data"

// PublishStatusVideoUseCase publishes a video as an ephemeral status story.
type PublishStatusVideoUseCase struct {
	media   appport.MediaMessenger
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

func NewPublishStatusVideoUseCase(mm appport.MediaMessenger, mf appport.MediaFetcher, l appport.Logger) *PublishStatusVideoUseCase {
	return &PublishStatusVideoUseCase{media: mm, fetcher: mf, logger: l}
}

func (uc *PublishStatusVideoUseCase) Execute(ctx context.Context, txtID string, req domain.PublishStatusVideoRequest) (*domain.PublishStatusVideoResult, error) {
	if req.Video == "" {
		return nil, apperr.New("missing_video", apperr.CategoryValidation, "missing Video in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	var data []byte

	switch {
	case isDataVideo(req.Video):
		var err error
		data, err = uc.decodeDataVideo(ctx, txtID, req.Video)
		if err != nil {
			return nil, err
		}

	case isHTTPURL(req.Video):
		var err error
		data, _, err = uc.fetcher.FetchBytes(ctx, req.Video, fetchVideoMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch video url", "txtID", txtID, "error", err)
			return nil, apperr.New("video_fetch_failed", apperr.CategoryValidation, "failed to fetch video from url", false, err)
		}

	default:
		return nil, apperr.New("unsupported_video_source", apperr.CategoryValidation,
			`data should start with "data:mime/type;base64,"`, false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_video_body", apperr.CategoryValidation, "video body is empty", false, nil)
	}

	mimeType := resolveMimeType(req.MimeType, data)
	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption, JPEGThumbnail: req.JPEGThumbnail}

	sent, err := uc.media.SendVideo(ctx, txtID, domain.StatusBroadcastJID, payload, nil, nil, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to publish status video", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "status video published", "msgID", sent.ID)
	return &domain.PublishStatusVideoResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}, nil
}

func isDataVideo(raw string) bool {
	return len(raw) >= len(dataPrefix) && raw[:len(dataPrefix)] == dataPrefix
}

func (uc *PublishStatusVideoUseCase) decodeDataVideo(ctx context.Context, txtID, raw string) ([]byte, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > fetchVideoMaxBytes {
				uc.logger.Warn(ctx, "data uri video payload exceeds size limit before decode", "txtID", txtID, "decodedBytes", n, "limit", fetchVideoMaxBytes)
				return nil, apperr.New("video_too_large", apperr.CategoryValidation,
					"decoded video payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		uc.logger.Warn(ctx, "failed to decode data uri video", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > fetchVideoMaxBytes {
		uc.logger.Warn(ctx, "data uri video payload exceeds size limit after decode", "txtID", txtID, "decodedBytes", len(decoded.Data), "limit", fetchVideoMaxBytes)
		return nil, apperr.New("video_too_large", apperr.CategoryValidation,
			"decoded video payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, nil
}
