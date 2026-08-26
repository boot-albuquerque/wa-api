package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/rs/zerolog/hlog"
)

// The unknown-field policy of the HTTP boundary (F268).
//
// Detection lives in domain.DecodeRequestWithUnknownFields; what to DO about a
// field the route does not know is a boundary decision, and it is made here so
// that every route makes the same one.
//
// Two levels, and the default is the lenient one on purpose:
//
//   - default: the request is accepted exactly as before, and a WARN names the
//     fields that were dropped. Nothing observable changes for a client; what
//     changes is that the silence becomes diagnosable.
//   - strict: the request is refused with 400 unknown_field, naming the
//     fields. This CHANGES the observable contract — bodies that work today
//     start failing — so it is opt-in and never inferred.
const (
	// EnvStrictUnknownFields turns the warning into a refusal. Opt-in, like
	// devui.EnvEnabled: whoever forgets to set it keeps the behaviour their
	// clients already depend on, which is the right direction for the error.
	EnvStrictUnknownFields = "WA_API_STRICT_UNKNOWN_FIELDS"

	// CodeUnknownField is the error code of the strict refusal. A code of its
	// own, and not CodeDecodePayload: the body decoded fine, it just carried a
	// name this route does not know — and telling those two apart is the whole
	// point of answering at all.
	CodeUnknownField = "unknown_field"

	// unknownFieldMessagePrefix opens the message that names the offending
	// keys. Shared by the log line and the HTTP body so the two cannot drift.
	unknownFieldMessagePrefix = "unknown field "

	// logFieldRoute correlates the record with the route, as the rest of this
	// package does.
	logFieldRoute = "route"

	// logFieldUnknownFields carries the names as a structured array, for
	// whoever queries logs instead of reading them.
	logFieldUnknownFields = "unknown_fields"
)

// errUnknownFieldAnswered tells the caller of decodeRequest that the strict
// policy has ALREADY written the response.
//
// It exists because the ~48 decode sites of this package each log their own
// cause, in their own words, and several tests assert those exact words. A
// helper that answered and then let the site answer again would write two
// bodies; a helper that unified the logging would rewrite assertions that were
// deliberate. So the site keeps its rejection path untouched and only learns
// to step aside when the response is already out.
var errUnknownFieldAnswered = errors.New("unknown field in request body: already answered")

// requestAnswered reports whether err means the response is already written,
// and therefore that the caller must return WITHOUT logging or responding.
func requestAnswered(err error) bool { return errors.Is(err, errUnknownFieldAnswered) }

// strictUnknownFields reports whether unknown fields must be refused.
//
// Read on each request rather than resolved once, matching devui.Enabled():
// the cost is a map lookup, and a value read where it is used cannot go stale
// against a process whose environment changed.
func strictUnknownFields() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvStrictUnknownFields))) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

// unknownFieldMessage renders the names the way both the log and the HTTP body
// show them: `unknown field "duration"`. Quoted, because a bare name is
// unreadable when the typo is a space or an empty string.
func unknownFieldMessage(fields []string) string {
	quoted := make([]string, len(fields))
	for i, field := range fields {
		quoted[i] = strconv.Quote(field)
	}
	return unknownFieldMessagePrefix + strings.Join(quoted, ", ")
}

// unknownFieldError builds the strict-mode refusal, naming the fields. Built
// per request rather than kept as a sentinel because the names ARE the
// message — an error saying only "unknown field" would leave the caller
// exactly where the silence left them.
func unknownFieldError(fields []string) *apperr.AppError {
	return &apperr.AppError{
		Code:     CodeUnknownField,
		Category: apperr.CategoryValidation,
		Message:  unknownFieldMessage(fields),
	}
}

// decodeRequest decodes the JSON body into dst and applies the unknown-field
// policy. It is the drop-in replacement for domain.DecodeRequest at the HTTP
// boundary: the decode error it returns is the SAME one, so every call site
// keeps its own rejection log and its own response.
//
// The one new obligation on the caller is to check requestAnswered(err) first:
// in strict mode this function answers the request itself, and the site must
// then return in silence.
func decodeRequest(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	unknown, err := domain.DecodeRequestWithUnknownFields(r.Body, dst)
	if err != nil {
		return err
	}
	if len(unknown) == 0 {
		return nil
	}

	if strictUnknownFields() {
		appErr := unknownFieldError(unknown)
		hlog.FromRequest(r).Warn().
			Err(appErr).
			Str(logFieldRoute, r.URL.Path).
			Strs(logFieldUnknownFields, unknown).
			Msg(unknownFieldMessage(unknown))
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, appErr)
		return errUnknownFieldAnswered
	}

	hlog.FromRequest(r).Warn().
		Str(logFieldRoute, r.URL.Path).
		Strs(logFieldUnknownFields, unknown).
		Msg(unknownFieldMessage(unknown))
	return nil
}
