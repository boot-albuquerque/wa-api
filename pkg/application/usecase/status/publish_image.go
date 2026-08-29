package status

import (
	"context"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const fetchImageMaxBytes int64 = 16 * 1024 * 1024

const dataImagePrefix = "data:image"

// PublishStatusImageUseCase publishes an image as an ephemeral status story.
// The image is uploaded and sent to StatusBroadcastJID; noise resolves the
// recipient list from the account's privacy settings (core/broadcast.go).
type PublishStatusImageUseCase struct {
	media   appport.MediaMessenger
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

func NewPublishStatusImageUseCase(mm appport.MediaMessenger, mf appport.MediaFetcher, l appport.Logger) *PublishStatusImageUseCase {
	return &PublishStatusImageUseCase{media: mm, fetcher: mf, logger: l}
}

func (uc *PublishStatusImageUseCase) Execute(ctx context.Context, txtID string, req domain.PublishStatusImageRequest) (*domain.PublishStatusImageResult, error) {
	if req.Image == "" {
		return nil, apperr.New("missing_image", apperr.CategoryValidation, "missing Image in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	var data []byte
	var mimeType string

	switch {
	case isDataURIImage(req.Image):
		var err error
		data, err = uc.decodeDataURIImage(ctx, txtID, req.Image)
		if err != nil {
			return nil, err
		}
		mimeType = resolveMimeType(req.MimeType, data)

	case isHTTPURL(req.Image):
		var err error
		data, _, err = uc.fetcher.FetchBytes(ctx, req.Image, fetchImageMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch image url", "txtID", txtID, "error", err)
			return nil, apperr.New("image_fetch_failed", apperr.CategoryValidation, "failed to fetch image from url", false, err)
		}
		mimeType = resolveMimeType(req.MimeType, data)

	default:
		return nil, apperr.New("unsupported_image_source", apperr.CategoryValidation,
			`Image data should start with "data:image/png;base64," or be an http(s) URL`, false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_image_body", apperr.CategoryValidation, "image body is empty", false, nil)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, apperr.New("invalid_image_mime_type", apperr.CategoryValidation, "resolved MIME type is not an image type", false, nil)
	}

	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption, JPEGThumbnail: req.JPEGThumbnail}

	sent, err := uc.media.SendImage(ctx, txtID, domain.StatusBroadcastJID, payload, nil, nil, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to publish status image", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "status image published", "msgID", sent.ID)
	return &domain.PublishStatusImageResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}, nil
}

func isDataURIImage(raw string) bool {
	return len(raw) >= len(dataImagePrefix) && raw[:len(dataImagePrefix)] == dataImagePrefix
}

func (uc *PublishStatusImageUseCase) decodeDataURIImage(ctx context.Context, txtID, raw string) ([]byte, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > fetchImageMaxBytes {
				uc.logger.Warn(ctx, "data uri image payload exceeds size limit before decode", "txtID", txtID, "decodedBytes", n, "limit", fetchImageMaxBytes)
				return nil, apperr.New("image_too_large", apperr.CategoryValidation,
					"decoded image payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		uc.logger.Warn(ctx, "failed to decode data uri image", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > fetchImageMaxBytes {
		uc.logger.Warn(ctx, "data uri image payload exceeds size limit after decode", "txtID", txtID, "decodedBytes", len(decoded.Data), "limit", fetchImageMaxBytes)
		return nil, apperr.New("image_too_large", apperr.CategoryValidation,
			"decoded image payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, nil
}
