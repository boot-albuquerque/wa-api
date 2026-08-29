package status

import (
	"context"
	"net/http"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const fetchAudioMaxBytes int64 = 16 * 1024 * 1024

const dataAudioPrefix = "data:audio/"

// PublishStatusAudioUseCase publishes an audio clip as an ephemeral status story.
type PublishStatusAudioUseCase struct {
	media   appport.MediaMessenger
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

func NewPublishStatusAudioUseCase(mm appport.MediaMessenger, mf appport.MediaFetcher, l appport.Logger) *PublishStatusAudioUseCase {
	return &PublishStatusAudioUseCase{media: mm, fetcher: mf, logger: l}
}

func (uc *PublishStatusAudioUseCase) Execute(ctx context.Context, txtID string, req domain.PublishStatusAudioRequest) (*domain.PublishStatusAudioResult, error) {
	if req.Audio == "" {
		return nil, apperr.New("missing_audio", apperr.CategoryValidation, "missing Audio in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	var data []byte
	var detectedMime string

	switch {
	case isDataURIAudio(req.Audio):
		var err error
		data, detectedMime, err = uc.decodeDataURIAudio(ctx, txtID, req.Audio)
		if err != nil {
			return nil, err
		}

	case isHTTPURL(req.Audio):
		var contentType string
		var err error
		data, contentType, err = uc.fetcher.FetchBytes(ctx, req.Audio, fetchAudioMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch audio url", "txtID", txtID, "error", err)
			return nil, apperr.New("audio_fetch_failed", apperr.CategoryValidation, "failed to fetch audio from url", false, err)
		}
		if strings.HasPrefix(strings.ToLower(contentType), "audio/") {
			detectedMime = contentType
		}

	default:
		return nil, apperr.New("unsupported_audio_source", apperr.CategoryValidation,
			"audio must be base64 (data:audio/) or valid HTTP URL", false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_audio_body", apperr.CategoryValidation, "audio body is empty", false, nil)
	}

	mimeType := resolveStatusAudioMimeType(req.MimeType, detectedMime, data)
	payload := domain.AudioPayload{Bytes: data, MimeType: mimeType}

	sent, err := uc.media.SendAudio(ctx, txtID, domain.StatusBroadcastJID, payload, nil, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to publish status audio", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "status audio published", "msgID", sent.ID)
	return &domain.PublishStatusAudioResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}, nil
}

func resolveStatusAudioMimeType(reqMimeType, detectedMime string, data []byte) string {
	switch {
	case reqMimeType != "":
		return reqMimeType
	case detectedMime != "":
		return detectedMime
	}
	if sniffed := http.DetectContentType(data); sniffed != "application/octet-stream" {
		return sniffed
	}
	return "audio/mpeg"
}

func isDataURIAudio(raw string) bool {
	return len(raw) >= len(dataAudioPrefix) && raw[:len(dataAudioPrefix)] == dataAudioPrefix
}

func (uc *PublishStatusAudioUseCase) decodeDataURIAudio(ctx context.Context, txtID, raw string) ([]byte, string, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > fetchAudioMaxBytes {
				uc.logger.Warn(ctx, "data uri audio payload exceeds size limit before decode", "txtID", txtID, "decodedBytes", n, "limit", fetchAudioMaxBytes)
				return nil, "", apperr.New("audio_too_large", apperr.CategoryValidation,
					"decoded audio payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		uc.logger.Warn(ctx, "failed to decode data uri audio", "txtID", txtID, "error", err)
		return nil, "", apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > fetchAudioMaxBytes {
		uc.logger.Warn(ctx, "data uri audio payload exceeds size limit after decode", "txtID", txtID, "decodedBytes", len(decoded.Data), "limit", fetchAudioMaxBytes)
		return nil, "", apperr.New("audio_too_large", apperr.CategoryValidation,
			"decoded audio payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, decoded.ContentType(), nil
}
