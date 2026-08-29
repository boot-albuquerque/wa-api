package errmap

import (
	"errors"

	"wa-api/internal/noise/capabilities/media"
	"wa-api/pkg/domain/apperr"
)

// Error codes returned to the API consumer.
const (
	CodeMediaUnavailable = "media_unavailable"
)

// ClassifyDownload translates CDN-level refusals from the WhatsApp media
// servers into the appropriate apperr category. Without this, a 403 or
// 404 from the CDN surfaces as our 500 — telling the caller we broke,
// when the media is simply gone (F254).
//
// Measured codes (2026-08-25, HOUSEKEEP F254):
//
//	403 → media no longer available on CDN (expired or purged)
//	404 → media not found on CDN
//	410 → media explicitly gone
//
// Only the codes above are mapped. Other CDN statuses pass through
// unchanged — we do not invent mappings for codes we have not observed.
func ClassifyDownload(err error) error {
	if err == nil {
		return nil
	}

	var dhe media.DownloadHTTPError
	if !errors.As(err, &dhe) {
		return err
	}

	switch dhe.StatusCode {
	case 403, 404, 410:
		return apperr.New(
			CodeMediaUnavailable,
			apperr.CategoryNotFound,
			"media is no longer available; it may have expired on the WhatsApp CDN",
			false,
			err,
		)
	default:
		return err
	}
}
