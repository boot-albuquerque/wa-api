package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/hlog"

	customhttp "wa-api/pkg/presentation/http"
)

// queryParamEngine is the name of the query parameter carrying the engine on
// the two GET routes of the pairing surface (/session/qr, /session/connect).
// A named constant because it is written in three places — two handlers and the
// OpenAPI source — and a literal repeated is the same bug waiting to diverge.
const queryParamEngine = "engine"

// routeVarTargetSession is the mux route variable naming the session a pairing
// operation acts ON, when a route declares one.
const routeVarTargetSession = "id"

// pairingEngineFromQuery reads the engine a GET pairing request names.
//
// It does NOT trim, lowercase or otherwise repair the value, matching
// domain.ParseEngine's own refusal to do so: a value that needs repairing is a
// value whose author is not sure what they meant, and guessing here would put
// the guess in front of somebody's WhatsApp session. A missing parameter yields
// the empty string, which ParseEngine rejects — that is the invalid_engine path,
// and it is deliberately the same one an unparseable value takes.
func pairingEngineFromQuery(r *http.Request) string {
	return r.URL.Query().Get(queryParamEngine)
}

// pairingTarget returns the session a pairing operation acts ON.
//
// # Why this is not simply the authenticated session
//
// The token authenticates WHO is asking — the actor. The session being paired
// is the target. On every route registered today the two coincide, because every
// pairing route is self-service, and that coincidence is exactly what made the
// original defect invisible: nothing had to be right for the answers to match.
//
// So the two questions are asked separately here, and the answer to "which
// engine serves this?" is derived from the TARGET (pairing.Registry.TargetEngine
// reads the target's persisted row) and never from the actor's context. The
// day a route lets an operator pair a session that is not their own, the engine
// follows the session rather than the caller — with no further change, and with
// TestPairing_UsesTargetEngineNotActorEngine already standing over it.
//
// A route variable wins over the actor when the registered route declares one.
// No route registered in pkg/bootstrap/wiring_routes.go declares `{id}` today,
// so this branch is inert in production by construction — authorising an
// operator to name someone else's session is a separate decision, and this
// function does not make it.
func pairingTarget(r *http.Request, actorID string) string {
	if id := mux.Vars(r)[routeVarTargetSession]; id != "" {
		return id
	}
	return actorID
}

// respondPairingRefusal writes the answer for a request the pairing registry
// refused BEFORE any provider was touched.
//
// The status is not passed as a number: every refusal pairing.Registry produces
// is an apperr.AppError, and customhttp.RespondJSON derives the status from its
// category (pkg/presentation/http/response.go). Passing a literal here would
// create a second place where 400/409/422 is decided, and the two would diverge
// the first time somebody edited one.
//
// The level is Warn and never Error: all four refusals are the caller naming
// something this process will not do, which is a client-caused outcome. Logging
// them at Error would make a mistyped engine indistinguishable from the database
// being down, which is the failure F90 was filed for.
func respondPairingRefusal(w http.ResponseWriter, r *http.Request, handler, targetID string, err error) {
	hlog.FromRequest(r).Warn().Err(err).
		Str("handler", handler).
		Str("target_session_id", targetID).
		Msg("pairing request refused before any provider was called")
	customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
}
