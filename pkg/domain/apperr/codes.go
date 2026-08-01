package apperr

import "net/http"

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
	case CategoryInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
