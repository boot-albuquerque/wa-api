package apperr

import (
	"context"
	"errors"
	"net/http"
)

// Category is a broad error class that the HTTP boundary maps to a status
// code. The set below is derived from a census of every statusCode actually
// passed to RespondJSON across the presentation layer (305 call sites, see
// the Fase 3 PR body for the raw counts) — not from theory. Only categories
// backed by an observed status make the cut; anything else needs a written
// justification before it's added.
//
//	200 OK                  -> CategoryNone (not an error)
//	400 Bad Request  (93x)  -> CategoryValidation
//	401 Unauthorized (37x)  -> CategoryUnauthorized
//	500 Internal     (92x)  -> CategoryInternal
type Category string

const (
	// CategoryValidation covers client-supplied input that fails validation:
	// malformed request bodies, missing required fields, invalid parameters.
	CategoryValidation Category = "validation"

	// CategoryUnauthorized covers authentication/authorization failures:
	// missing, invalid, or insufficiently privileged credentials.
	CategoryUnauthorized Category = "unauthorized"

	// CategoryConflict covers requests that are well formed and authorized but
	// cannot be served in the CURRENT state, or by THIS instance.
	//
	// It exists because the three categories above forced a wrong answer twice
	// (F95). Both cases are the same shape: the caller did nothing wrong, and
	// repeating the identical request against the same target yields the same
	// result — but changing the target or the state makes it succeed.
	//
	//   - session owned by another replica: route the request to its owner
	//   - logout of a disconnected session: reconnect first, then retry
	//
	// CategoryValidation (400) tells the caller "fix your payload", which is
	// actively misleading here: there is nothing to fix in the payload.
	CategoryConflict Category = "conflict"

	// CategoryInternal covers everything the caller cannot fix by changing
	// their request: downstream failures, bugs, unexpected state.
	CategoryInternal Category = "internal"
)

// HTTPStatus maps a Category to the HTTP status code it corresponds to.
// The table exists and is tested here, but nothing calls it yet — Fase 4a
// is the one that wires the HTTP boundary to it.
func (c Category) HTTPStatus() int {
	switch c {
	case CategoryValidation:
		return http.StatusBadRequest
	case CategoryUnauthorized:
		return http.StatusUnauthorized
	case CategoryConflict:
		return http.StatusConflict
	case CategoryInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// IsClientGaveUp reports whether err is the CLIENT abandoning the request —
// a closed browser tab, a navigation, a cancelled fetch.
//
// It exists to keep those out of `error` level. Logging them as errors makes a
// routine tab switch indistinguishable from the database being down: F90 was
// filed after fifteen consecutive `error` lines reading
// "database error: context canceled", all of them a browser cancelling the dev
// panel's polling while the process stayed perfectly healthy. In an
// environment that alerts on log level, a page refresh pages someone at night.
//
// It deliberately does NOT match context.DeadlineExceeded. That one is OUR
// timeout expiring — we promised an answer within a budget and failed to
// deliver — and it stays an error.
func IsClientGaveUp(err error) bool {
	return errors.Is(err, context.Canceled)
}
