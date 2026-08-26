package handlers

import "wa-api/pkg/domain/apperr"

// Boundary sentinel errors shared across handlers. Each describes a failure
// at the HTTP boundary — something missing or unreadable in the request —
// never a domain failure, which comes typed from the use cases.
//
// These are *apperr.AppError so that RespondJSON derives both the status
// code AND the structured {"code":..., "message":...} object from the
// taxonomy. The 244 call sites that pass them need no change: the
// Category.HTTPStatus matches the status each call site already passes
// (F236).
var (
	errUnauthorized = &apperr.AppError{
		Code:     CodeUnauthorized,
		Category: apperr.CategoryUnauthorized,
		Message:  "unauthorized",
	}
	errMissingSessionID = &apperr.AppError{
		Code:     CodeMissingSessionID,
		Category: apperr.CategoryValidation,
		Message:  "missing session id",
	}
	errMissingID = &apperr.AppError{
		Code:     CodeMissingID,
		Category: apperr.CategoryValidation,
		Message:  "missing ID",
	}
	errDecodePayload = &apperr.AppError{
		Code:     CodeDecodePayload,
		Category: apperr.CategoryValidation,
		Message:  "could not decode payload",
	}
	errMissingJID = &apperr.AppError{
		Code:     CodeMissingJID,
		Category: apperr.CategoryValidation,
		Message:  "missing jid in path",
	}
)

const (
	CodeUnauthorized     = "unauthorized"
	CodeMissingSessionID = "missing_session_id"
	CodeMissingID        = "missing_id"
	CodeDecodePayload    = "could_not_decode_payload"
	CodeMissingJID       = "missing_jid"
	CodeInvalidJID       = "invalid_jid"
)
