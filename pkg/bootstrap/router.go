package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	appport "wa-api/pkg/application/contracts"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"

	"github.com/rs/zerolog/log"
)

// Deps is everything NewRouter/Routes need to build the router: no CLI
// flag parsing, no executable-path lookup, no package-level globals. See
// depsFromServer for how this is populated at runtime from *server.
type Deps struct {
	DB         *sqlx.DB
	UserCache  *cache.Cache
	AdminToken string
	StaticDir  string // empty => don't register the FileServer
	Log        zerolog.Logger

	// StartClient injects (*server).startClient (lifecycle.go). By the time
	// NewRouter runs, it's already bound into CustomHandlers.Session.Connect
	// (see initConnectHandler) — this field exists purely so NewRouter can
	// fail loud if the caller forgot to wire it, instead of nil-derefing on
	// the first /session/connect request. Field func: this is dynamic
	// coupling the compiler doesn't cover (§1.3, fifth row of the
	// verification-mechanism table).
	StartClient func(userID, textjid, token string, kill chan bool)

	// CustomHandlers backs the 87 custom routes. Required by NewRouter.
	// Routes() supplies a zero-value one internally when omitted, since
	// route enumeration only walks the mux tree and never calls a
	// handler's ServeHTTP.
	CustomHandlers *customHandlers
}

// RouteInfo is one registered route, as returned by Routes.
type RouteInfo struct {
	Path    string
	Methods []string
}

// NewRouter builds the mux.Router from Deps alone. Fails loud (panic) on
// missing required fields instead of nil-derefing on the first request —
// the same guard shape the media fix in wiring_delegates.go established
// for the exact same class of defect (a struct field never populated).
func NewRouter(d Deps) *mux.Router {
	if d.StartClient == nil {
		panic("bootstrap.NewRouter: Deps.StartClient is required")
	}
	if d.CustomHandlers == nil {
		panic("bootstrap.NewRouter: Deps.CustomHandlers is required")
	}
	if d.UserCache == nil {
		// Unlike StartClient (bound into CustomHandlers before Deps is
		// built, not read here), UserCache IS read directly by buildRouter
		// below (authAlice(d.DB.DB, d.UserCache)), and middleware/auth.go's
		// AuthAlice calls userCache.Get(token) unconditionally as its first
		// substantive operation — a nil *cache.Cache panics there on the
		// first authenticated request. This is the field whose nil actually
		// reproduces the wiring_delegates.go:64 defect class; guard it.
		panic("bootstrap.NewRouter: Deps.UserCache is required")
	}
	return buildRouter(d)
}

// Routes enumerates {path, methods} for every registered route without
// requiring a fully-wired CustomHandlers or StartClient. This is what
// cmd/listroutes calls, and what the golden-file harness in F4a diffs
// against — before this phase, enumerating routes from outside
// pkg/bootstrap was not possible: the registration loop lived inside the
// unexported (*server).registerCustomRoutes.
func Routes(d Deps) []RouteInfo {
	if d.DB == nil {
		// Only Deps.DB.DB gets read (by authAlice, as *sql.DB), never
		// invoked as a live connection during registration or Walk.
		d.DB = &sqlx.DB{}
	}
	if d.CustomHandlers == nil {
		d.CustomHandlers = emptyCustomHandlers()
	}
	if d.StartClient == nil {
		d.StartClient = func(string, string, string, chan bool) {}
	}
	router := buildRouter(d)
	var infos []RouteInfo
	_ = router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, _ := route.GetMethods()
		infos = append(infos, RouteInfo{Path: path, Methods: methods})
		return nil
	})
	return infos
}

// emptyCustomHandlers returns a customHandlers with every handler group
// populated (never nil) but every leaf handler at its zero value. Route
// registration accesses several leaf fields directly (e.g.
// Misc.DeleteUserComplete), and a field read through a nil GROUP pointer
// panics regardless of whether the handler is ever invoked — unlike a
// method call on a nil receiver (e.g. User.ListUsers()), which is safe as
// long as the method body doesn't dereference the receiver eagerly. Safe
// for route enumeration, which only wraps leaf handlers, never calls
// ServeHTTP on them.
func emptyCustomHandlers() *customHandlers {
	return &customHandlers{
		Message:   &MessageHandlers{},
		Session:   &SessionHandlers{},
		Webhook:   &WebhookHandlers{},
		User:      &handlers.UserHandlers{},
		Group:     &handlers.GroupHandlers{},
		Storage:   &handlers.StorageHandlers{},
		Misc:      &handlers.MiscHandlers{},
		Blocklist: &handlers.BlocklistHandlers{},
		Download:  &handlers.DownloadHandlers{},
		Presence:  &handlers.PresenceHandlers{},
		Reaction:  &handlers.ReactionHandlers{},
		Contact:   &handlers.ContactHandlers{},
		GroupMgmt: &handlers.GroupManagementHandlers{},
	}
}

// recoverMiddleware is the outermost middleware on the router: it must run
// before authAlice/authAdmin, because AuthAlice.db.Query panics if Deps.DB
// carries a nil *sql.DB, and a recover appended after auth would not cover
// that. Registered via router.Use,
// not alice.Chain, specifically so it wraps admin routes too (their own
// middleware is mux.Router.Use(authAdmin(...)) on a subrouter, not part of
// the alice chain `c` below).
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reportPanic(w, r, rec, log.Error())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// innerRecoverMiddleware is the SECOND recover layer, and both layers are
// load-bearing — do not "simplify" either one away:
//
//   - The OUTER recoverMiddleware (router.Use) is the only one that covers
//     panics raised outside the boundary-logging stack: /admin/* routes
//     (registered on their own subrouter), and panics raised by the
//     hlog/alice machinery itself (if hlog.NewHandler or RequestIDHandler
//     panicked, this inner recover — which lives DOWNSTREAM of them — would
//     never run).
//   - This INNER recover is appended immediately after hlog.AccessHandler,
//     i.e. it sits INSIDE AccessHandler. That placement fixes two defects the
//     outer recover alone cannot:
//     (1) AccessHandler's deferred callback closes over its own
//     ResponseWriter wrapper. With only the outer recover, a panic unwound
//     past AccessHandler's defer BEFORE anything wrote a status, so the
//     callback observed lw.Status()==0 and outcomeFor(0) logged "success"
//     for a request that actually returned 500. Writing the 500 here, before
//     this frame returns, means the callback observes 500 and correctly logs
//     outcome="server_error".
//     (2) The outer recover logs through the package-global zerolog, which
//     carries no req_id, so the panic record could not be correlated with the
//     boundary record for the same request. This one logs through
//     hlog.FromRequest(r), picking up req_id like every other request-scoped
//     record.
func innerRecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reportPanic(w, r, rec, hlog.FromRequest(r).Error())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// reportPanic is the shared body of both recover layers: it writes the panic
// record into the supplied (already level-bound) event and responds 500. Only
// the logger differs between the two callers — the response shape must not.
func reportPanic(w http.ResponseWriter, r *http.Request, rec any, ev *zerolog.Event) {
	ev.
		Interface("panic", rec).
		Str("method", r.Method).
		Stringer("url", r.URL).
		Msg("recovered from panic in HTTP handler")
	customhttp.RespondJSON(w, http.StatusInternalServerError, nil, fmt.Errorf("panic: %v", rec))
}

// boundaryLogMiddlewares is the request-scoped logging stack, shared by the
// alice chain `c` (which then appends authAlice) and by the /admin subrouter
// and the 404/405 handlers (which do not). Order is outermost-first.
func boundaryLogMiddlewares(l zerolog.Logger) []alice.Constructor {
	return []alice.Constructor{
		hlog.NewHandler(l),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RefererHandler("referer"),
		userIDHolderHandler,
		hlog.AccessHandler(writeBoundaryRecord),
		innerRecoverMiddleware,
	}
}

// writeBoundaryRecord is hlog.AccessHandler's callback: the one boundary
// record per request.
func writeBoundaryRecord(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Str("method", r.Method).
		Stringer("url", r.URL).
		Str("route", accessLogRoute(r)).
		Int("status", status).
		Int("size", size).
		Dur("duration", duration).
		Float64("duration_ms", float64(duration.Nanoseconds())/float64(time.Millisecond)).
		Str("outcome", outcomeFor(status)).
		Str("userid", accessLogUserID(r)).
		Msg("Got API Request")
}

func buildRouter(d Deps) *mux.Router {
	router := mux.NewRouter()
	router.Use(recoverMiddleware)
	router.Use(bodyLimitMiddleware)
	router.Use(newRateLimitObserver().middleware)

	// DECISION: unmatched routes (404) and wrong-method requests (405) COUNT
	// as boundary-logged requests. A scanner sweep across hundreds of unknown
	// paths is exactly the traffic an operator needs visibility into, and it
	// would be inconsistent to log a 401 (auth failure on a KNOWN route) but
	// not a 404 (a probe against an UNKNOWN one). These two handlers must be
	// wrapped explicitly: mux assigns NotFoundHandler/MethodNotAllowedHandler
	// WITHOUT applying router.Use middlewares (mux@v1.8.1 mux.go:151-165 —
	// the middleware loop only runs on the matched-route branch). No
	// authAlice here: there is no route or user to authenticate against.
	router.NotFoundHandler = alice.New(boundaryLogMiddlewares(d.Log)...).Then(http.NotFoundHandler())
	router.MethodNotAllowedHandler = alice.New(boundaryLogMiddlewares(d.Log)...).Then(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		}))

	// Admin routes — authAdmin middleware validates the admin token.
	//
	// DECISION: /admin/* IS boundary-logged. These are the highest-privilege
	// operations in the system (list/create/edit/delete users), so audit
	// visibility matters here more than anywhere else. Registered BEFORE
	// authAdmin because mux applies middlewares outermost-first in
	// registration order (middleware.go:24-27 appends; mux.go:143-145 wraps in
	// reverse) — so the logging stack encloses authAdmin and a 401 from a bad
	// admin token still produces its boundary record, matching the 401
	// decision already made for the alice chain. Calling Use twice chains both
	// sets; it does not replace.
	adminRoutes := router.PathPrefix("/admin").Subrouter()
	for _, mw := range boundaryLogMiddlewares(d.Log) {
		adminRoutes.Use(mux.MiddlewareFunc(mw))
	}
	adminRoutes.Use(authAdmin(d.AdminToken))
	adminRoutes.Handle("/users", d.CustomHandlers.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", d.CustomHandlers.User.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", d.CustomHandlers.Misc.DeleteUserComplete).Methods("DELETE")

	// Chain order matters, and alice.Chain.Then applies constructors so the
	// FIRST appended one is OUTERMOST (verified against alice/chain.go:45-55).
	// Two properties this order buys, both of which the previous order broke:
	//
	//  1. authAlice is now INNERMOST, not outermost. Before, a request
	//     rejected by auth returned before hlog ever entered the chain, so a
	//     401 produced no boundary record at all — precisely the signal an
	//     operator needs when a token is wrong. A 401 IS a request: it must
	//     be logged. authAlice still gates every route it wraps; only the
	//     logging wrapper moved to enclose it.
	//  2. RequestIDHandler runs BEFORE AccessHandler's callback fires, so
	//     req_id is already on the context logger when the boundary line is
	//     written (AccessHandler defers its callback until after the inner
	//     handler returns, but it resolves the logger from the request it was
	//     handed — which must already carry req_id).
	c := alice.New(boundaryLogMiddlewares(d.Log)...)

	c = c.Append(authAlice(d.DB.DB, d.UserCache))
	c = c.Append(recordUserIDHandler)

	// /livez is the container-level liveness probe: no auth, no DB query,
	// no runtime.ReadMemStats — cheap enough to hit unauthenticated on
	// every HEALTHCHECK tick without becoming a DoS amplifier. /health
	// stays behind auth (registered below via registerCustomRoutes): it
	// fans out to a DB COUNT(*) and ReadMemStats and returns sizing/version
	// data that must not be public.
	router.Handle("/livez", alice.New().Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))).Methods("GET")

	registerCustomRoutes(router, c, d.CustomHandlers)

	if d.StaticDir != "" {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir(d.StaticDir)))
	}

	return router
}

// accessLogUserID reads the request's userinfo in comma-ok form. This
// replaces the repo's only naked type assertion on context.Value (33 of 43
// context.Value reads use a typed key in comma-ok form; this was the sole
// exception, per the Fase 1a dynamic-coupling audit). It was unreachable
// with a nil value in production (authAlice always populates or returns
// 401 first), but became a trap the moment the golden harness assembled
// the router with different chains — exactly what this phase does.
// userIDHolderKey / userIDHolder carry the authenticated user id ACROSS the
// boundary-logging wrapper. Necessary because authAlice now runs INSIDE
// hlog.AccessHandler: authAlice publishes the userinfo by deriving a new
// request (r.WithContext), which only downstream handlers observe, while
// AccessHandler's deferred callback still holds the request it was handed.
// The holder is a per-request pointer installed outside AccessHandler and
// filled just inside authAlice, so both sides see the same value without
// the logging wrapper having to move back outside auth (which would cost
// the 401 boundary record this phase exists to add).
type userIDHolderKey struct{}

type userIDHolder struct{ id string }

// userIDHolderHandler installs the holder. Appended before AccessHandler.
func userIDHolderHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userIDHolderKey{}, &userIDHolder{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recordUserIDHandler fills the holder. Appended immediately after
// authAlice, so it only ever runs once auth has succeeded — a rejected
// request leaves the holder empty, which is the correct value for it.
// Single-goroutine per request: written here, read by the AccessHandler
// callback after the inner handler returns.
func recordUserIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := r.Context().Value(userIDHolderKey{}).(*userIDHolder); ok {
			h.id = userInfoID(r)
		}
		next.ServeHTTP(w, r)
	})
}

// accessLogRoute returns the NORMALIZED route template ("/user/{id}") rather
// than the resolved URL ("/user/golden-test-id"), so boundary records
// aggregate per route instead of exploding into one cardinality bucket per
// path parameter value. Falls back to the resolved path when the request
// matched no route (404) or the route carries no path template — never
// panics on a nil route.
func accessLogRoute(r *http.Request) string {
	if route := mux.CurrentRoute(r); route != nil {
		if tmpl, err := route.GetPathTemplate(); err == nil && tmpl != "" {
			return tmpl
		}
	}
	return r.URL.Path
}

// outcomeFor buckets an HTTP status into the three outcomes an operator
// actually alerts on. 4xx is the client's fault and must not read as an
// incident; 5xx is ours.
func outcomeFor(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "success"
	}
}

func accessLogUserID(r *http.Request) string {
	if h, ok := r.Context().Value(userIDHolderKey{}).(*userIDHolder); ok && h.id != "" {
		return h.id
	}
	return userInfoID(r)
}

// userInfoID reads the request's userinfo in comma-ok form.
func userInfoID(r *http.Request) string {
	v, ok := r.Context().Value(appport.UserInfoKey).(interface{ Get(string) string })
	if !ok {
		return ""
	}
	return v.Get("Id")
}
