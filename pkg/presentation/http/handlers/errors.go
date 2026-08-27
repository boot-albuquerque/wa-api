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

// Boundary validation codes for the group-management family. Each names ONE
// condition, so a client branches on the code instead of parsing a sentence —
// the generic `invalid_request` these used to fall back to only said "something
// was wrong" (docs/HTTP-DTO-CONVENTIONS.md §7).
//
// The wire field name each code guards is spelled in the code itself, in
// snake_case, even where the JSON key is not: the request accepts both
// `groupJID` and `groupjid`, and both must answer with ONE stable code.
const (
	CodeMissingName             = "missing_name"
	CodeMissingParticipants     = "missing_participants"
	CodeEmptyParticipant        = "empty_participant"
	CodeMissingInviteCode       = "missing_code"
	CodeMissingGroupJID         = "missing_group_jid"
	CodeMissingTopic            = "missing_topic"
	CodeMissingPhoto            = "missing_photo"
	CodeMissingPhones           = "missing_phones"
	CodeEmptyPhone              = "empty_phone"
	CodeMissingAction           = "missing_action"
	CodeMutuallyExclusiveParent = "mutually_exclusive_parent_fields"
	CodeInvalidPhotoEncoding    = "invalid_photo_encoding"
)

// Webhook persistence codes. The four are NOT collapsed into one
// `webhook_failed`: the caller of DELETE /webhook and the caller of GET
// /webhook take different actions when the store is unavailable, and a single
// code makes the two indistinguishable in a client's telemetry.
const (
	CodeWebhookReadFailed   = "webhook_read_failed"
	CodeWebhookWriteFailed  = "webhook_write_failed"
	CodeWebhookUpdateFailed = "webhook_update_failed"
	CodeWebhookDeleteFailed = "webhook_delete_failed"
)
