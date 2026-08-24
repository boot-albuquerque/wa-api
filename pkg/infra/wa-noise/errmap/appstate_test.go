package errmap_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"wa-api/internal/wa-noise/capabilities/appstatesync"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/errmap"
)

// The MEASURED case: POST /chat/mute and POST /message/star with a well
// formed patch on regular_high. The server answered 409 conflict, and we
// answered 500 "internal server error" — "we broke" — for a state
// desynchronization that is NOT our fault (F223, 2026-08-24).
//
// The error text is the one observed in the production log, built by
// appstatesync/send.go:handleSendError:
//
//	mainErr = fmt.Errorf("%w (%s): %s", ErrUpdate, patch.Type, errorTagNode.XMLString())
//
// When the conflict retry fails to apply patches:
//
//	fmt.Errorf("%w (also, applying patches in the response failed: %w)", mainErr, applyErr)
func TestClassifyAppState_ConflictBecomesA409(t *testing.T) {
	// Reproduce the real error shape from appstatesync/send.go:handleSendError.
	// The XML comes from waBinary.Node.XMLString() — the exact format the
	// fork produces. The "also, applying patches" suffix is from the retry
	// path that failed (the measured case from the log).
	conflictXML := `<error code="409" text="conflict"/>`
	innerErr := fmt.Errorf("%w (regular_high): %s", appstatesync.ErrUpdate, conflictXML)
	measured := fmt.Errorf("%w (also, applying patches in the response failed: failed to decode app state regular_high patches: failed to verify patch v64: mismatching LTHash)", innerErr)

	got := errmap.ClassifyAppState(measured)

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("the conflict was not translated to AppError: %T (%v)", got, got)
	}
	if s := app.Category.HTTPStatus(); s != http.StatusConflict {
		t.Errorf("status = %d, want 409: a 409 from the server must not become a 500 from us", s)
	}
	if app.Code != errmap.CodeAppStateConflict {
		t.Errorf("code = %q, want %q", app.Code, errmap.CodeAppStateConflict)
	}
	if !app.Retryable {
		t.Error("retryable = false: a state conflict is recoverable after resync")
	}
	if !errors.Is(got, measured) {
		t.Error("the original error is no longer reachable via errors.Is: the log loses the cause")
	}
}

// The COMPLEMENT of the conflict test: a non-409 app-state error must stay
// as-is. Turning everything into 409 would trade one lie for another — the
// most likely failure mode of this fix (task spec, requirement #2 & #5).
func TestClassifyAppState_NonConflictStaysUnchanged(t *testing.T) {
	// A server error (500) wrapping ErrUpdate — NOT a conflict. This is the
	// shape handleSendError produces for codes other than 409.
	serverErrorXML := `<error code="500" text="internal-server-error"/>`
	nonConflict := fmt.Errorf("%w (regular_high): %s", appstatesync.ErrUpdate, serverErrorXML)

	got := errmap.ClassifyAppState(nonConflict)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("a non-conflict app state error was classified as AppError (%s): "+
			"only the 409 conflict should be translated", app.Category)
	}
	if got != nonConflict {
		t.Errorf("the error was modified: want the original back unchanged")
	}
}

// Nil and non-ErrUpdate errors must pass through untouched — the same
// boundary that ClassifyIQ enforces.
func TestClassifyAppState_PassthroughCases(t *testing.T) {
	if got := errmap.ClassifyAppState(nil); got != nil {
		t.Errorf("nil turned into %v", got)
	}

	other := errors.New("dial tcp: connection refused")
	if got := errmap.ClassifyAppState(other); got != other {
		t.Errorf("unrelated error was modified: %v", got)
	}
}

// An error that mentions code="409" but is NOT an ErrUpdate must NOT be
// classified. Without the ErrUpdate guard, a random error containing "409"
// in its text would become a conflict — a false positive.
func TestClassifyAppState_Code409WithoutErrUpdateIsIgnored(t *testing.T) {
	fake := errors.New(`some other problem with code="409" in it`)

	got := errmap.ClassifyAppState(fake)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("an error unrelated to app state was classified as conflict: %v", got)
	}
	if got != fake {
		t.Errorf("the error was modified")
	}
}

// The bare ErrUpdate sentinel — no XML, no type. This can happen when the
// collection response has no <error> tag (the fallback path in
// handleSendError). It must NOT become 409.
func TestClassifyAppState_BareErrUpdateIsNotConflict(t *testing.T) {
	bare := fmt.Errorf("%w: <collection />", appstatesync.ErrUpdate)

	got := errmap.ClassifyAppState(bare)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("bare ErrUpdate without a conflict code was classified: %s", app.Category)
	}
}
