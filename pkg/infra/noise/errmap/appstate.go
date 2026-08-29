package errmap

import (
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"wa-api/internal/noise/capabilities/appstatesync"
	"wa-api/pkg/domain/apperr"
)

// CodeAppStateConflict identifies an app-state patch rejected by the server
// with a 409 conflict — local state is out of sync with the server's.
const CodeAppStateConflict = "app_state_conflict"

// ClassifyAppState translates an app-state conflict into CategoryConflict,
// leaving everything else untouched.
//
// It exists because the conflict reached the caller as 500 internal server
// error: measured on POST /chat/mute and POST /message/star on 2026-08-24,
// where the server answered 409 conflict on regular_high patches and we
// answered "we broke, retry" (F223). The caller cannot tell "we are out of
// sync, try again" from "the server is down".
//
// Only the 409 conflict becomes CategoryConflict. Network failures, encoding
// errors, and non-conflict server refusals pass through unchanged — turning
// them into 409 would trade one lie for another.
//
// The match is on the error TEXT because the fork (internal/wa-noise) does not
// export a typed error or sentinel for the conflict specifically — ErrUpdate
// (= ErrAppStateUpdate) covers ALL server-side app-state failures. The matched
// substring `code="409"` comes from waBinary.Node.XMLString() as embedded by
// appstatesync/send.go:handleSendError. This is fragile by construction: if
// the fork changes its XML serialization, this match breaks silently. The
// alternative — modifying internal/ to export a conflict sentinel — was ruled
// out by project policy (ADR-001).
func ClassifyAppState(err error) error {
	if err == nil {
		return nil
	}

	if !errors.Is(err, appstatesync.ErrUpdate) {
		return err
	}

	if !isAppStateConflict(err) {
		return err
	}

	log.Debug().Str("category", string(apperr.CategoryConflict)).
		Str("code", CodeAppStateConflict).
		Msg("translated app state conflict")

	return apperr.New(
		CodeAppStateConflict,
		apperr.CategoryConflict,
		"app state conflict: the local state is out of sync with the server",
		true,
		err,
	)
}

// isAppStateConflict checks whether the error is specifically a 409 conflict.
//
// String matching because the fork wraps the XML node text into the error
// message (fmt.Errorf("%w (%s): %s", ErrUpdate, patchType, node.XMLString()))
// and does not expose the code as a typed field. The substring `code="409"`
// is the XML attribute as serialized by waBinary.Node.XMLString(); the
// production error looks like:
//
//	server returned error updating app state (regular_high):
//	  <error code="409" text="conflict"/>
//
// See appstatesync/send.go:handleSendError for the construction.
func isAppStateConflict(err error) bool {
	return strings.Contains(err.Error(), `code="409"`)
}
