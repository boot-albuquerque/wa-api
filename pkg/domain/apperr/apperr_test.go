package apperr

import (
	"errors"
	"net/http"
	"testing"
)

func TestNew(t *testing.T) {
	wrapped := errors.New("db connection refused")
	err := New("user_not_found", CategoryValidation, "user not found", false, wrapped)

	if err.Code != "user_not_found" {
		t.Errorf("Code = %q, want %q", err.Code, "user_not_found")
	}
	if err.Category != CategoryValidation {
		t.Errorf("Category = %q, want %q", err.Category, CategoryValidation)
	}
	if err.Message != "user not found" {
		t.Errorf("Message = %q, want %q", err.Message, "user not found")
	}
	if err.Retryable {
		t.Error("Retryable = true, want false")
	}
	if err.Err != wrapped {
		t.Errorf("Err = %v, want %v", err.Err, wrapped)
	}
}

func TestError_WithWrappedErr(t *testing.T) {
	wrapped := errors.New("connection refused")
	err := New("db_error", CategoryInternal, "database error", true, wrapped)

	want := "database error: connection refused"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestError_WithoutWrappedErr(t *testing.T) {
	err := New("invalid_input", CategoryValidation, "invalid input", false, nil)

	want := "invalid input"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnwrap(t *testing.T) {
	wrapped := errors.New("root cause")
	err := New("db_error", CategoryInternal, "database error", true, wrapped)

	if got := errors.Unwrap(err); got != wrapped {
		t.Errorf("errors.Unwrap() = %v, want %v", got, wrapped)
	}
}

func TestUnwrap_Nil(t *testing.T) {
	err := New("invalid_input", CategoryValidation, "invalid input", false, nil)

	if got := errors.Unwrap(err); got != nil {
		t.Errorf("errors.Unwrap() = %v, want nil", got)
	}
}

func TestErrorsIs_SameCode(t *testing.T) {
	sentinel := New("user_not_found", CategoryValidation, "user not found", false, nil)
	occurrence := New("user_not_found", CategoryValidation, "user 42 not found", false, errors.New("sql: no rows"))

	if !errors.Is(occurrence, sentinel) {
		t.Error("errors.Is(occurrence, sentinel) = false, want true — same Code should match")
	}
}

func TestErrorsIs_DifferentCode(t *testing.T) {
	sentinel := New("user_not_found", CategoryValidation, "user not found", false, nil)
	other := New("invalid_input", CategoryValidation, "invalid input", false, nil)

	if errors.Is(other, sentinel) {
		t.Error("errors.Is(other, sentinel) = true, want false — different Code should not match")
	}
}

func TestErrorsIs_NonAppError(t *testing.T) {
	sentinel := New("user_not_found", CategoryValidation, "user not found", false, nil)
	plain := errors.New("some other error")

	if errors.Is(plain, sentinel) {
		t.Error("errors.Is(plain, sentinel) = true, want false — plain error is not an AppError")
	}
}

func TestErrorsAs(t *testing.T) {
	wrapped := errors.New("root cause")
	original := New("db_error", CategoryInternal, "database error", true, wrapped)

	var target *AppError
	if !errors.As(error(original), &target) {
		t.Fatal("errors.As() = false, want true")
	}
	if target.Code != "db_error" {
		t.Errorf("target.Code = %q, want %q", target.Code, "db_error")
	}
}

func TestCategory_HTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		category Category
		want     int
	}{
		{"validation", CategoryValidation, http.StatusBadRequest},
		{"unauthorized", CategoryUnauthorized, http.StatusUnauthorized},
		{"internal", CategoryInternal, http.StatusInternalServerError},
		{"unknown category defaults to internal", Category("something_new"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.category.HTTPStatus(); got != tt.want {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRetryable(t *testing.T) {
	validationErr := New("invalid_input", CategoryValidation, "invalid input", false, nil)
	if validationErr.Retryable {
		t.Error("validation error should not be retryable")
	}

	transientErr := New("upstream_timeout", CategoryInternal, "upstream timed out", true, nil)
	if !transientErr.Retryable {
		t.Error("transient internal error should be retryable")
	}
}
