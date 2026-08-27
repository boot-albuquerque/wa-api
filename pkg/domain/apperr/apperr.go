// Package apperr defines the domain error type that gives call sites a
// code, a category, and a retryable flag to base HTTP-boundary decisions
// on.
//
// The package doc used to say "nothing in the repository constructs or
// consumes AppError yet". That has not been true for some time, and a stale
// doc here is expensive: the same claim was repeated in a HOUSEKEEP entry and
// produced a plan for work that was already half done (see the note on
// Category.HTTPStatus, which had exactly this defect). Measured 2026-08-27:
//
//   - CONSTRUCTED at ~285 call sites under pkg/, across
//     pkg/application/usecase (the validation guards of the message, group and
//     chat families), pkg/infra and pkg/presentation/http/handlers;
//   - CONSUMED by pkg/presentation/http.RespondJSON, which derives BOTH the
//     HTTP status (from Category.HTTPStatus) and the {"code","message"} object
//     from it.
//
// The incremental migration is of the REMAINDER: roughly 305 fmt.Errorf and
// errors.New call sites under pkg/ are still untyped. Most of them are not
// HTTP-boundary errors at all (configuration parsing in pkg/bootstrap,
// database plumbing in pkg/infra/db), and those are fine untyped — what
// RespondJSON needs typed is the error a client can act on. Every literal
// code, wherever it is constructed, is held to the canonical snake_case rule
// by TestErrorCodesAreCanonicalSnakeCase in
// pkg/presentation/http/handlers/error_code_contract_test.go.
package apperr

import (
	"errors"

	"github.com/rs/zerolog/log"
)

// AppError is a domain error carrying enough structure for the HTTP
// boundary to make a decision without inspecting message text.
type AppError struct {
	// Code identifies the specific error condition (e.g. "user_not_found",
	// "invalid_webhook_url"). Unlike Category, Code is open-ended — new
	// codes can be added freely as call sites are migrated.
	Code string

	// Category is the broad class this error belongs to, and determines the
	// HTTP status the boundary responds with (see Category.HTTPStatus).
	Category Category

	// Message is a human-readable description, safe to return to the
	// caller. It must not leak internal detail (stack traces, SQL, file
	// paths) — that belongs in the wrapped Err, logged but not returned.
	Message string

	// Retryable reports whether the caller can reasonably retry the same
	// request and expect a different outcome (e.g. a transient downstream
	// failure). Validation and auth errors are never retryable — retrying
	// with the same input produces the same result.
	Retryable bool

	// Err is the underlying error, if any. Not part of the public Message.
	Err error
}

// New constructs an AppError. Err may be nil.
func New(code string, category Category, message string, retryable bool, err error) *AppError {
	// Debug, e não Warn: construir um AppError não é por si só uma falha —
	// quem decide a gravidade é o call site que o consome. O registro aqui
	// existe para dar origem à taxonomia (qual código, qual categoria, qual
	// causa) sem depender de o consumidor lembrar de logar.
	log.Debug().
		Str("code", code).
		Str("category", string(category)).
		Bool("retryable", retryable).
		AnErr("cause", err).
		Msg("domain error constructed")
	return &AppError{
		Code:      code,
		Category:  category,
		Message:   message,
		Retryable: retryable,
		Err:       err,
	}
}

// Error implements the error interface. It returns Message, with the
// wrapped error appended if present — matching the convention already used
// by the repository's existing fmt.Errorf("...: %w", err) call sites.
func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap exposes the wrapped error to errors.Is/errors.As, matching the
// two existing errors.Is call sites in the repository
// (pkg/infra/history/sync.go, pkg/bootstrap/lifecycle.go) that this
// taxonomy is meant to generalize.
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is reports whether target has the same Code — the identity an AppError
// is compared by, not pointer identity or message text.
func (e *AppError) Is(target error) bool {
	var other *AppError
	if !errors.As(target, &other) {
		// target's chain has no AppError at all — a normal, frequent case
		// (e.g. errors.Is(err, sql.ErrNoRows)), not evidence of anything
		// wrong. Not logged: logging it here would fire on every legitimate
		// sentinel comparison against a non-AppError.
		return false
	}
	return e.Code == other.Code
}
