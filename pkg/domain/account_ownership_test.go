package domain

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrSessionSuperseded_IdentityComparable guards against F236's class of
// bug: a call site that got ErrSessionSuperseded via fmt.Errorf("%w", ...)
// and then a DIFFERENT call site constructing "session_superseded" by
// message text alone must not silently compare unequal by errors.Is.
func TestErrSessionSuperseded_IdentityComparable(t *testing.T) {
	wrapped := fmt.Errorf("checking token: %w", ErrSessionSuperseded)
	if !errors.Is(wrapped, ErrSessionSuperseded) {
		t.Fatal("errors.Is failed to match a wrapped ErrSessionSuperseded")
	}

	textOnly := errors.New(sessionSupersededCode)
	if errors.Is(textOnly, ErrSessionSuperseded) {
		t.Fatal("a plain errors.New with the same message matched ErrSessionSuperseded by errors.Is: identity must be by TYPE, not text")
	}
}

func TestErrSessionSuperseded_MessageDoesNotLeakDetail(t *testing.T) {
	// The prompt requires "sem vazar detalhes da nova sessão". The error
	// message must be exactly the stable code, nothing appended.
	if got, want := ErrSessionSuperseded.Error(), sessionSupersededCode; got != want {
		t.Errorf("ErrSessionSuperseded.Error() = %q, want %q (must not carry extra detail)", got, want)
	}
}
