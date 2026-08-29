package engine

// A dirty stop is not an error to log; it is a fact the NEXT boot has to know.
//
// Decision A, 2026-08-12. Invariant 2 forbids stopping by signal, and phase 4C
// is why: repeated SIGTERM shutdowns logged the account out on the 4th and 6th
// iteration. But 4C measured signal as the ROUTINE path, taken every time. The
// residual case this file serves is the opposite — the signal only after
// Browser.close was sent, the exit was waited for, and neither worked. Carrying
// 4C's number into a rare last resort would be reusing a measurement outside the
// condition that produced it, which is the mistake this repository keeps paying
// for.
//
// So the signal stays, and the cost of keeping it is paid here: a stop that went
// dirty leaves a mark in the profile, and the next boot reads it. The session is
// then SUSPECT — to be verified rather than presumed good. That is the whole
// mechanism, and it exists because the alternative was worse: refusing to signal
// leaves the process alive, and ReclaimProfile refuses a profile whose lock
// holder is still running, so the session would be protected from corruption by
// being made unreachable.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// suspectMarker is written INSIDE the profile directory, on purpose.
//
// The profile is the thing whose integrity is in doubt, and it is what travels:
// a profile restored onto another host, or into a container, carries its own
// doubt with it. A marker in a sidecar directory would be left behind by exactly
// the copy that most needs it.
const suspectMarker = ".wa-headless-session-suspect"

// suspectFilePerm matches the profile's own 0o600 family: the marker names no
// secret, but it sits among files that do.
const suspectFilePerm = 0o600

// MarkSessionSuspect records that a profile's browser went down dirty.
//
// It is called by CleanStop and should not be called anywhere else — the point
// is that no call site can forget it, which is why it is not left to callers.
//
// A failure to write is returned rather than swallowed. It matters: an unwritten
// marker is a session that will be presumed good on the next boot, which is the
// exact state this mechanism exists to prevent.
func MarkSessionSuspect(profileDir string, via StopVia) error {
	if profileDir == "" {
		// Nothing to mark. A browser with no profile directory holds no
		// credential, so there is no session to be suspicious of.
		return nil
	}
	path := filepath.Join(profileDir, suspectMarker)
	if err := os.WriteFile(path, []byte(string(via)+"\n"), suspectFilePerm); err != nil {
		return fmt.Errorf("marking session suspect in %s: %w", profileDir, err)
	}
	return nil
}

// SessionSuspect reports whether the last stop of this profile went dirty, and
// how.
//
// A missing marker means "not suspect", and a profile that never ran is not
// suspect either. Any OTHER error is returned rather than folded into false: a
// profile we cannot read is not a profile we may declare healthy.
func SessionSuspect(profileDir string) (suspect bool, via StopVia, err error) {
	if profileDir == "" {
		return false, "", nil
	}
	raw, readErr := os.ReadFile(filepath.Join(profileDir, suspectMarker))
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("reading the suspect marker in %s: %w", profileDir, readErr)
	}
	return true, StopVia(strings.TrimSpace(string(raw))), nil
}

// ClearSessionSuspect removes the mark, and MUST only be called once the
// session has actually been verified against the application.
//
// It is separate from SessionSuspect for a reason worth stating: a reader that
// cleared the mark would make the doubt survive exactly one query, so a boot
// that read the state and then crashed would lose it. The doubt outlives
// restarts until something proves the session is fine.
func ClearSessionSuspect(profileDir string) error {
	if profileDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(profileDir, suspectMarker))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing the suspect marker in %s: %w", profileDir, err)
	}
	return nil
}
