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

// --- 405 on unfollow (original F233 case, preserved) ---

func TestClassifyNewsletter_405OnUnfollow(t *testing.T) {
	measured := graphqlError(405, "Not Allowed", "CRITICAL")

	got := errmap.ClassifyNewsletter(measured, errmap.OpNewsletterUnfollow)

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("the 405 was not translated to AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusForbidden {
		t.Errorf("status = %d, want 403", s)
	}
	if app.Code != errmap.CodeNewsletterAdminCannotUnfollow {
		t.Errorf("code = %q, want %q", app.Code, errmap.CodeNewsletterAdminCannotUnfollow)
	}
	if app.Retryable {
		t.Error("retryable = true: an admin refusal is not recoverable by retry")
	}
	if !errors.Is(got, measured) {
		t.Error("the original error is no longer reachable via errors.Is")
	}
}

// --- 405 on demote (F235: must NOT say "unfollow") ---

func TestClassifyNewsletter_405OnDemote(t *testing.T) {
	measured := graphqlError(405, "Not Allowed", "CRITICAL")

	got := errmap.ClassifyNewsletter(measured, errmap.OpNewsletterDemote)

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("the 405 was not translated to AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusForbidden {
		t.Errorf("status = %d, want 403", s)
	}
	if app.Code != errmap.CodeNewsletterCannotDemoteOwner {
		t.Errorf("code = %q, want %q", app.Code, errmap.CodeNewsletterCannotDemoteOwner)
	}
	if app.Code == errmap.CodeNewsletterAdminCannotUnfollow {
		t.Error("demote must NOT use the unfollow error code — this is the F235 regression")
	}
	if !errors.Is(got, measured) {
		t.Error("the original error is no longer reachable via errors.Is")
	}
}

// --- 401 on change_owner (measured: new owner is not admin) ---

func TestClassifyNewsletter_401OnChangeOwner(t *testing.T) {
	measured := graphqlError(401, "Not Authorized", "CRITICAL")

	got := errmap.ClassifyNewsletter(measured, errmap.OpNewsletterChangeOwner)

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("the 401 was not translated to AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusForbidden {
		t.Errorf("status = %d, want 403", s)
	}
	if app.Code != errmap.CodeNewsletterNewOwnerNotAdmin {
		t.Errorf("code = %q, want %q", app.Code, errmap.CodeNewsletterNewOwnerNotAdmin)
	}
	if !errors.Is(got, measured) {
		t.Error("the original error is no longer reachable via errors.Is")
	}
}

// --- Passthrough: non-mapped combinations stay untouched ---

func TestClassifyNewsletter_NonForbiddenStaysUnchanged(t *testing.T) {
	serverError := graphqlError(500, "Internal Server Error", "CRITICAL")

	got := errmap.ClassifyNewsletter(serverError, errmap.OpNewsletterUnfollow)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("a non-mapped GraphQL error was classified as AppError (%s): "+
			"only observed (code, operation) pairs should be translated", app.Category)
	}
	if got != serverError {
		t.Errorf("the error was modified: want the original back unchanged")
	}
}

func TestClassifyNewsletter_PassthroughCases(t *testing.T) {
	if got := errmap.ClassifyNewsletter(nil, ""); got != nil {
		t.Errorf("nil turned into %v", got)
	}

	other := errors.New("dial tcp: connection refused")
	if got := errmap.ClassifyNewsletter(other, ""); got != other {
		t.Errorf("unrelated error was modified: %v", got)
	}
}

func TestClassifyNewsletter_Text405WithoutGraphQLErrorIsIgnored(t *testing.T) {
	fake := errors.New("some other problem with 405 in it")

	got := errmap.ClassifyNewsletter(fake, errmap.OpNewsletterUnfollow)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("an error unrelated to GraphQL was classified as forbidden: %v", got)
	}
	if got != fake {
		t.Errorf("the error was modified")
	}
}

func TestClassifyNewsletter_ZeroCodeIsIgnored(t *testing.T) {
	zeroCode := graphqlError(0, "Unknown error", "WARNING")

	got := errmap.ClassifyNewsletter(zeroCode, errmap.OpNewsletterUnfollow)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("zero-code GraphQL error was classified: %v", got)
	}
}

// --- 405 on empty operation stays untouched (no mapping for generic calls) ---

func TestClassifyNewsletter_405OnEmptyOperationPassesThrough(t *testing.T) {
	err := graphqlError(405, "Not Allowed", "CRITICAL")

	got := errmap.ClassifyNewsletter(err, "")

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("a 405 with no operation should NOT be classified — got code %q", app.Code)
	}
}

// --- 500 on any operation continues as 500 (the HOUSEKEEP rule) ---

func TestClassifyNewsletter_500OnDemoteContinuesAs500(t *testing.T) {
	err := graphqlError(500, "Internal Server Error", "CRITICAL")

	got := errmap.ClassifyNewsletter(err, errmap.OpNewsletterDemote)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("500 on demote should not be translated: %v", got)
	}
}

// --- Control negative: removing operation guard from 405 must break ---

func TestClassifyNewsletter_ControlNegative_405OnDemoteMustNotBeUnfollow(t *testing.T) {
	measured := graphqlError(405, "Not Allowed", "CRITICAL")

	gotDemote := errmap.ClassifyNewsletter(measured, errmap.OpNewsletterDemote)
	gotUnfollow := errmap.ClassifyNewsletter(measured, errmap.OpNewsletterUnfollow)

	var demoteApp, unfollowApp *apperr.AppError
	if !errors.As(gotDemote, &demoteApp) {
		t.Fatal("demote 405 was not classified")
	}
	if !errors.As(gotUnfollow, &unfollowApp) {
		t.Fatal("unfollow 405 was not classified")
	}
	if demoteApp.Code == unfollowApp.Code {
		t.Errorf("demote and unfollow produced the same code %q — "+
			"the operation guard is not working (F235 regression)", demoteApp.Code)
	}
}
