package errmap_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"wa-api/internal/wa-noise/capabilities/media"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/wa-noise/errmap"
)

// cdnError builds the error chain the download path produces:
//
//	fmt.Errorf("failed to download media from last host: %w", DownloadHTTPError{...})
//
// Source: internal/wa-noise/capabilities/media/download.go, line ~149.
func cdnError(statusCode int) error {
	return fmt.Errorf("failed to download media from last host: %w",
		media.DownloadHTTPError{Response: &http.Response{StatusCode: statusCode}},
	)
}

func TestClassifyDownload_403BecomesMediaUnavailable(t *testing.T) {
	got := errmap.ClassifyDownload(cdnError(403))

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("expected *apperr.AppError, got %T: %v", got, got)
	}
	if app.Code != errmap.CodeMediaUnavailable {
		t.Fatalf("code = %q, want %q", app.Code, errmap.CodeMediaUnavailable)
	}
	if app.Category != apperr.CategoryNotFound {
		t.Fatalf("category = %q, want %q", app.Category, apperr.CategoryNotFound)
	}
	if want := http.StatusNotFound; app.Category.HTTPStatus() != want {
		t.Fatalf("HTTPStatus = %d, want %d", app.Category.HTTPStatus(), want)
	}
}

func TestClassifyDownload_404BecomesMediaUnavailable(t *testing.T) {
	got := errmap.ClassifyDownload(cdnError(404))

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("expected *apperr.AppError, got %T: %v", got, got)
	}
	if app.Code != errmap.CodeMediaUnavailable {
		t.Fatalf("code = %q, want %q", app.Code, errmap.CodeMediaUnavailable)
	}
}

func TestClassifyDownload_410BecomesMediaUnavailable(t *testing.T) {
	got := errmap.ClassifyDownload(cdnError(410))

	var app *apperr.AppError
	if !errors.As(got, &app) {
		t.Fatalf("expected *apperr.AppError, got %T: %v", got, got)
	}
	if app.Code != errmap.CodeMediaUnavailable {
		t.Fatalf("code = %q, want %q", app.Code, errmap.CodeMediaUnavailable)
	}
}

func TestClassifyDownload_500PassesThrough(t *testing.T) {
	orig := cdnError(500)
	got := errmap.ClassifyDownload(orig)

	var app *apperr.AppError
	if errors.As(got, &app) {
		t.Fatalf("500 should NOT be classified, but got apperr code %q", app.Code)
	}
}

func TestClassifyDownload_NonDownloadErrorPassesThrough(t *testing.T) {
	orig := fmt.Errorf("some other error")
	got := errmap.ClassifyDownload(orig)
	if got != orig {
		t.Fatalf("non-download error should pass through unchanged")
	}
}

func TestClassifyDownload_NilReturnsNil(t *testing.T) {
	if got := errmap.ClassifyDownload(nil); got != nil {
		t.Fatalf("nil should return nil, got %v", got)
	}
}
