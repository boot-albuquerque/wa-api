package errmap

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"

	"wa-api/internal/noise"
	"wa-api/pkg/domain/apperr"
)

// Error codes for upstream info query refusals. One constant per outcome the
// caller can act on differently — a bare string repeated in two places is the
// same bug waiting to diverge (ADR-0004).
const (
	CodeUpstreamRejected     = "upstream_rejected"
	CodeUpstreamForbidden    = "upstream_forbidden"
	CodeUpstreamUnauthorized = "upstream_unauthorized"
	CodeUpstreamNotFound     = "upstream_not_found"
	CodeUpstreamRateLimited  = "upstream_rate_limited"
	CodeUpstreamUnavailable  = "upstream_unavailable"
)

// ClassifyIQ translates a WhatsApp server refusal into the apperr vocabulary,
// leaving anything else untouched.
//
// It exists because the refusal reached the caller as 500 internal server
// error: measured on POST /user/block with a well formed number that has no
// WhatsApp account, where the server answered 400 bad-request and we answered
// "we broke, retry" (F204). The caller retries forever and the answer never
// changes.
//
// This is the facade's job and nowhere else's: the sentinels live in the
// vendored fork, which ADR-001 forbids the adapters from importing. Translating
// here is what lets every adapter above stay ignorant of the fork's error
// vocabulary.
//
// Errors that are NOT info query refusals pass through unchanged, including
// nil. A translator that wrapped everything would turn transport failures and
// our own bugs into upstream refusals, which is the same class of lie in the
// opposite direction.
func ClassifyIQ(err error) error {
	if err == nil {
		return nil
	}

	var iq *noise.IQError
	if !errors.As(err, &iq) {
		return err
	}

	code, category := classifyIQCode(iq.Code)

	// Debug, e não Warn: a falha em si já é registada por quem chama (o
	// adaptador loga "Failed to block user"), e duplicar seria ruído. O que
	// esta linha acrescenta é a DECISÃO da tradução — qual status de montante
	// virou qual categoria nossa. É o que se quer ver quando um cliente
	// reclama de um 422 e é preciso saber se o mapa está certo.
	log.Debug().Int("upstreamStatus", iq.Code).Str("upstreamText", iq.Text).
		Str("category", string(category)).Str("code", code).
		Msg("translated WhatsApp info query refusal")

	return apperr.New(code, category, iqMessage(iq.Code), category == apperr.CategoryRateLimited, err)
}

// classifyIQCode maps the upstream status to our vocabulary.
//
// The 5xx family stays CategoryInternal on purpose: those are genuine upstream
// FAILURES, not refusals, and "retry later" is the honest advice. Only the 4xx
// family is a refusal the caller could act on.
func classifyIQCode(code int) (string, apperr.Category) {
	switch code {
	case http.StatusUnauthorized:
		return CodeUpstreamUnauthorized, apperr.CategoryUnauthorized
	case http.StatusForbidden:
		return CodeUpstreamForbidden, apperr.CategoryForbidden
	case http.StatusNotFound, http.StatusGone:
		return CodeUpstreamNotFound, apperr.CategoryNotFound
	case http.StatusTooManyRequests, iqStatusResourceLimit:
		return CodeUpstreamRateLimited, apperr.CategoryRateLimited
	case http.StatusInternalServerError, http.StatusServiceUnavailable, iqStatusPartialServerError:
		return CodeUpstreamUnavailable, apperr.CategoryInternal
	default:
		// 400 bad-request, 405 not-allowed, 406 not-acceptable, 423 locked, and
		// anything the server adds later. All of them are "understood and
		// refused", which is exactly what 422 says.
		return CodeUpstreamRejected, apperr.CategoryUpstreamRejected
	}
}

// Status codes the WhatsApp server uses that net/http has no constant for.
const (
	iqStatusResourceLimit      = 419
	iqStatusPartialServerError = 530
)

// iqMessage is what the CLIENT reads. It must be safe to return by
// construction (see pkg/domain/apperr): no internal detail, no PII, no echo of
// the JID that was refused.
func iqMessage(code int) string {
	switch code {
	case http.StatusUnauthorized:
		return "WhatsApp rejected this session's credentials for that request"
	case http.StatusForbidden:
		return "WhatsApp does not permit this operation on that target"
	case http.StatusNotFound, http.StatusGone:
		return "WhatsApp has no such target"
	case http.StatusTooManyRequests, iqStatusResourceLimit:
		return "WhatsApp is rate limiting this session; retry later"
	case http.StatusInternalServerError, http.StatusServiceUnavailable, iqStatusPartialServerError:
		return "WhatsApp failed to answer this request; retry later"
	default:
		return "WhatsApp refused this request; repeating it unchanged will not help"
	}
}
