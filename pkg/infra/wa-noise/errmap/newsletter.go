package errmap

import (
	"errors"

	"github.com/rs/zerolog/log"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain/apperr"
)

// CodeNewsletterAdminCannotUnfollow identifies a GraphQL 405 from the WhatsApp
// server on an unfollow attempt — the caller is an admin of the channel and
// admins cannot unfollow channels they manage.
const CodeNewsletterAdminCannotUnfollow = "newsletter_admin_cannot_unfollow"

// ClassifyNewsletter translates a GraphQL 405 refusal into CategoryForbidden,
// leaving everything else untouched.
//
// It exists because the refusal reached the caller as 500 internal server
// error: measured on POST /newsletter/unfollow on 2026-08-25, where the
// WhatsApp server answered 405 Not Allowed (CRITICAL) and we answered
// "we broke, retry" (F233). The caller cannot tell "you are an admin, dismiss
// yourself first" from "the server is down".
//
// Only the 405 becomes CategoryForbidden. Network failures, encoding errors,
// and other GraphQL error codes pass through unchanged — turning them into 403
// would trade one lie for another.
//
// The match is on the TYPED GraphQLError.Extensions.ErrorCode field, not on
// error text. This is possible because the fork (internal/wa-noise) wraps
// GraphQL errors as types.GraphQLErrors (implementing Unwrap() []error), and
// each element is a types.GraphQLError with an Is() method that compares by
// ErrorCode. This is more robust than the string match in ClassifyAppState:
// renaming the error message does not break the match.
//
// Source of the error: internal/wa-noise/capabilities/newsletter/mex.go,
// decodeGraphQLResult, line ~111:
//
//	return gqlResp.Data, fmt.Errorf("graphql error: %w", gqlResp.Errors)
func ClassifyNewsletter(err error) error {
	if err == nil {
		return nil
	}

	var gqlErr types.GraphQLError
	if !errors.As(err, &gqlErr) {
		return err
	}

	if gqlErr.Extensions.ErrorCode != 405 {
		return err
	}

	log.Debug().Int("graphqlCode", gqlErr.Extensions.ErrorCode).
		Str("category", string(apperr.CategoryForbidden)).
		Str("code", CodeNewsletterAdminCannotUnfollow).
		Msg("translated newsletter admin refusal")

	return apperr.New(
		CodeNewsletterAdminCannotUnfollow,
		apperr.CategoryForbidden,
		"channel admins cannot unfollow their own channel; dismiss yourself as admin first",
		false,
		err,
	)
}
