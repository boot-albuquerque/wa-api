package errmap

import (
	"errors"

	"github.com/rs/zerolog/log"

	"wa-api/internal/noise/protocol/types"
	"wa-api/pkg/domain/apperr"
)

// Newsletter operations passed to ClassifyNewsletter so the error message
// matches the action the caller attempted. Defined here instead of in the
// adapter to keep the errmap self-contained.
const (
	OpNewsletterUnfollow    = "unfollow"
	OpNewsletterDemote      = "demote"
	OpNewsletterChangeOwner = "change_owner"
)

// Error codes returned to the API consumer.
const (
	CodeNewsletterAdminCannotUnfollow = "newsletter_admin_cannot_unfollow"
	CodeNewsletterCannotDemoteOwner   = "newsletter_cannot_demote_owner"
	CodeNewsletterNewOwnerNotAdmin    = "newsletter_new_owner_not_admin"
)

// ClassifyNewsletter translates GraphQL 4xx refusals from the WhatsApp
// newsletter API into CategoryForbidden / CategoryUnauthorized, choosing
// the error code and message based on the operation that was attempted.
//
// Measured codes (2026-08-25, HOUSEKEEP F233c / F235):
//
//	405 on unfollow     → admin cannot unfollow own channel
//	405 on demote       → cannot demote the channel owner
//	401 on change_owner → new owner is not an admin
//
// Codes / operations not in the table above pass through unchanged — we do
// not invent mappings for codes we have not observed (HOUSEKEEP rule).
func ClassifyNewsletter(err error, operation string) error {
	if err == nil {
		return nil
	}

	var gqlErr types.GraphQLError
	if !errors.As(err, &gqlErr) {
		return err
	}

	code, msg, ok := newsletterRefusal(gqlErr.Extensions.ErrorCode, operation)
	if !ok {
		return err
	}

	log.Debug().
		Int("graphqlCode", gqlErr.Extensions.ErrorCode).
		Str("operation", operation).
		Str("category", string(apperr.CategoryForbidden)).
		Str("code", code).
		Msg("translated newsletter refusal")

	return apperr.New(code, apperr.CategoryForbidden, msg, false, err)
}

// newsletterRefusal returns the app-error code and human message for a
// (graphqlCode, operation) pair, or ok=false when the pair has not been
// observed and must not be translated.
func newsletterRefusal(graphqlCode int, operation string) (code, msg string, ok bool) {
	switch {
	case graphqlCode == 405 && operation == OpNewsletterUnfollow:
		return CodeNewsletterAdminCannotUnfollow,
			"channel admins cannot unfollow their own channel; dismiss yourself as admin first",
			true

	case graphqlCode == 405 && operation == OpNewsletterDemote:
		return CodeNewsletterCannotDemoteOwner,
			"the channel owner cannot be demoted; transfer ownership first",
			true

	case graphqlCode == 401 && operation == OpNewsletterChangeOwner:
		return CodeNewsletterNewOwnerNotAdmin,
			"the new owner must already be a channel admin",
			true

	default:
		return "", "", false
	}
}
