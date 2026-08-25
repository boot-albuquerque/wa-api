package errmap_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/errmap"
)

// graphqlError builds the error chain that mex.go:decodeGraphQLResult produces:
//
//	fmt.Errorf("graphql error: %w", GraphQLErrors{...})
//
// Source: internal/wa-noise/capabilities/newsletter/mex.go, line ~111.
func graphqlError(code int, message, severity string) error {
	gqlErrors := types.GraphQLErrors{
		{
			Extensions: types.GraphQLErrorExtensions{
				ErrorCode: code,
				Severity:  severity,
			},
			Message: message,
		},
	}
	return fmt.Errorf("graphql error: %w", gqlErrors)
}

// The MEASURED case: POST /newsletter/unfollow on a channel whose owner is the
// caller. The server answered 405 Not Allowed (CRITICAL), and we answered 500
// "internal server error" — "we broke" — for a business rule refusal that is
// NOT our fault (F233, 2026-08-25).
//
// The error shape is the one observed in the production log, built by
// decodeGraphQLResult wrapping GraphQLErrors{GraphQLError{ErrorCode:405, ...}}.
func TestClassifyNewsletter_405BecomesA403(t *testing.T) {
	measured := graphqlError(405, "Not Allowed", "CRITICAL")

	got := errmap.ClassifyNewsletter(measured)

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("the 405 was not translated to AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusForbidden {
		t.Errorf("status = %d, want 403: a 405 admin refusal must not become a 500 from us", s)
	}
	if app.Code != errmap.CodeNewsletterAdminCannotUnfollow {
		t.Errorf("code = %q, want %q", app.Code, errmap.CodeNewsletterAdminCannotUnfollow)
	}
	if app.Retryable {
		t.Error("retryable = true: an admin refusal is not recoverable by retry")
	}
	if !errors.Is(got, measured) {
		t.Error("the original error is no longer reachable via errors.Is: the log loses the cause")
	}
}

// The COMPLEMENT: a non-405 GraphQL error must stay as-is. Turning everything
// into 403 would trade one lie for another — the most likely failure mode of
// this fix.
func TestClassifyNewsletter_NonForbiddenStaysUnchanged(t *testing.T) {
	serverError := graphqlError(500, "Internal Server Error", "CRITICAL")

	got := errmap.ClassifyNewsletter(serverError)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("a non-405 GraphQL error was classified as AppError (%s): "+
			"only the 405 should be translated", app.Category)
	}
	if got != serverError {
		t.Errorf("the error was modified: want the original back unchanged")
	}
}

// Nil and non-GraphQL errors must pass through untouched.
func TestClassifyNewsletter_PassthroughCases(t *testing.T) {
	if got := errmap.ClassifyNewsletter(nil); got != nil {
		t.Errorf("nil turned into %v", got)
	}

	other := errors.New("dial tcp: connection refused")
	if got := errmap.ClassifyNewsletter(other); got != other {
		t.Errorf("unrelated error was modified: %v", got)
	}
}

// An error that happens to contain "405" as TEXT but is NOT a GraphQLError
// must NOT be classified. Without the typed check, a random error string
// could be misinterpreted.
func TestClassifyNewsletter_Text405WithoutGraphQLErrorIsIgnored(t *testing.T) {
	fake := errors.New("some other problem with 405 in it")

	got := errmap.ClassifyNewsletter(fake)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("an error unrelated to GraphQL was classified as forbidden: %v", got)
	}
	if got != fake {
		t.Errorf("the error was modified")
	}
}

// A GraphQL error with code 0 (the zero value) must NOT be classified.
func TestClassifyNewsletter_ZeroCodeIsIgnored(t *testing.T) {
	zeroCode := graphqlError(0, "Unknown error", "WARNING")

	got := errmap.ClassifyNewsletter(zeroCode)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("zero-code GraphQL error was classified: %v", got)
	}
}
