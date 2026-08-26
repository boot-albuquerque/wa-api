package handlers

import "wa-api/pkg/domain/apperr"

// Boundary-error sentinels shared by the handlers in this package. Each
// describes a missing or unreadable piece of the HTTP request — never a
// domain failure, which arrives typed from the use cases.
//
// All five are *apperr.AppError so that RespondJSON produces the structured
// envelope ({"code": ..., "error": {"code": ..., "message": ...}}) that
// the F224 errors already use — eliminating the two-format split (F236).
var (
	errUnauthorized = &apperr.AppError{
		Code:     "unauthorized",
		Category: apperr.CategoryUnauthorized,
		Message:  "unauthorized",
	}
	errMissingSessionID = &apperr.AppError{
		Code:     "missing_session_id",
		Category: apperr.CategoryValidation,
		Message:  "missing session id",
	}
	errMissingID = &apperr.AppError{
		Code:     "missing_id",
		Category: apperr.CategoryValidation,
		Message:  "missing ID",
	}
	errDecodePayload = &apperr.AppError{
		Code:     "decode_payload_failed",
		Category: apperr.CategoryValidation,
		Message:  "could not decode payload",
	}
	// errMissingJID covers the {jid} path parameter, not a body field —
	// distinct from errDecodePayload on purpose (F81).
	errMissingJID = &apperr.AppError{
		Code:     "missing_jid",
		Category: apperr.CategoryValidation,
		Message:  "missing jid in path",
	}
)
