package apperr

import (
	"context"
	"errors"
	"net/http"
)

// Category is a broad error class that the HTTP boundary maps to a status
// code. The set below is derived from a census of every statusCode actually
// passed to RespondJSON across the presentation layer (305 call sites, see
// the Fase 3 PR body for the raw counts) — not from theory. Only categories
// backed by an observed status make the cut; anything else needs a written
// justification before it's added.
//
//	200 OK                  -> CategoryNone (not an error)
//	400 Bad Request  (93x)  -> CategoryValidation
//	401 Unauthorized (37x)  -> CategoryUnauthorized
//	500 Internal     (92x)  -> CategoryInternal
type Category string

const (
	// CategoryValidation covers client-supplied input that fails validation:
	// malformed request bodies, missing required fields, invalid parameters.
	CategoryValidation Category = "validation"

	// CategoryUnauthorized covers authentication/authorization failures:
	// missing, invalid, or insufficiently privileged credentials.
	CategoryUnauthorized Category = "unauthorized"

	// CategoryConflict covers requests that are well formed and authorized but
	// cannot be served in the CURRENT state, or by THIS instance.
	//
	// It exists because the three categories above forced a wrong answer twice
	// (F95). Both cases are the same shape: the caller did nothing wrong, and
	// repeating the identical request against the same target yields the same
	// result — but changing the target or the state makes it succeed.
	//
	//   - session owned by another replica: route the request to its owner
	//   - logout of a disconnected session: reconnect first, then retry
	//
	// CategoryValidation (400) tells the caller "fix your payload", which is
	// actively misleading here: there is nothing to fix in the payload.
	CategoryConflict Category = "conflict"

	// CategoryNotFound covers a well formed, authorized request for a resource
	// that is not there.
	//
	// It exists for the same reason CategoryConflict does: the categories above
	// forced a wrong answer. `user not found` was returning 500, and a caller
	// could not tell "the database is down" from "that id does not exist" — one
	// is worth retrying, the other never will be (F101).
	//
	// CategoryValidation (400) would be worse than the 500 it replaces: it says
	// "fix your payload", and the payload is fine. CategoryInternal (500) says
	// "we broke", and we did not.
	CategoryNotFound Category = "not_found"

	// CategoryNotImplemented covers a well formed, authorized request for an
	// endpoint whose FEATURE the user has turned off.
	//
	// It exists because GET /chat/history has answered 501 with
	// "message history is disabled for this user" since commit 3dafae0, and
	// that 501 is public contract: callers read it as "switch the feature on",
	// not as "retry" or "fix your payload". None of the categories above can
	// produce a 501 — CategoryValidation (400) says the payload is wrong when
	// there is nothing to fix, and CategoryInternal (500) claims we broke when
	// we did not.
	CategoryNotImplemented Category = "not_implemented"

	// CategoryForbidden covers a request whose credentials are valid but whose
	// TARGET refuses it — the upstream WhatsApp server answering 403 forbidden.
	//
	// It is not CategoryUnauthorized (401): 401 tells the caller "authenticate,
	// or present better credentials", and there are no better credentials to
	// present. The session is authenticated; the operation is not permitted on
	// that target. Squashing the two makes a caller retry a login that was
	// never the problem (F204).
	CategoryForbidden Category = "forbidden"

	// CategoryRateLimited covers upstream throttling — WhatsApp answering 429
	// rate-overlimit.
	//
	// It is the ONLY category here that the caller should retry unchanged, and
	// the only reason it is worth its own entry: under CategoryInternal (500)
	// it is indistinguishable from "we broke", and under CategoryValidation
	// (400) the caller would edit a payload that is already correct. Both send
	// the caller to the wrong remedy (F204).
	CategoryRateLimited Category = "rate_limited"

	// CategoryUpstreamRejected covers a well formed, authorized request that
	// the WhatsApp server REFUSED — most often 400 bad-request for a number
	// with no WhatsApp account.
	//
	// It exists because every other category lies about this case, and the
	// measurement that produced F204 shows all three lies:
	//
	//   - CategoryInternal (500), what we did before, says "we broke, retry".
	//     The caller retries forever; the answer never changes.
	//   - CategoryValidation (400) says "fix your payload". The payload is a
	//     well formed phone number. There is nothing to fix.
	//   - CategoryNotFound (404) reads truthfully for the measured case, but it
	//     is US reinterpreting: the server said 400, not 404, and the same 400
	//     covers refusals that are not "absent".
	//
	// 422 says what actually happened: the request was understood and refused
	// by the party upstream, and repeating it unchanged will not help.
	CategoryUpstreamRejected Category = "upstream_rejected"

	// CategoryCapabilityUnsupported covers a well formed, authorized request
	// for an operation THIS SESSION'S ENGINE does not serve.
	//
	// The census at the top of this file says a new category needs a written
	// justification. Here it is: engine-explicit pairing (worktree
	// feature/pairing-explicit-engine) made "the request is fine, and the
	// transport you named cannot do this" a reachable answer for the first
	// time, and every existing category answers it wrongly.
	//
	//   - CategoryValidation (400) says "fix your payload". The payload names
	//     a real engine and a real operation; there is nothing to fix short of
	//     using a different session, which is not a payload edit.
	//   - CategoryConflict (409) says "retry against the right target or after
	//     the state changes". The state will never change: an engine's
	//     capability set is static for the life of the build.
	//   - CategoryNotImplemented (501) is already public contract for "the
	//     USER turned this feature off" on GET /chat/history. Nobody turned
	//     this off.
	//   - CategoryUpstreamRejected (422) has the right STATUS and the wrong
	//     story: it means the WhatsApp server understood and refused. Here
	//     nothing ever left this process — no provider was called at all, and
	//     saying otherwise would send whoever reads the log looking upstream.
	//
	// It shares 422 with CategoryUpstreamRejected on purpose: both are
	// "understood, and will not be served, and repeating it unchanged will not
	// help". The Code is what separates them for anyone who needs to.
	CategoryCapabilityUnsupported Category = "capability_unsupported"

	// CategoryInternal covers everything the caller cannot fix by changing
	// their request: downstream failures, bugs, unexpected state.
	CategoryInternal Category = "internal"
)

// HTTPStatus maps a Category to the HTTP status code it corresponds to.
//
// É CHAMADO, em pkg/presentation/http/response.go:52 — `RespondJSON` IGNORA o
// status que o call site passou quando o erro é um *apperr.AppError, e usa
// este. O comentário anterior dizia "nothing calls it yet, Fase 4a is the one
// that wires it", e ficou desatualizado quando a fiação aconteceu.
//
// Isso não é detalhe de comentário: a entrada da F66 em HOUSEKEEP.md repetiu a
// afirmação daqui e concluiu que a correção exigiria "67 sítios MAIS a ligação
// do HTTPStatus()". A ligação já existia, e o trabalho era metade do
// estimado. Comentário desatualizado vira plano errado.
func (c Category) HTTPStatus() int {
	switch c {
	case CategoryValidation:
		return http.StatusBadRequest
	case CategoryUnauthorized:
		return http.StatusUnauthorized
	case CategoryConflict:
		return http.StatusConflict
	case CategoryNotFound:
		return http.StatusNotFound
	case CategoryNotImplemented:
		return http.StatusNotImplemented
	case CategoryForbidden:
		return http.StatusForbidden
	case CategoryRateLimited:
		return http.StatusTooManyRequests
	case CategoryUpstreamRejected:
		return http.StatusUnprocessableEntity
	case CategoryCapabilityUnsupported:
		return http.StatusUnprocessableEntity
	case CategoryInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// CodeSessionNotConnected marks a request that needs a live WhatsApp transport
// on a session that has none — today, logout (F93).
//
// It lives here, and not in the infra package that raises it, because two
// layers have to agree on it: infra RAISES it, and the use case READS it to
// decide that the local state must be aligned. A code duplicated across layers
// diverges the first time someone edits one of them.
const CodeSessionNotConnected = "session_not_connected"

// IsClientGaveUp reports whether err is the CLIENT abandoning the request —
// a closed browser tab, a navigation, a cancelled fetch.
//
// It exists to keep those out of `error` level. Logging them as errors makes a
// routine tab switch indistinguishable from the database being down: F90 was
// filed after fifteen consecutive `error` lines reading
// "database error: context canceled", all of them a browser cancelling the dev
// panel's polling while the process stayed perfectly healthy. In an
// environment that alerts on log level, a page refresh pages someone at night.
//
// It deliberately does NOT match context.DeadlineExceeded. That one is OUR
// timeout expiring — we promised an answer within a budget and failed to
// deliver — and it stays an error.
func IsClientGaveUp(err error) bool {
	return errors.Is(err, context.Canceled)
}
