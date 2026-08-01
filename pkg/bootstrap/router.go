package bootstrap

import (
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
		Extended:  &handlers.ExtendedHandlers{},
		GroupMgmt: &handlers.GroupManagementHandlers{},
	}
}

// recoverMiddleware is the outermost middleware on the router: it must run
// before authAlice/authAdmin, because both have a panic reachable inside
// them (middleware/auth.go's RespondJSON panics on encode failure, and
// AuthAlice.db.Query panics if Deps.DB carries a nil *sql.DB). A recover
// appended after auth would not cover either. Registered via router.Use,
// not alice.Chain, specifically so it wraps admin routes too (their own
// middleware is mux.Router.Use(authAdmin(...)) on a subrouter, not part of
// the alice chain `c` below).
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().
					Interface("panic", rec).
					Str("method", r.Method).
					Stringer("url", r.URL).
					Msg("recovered from panic in HTTP handler")
				customhttp.RespondJSON(w, http.StatusInternalServerError, nil, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func buildRouter(d Deps) *mux.Router {
	router := mux.NewRouter()
	router.Use(recoverMiddleware)
	router.Use(bodyLimitMiddleware)
	router.Use(newRateLimitObserver().middleware)

	// Admin routes — authAdmin middleware validates the admin token.
	adminRoutes := router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(authAdmin(d.AdminToken))
	adminRoutes.Handle("/users", d.CustomHandlers.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", d.CustomHandlers.User.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", d.CustomHandlers.User.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", d.CustomHandlers.Misc.DeleteUserComplete).Methods("DELETE")

	c := alice.New()
	c = c.Append(authAlice(d.DB.DB, d.UserCache))
	c = c.Append(hlog.NewHandler(d.Log))

	c = c.Append(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Str("method", r.Method).
			Stringer("url", r.URL).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Str("userid", accessLogUserID(r)).
			Msg("Got API Request")
	}))

	c = c.Append(hlog.RemoteAddrHandler("ip"))
	c = c.Append(hlog.UserAgentHandler("user_agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(hlog.RequestIDHandler("req_id", "Request-Id"))

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
func accessLogUserID(r *http.Request) string {
	v, ok := r.Context().Value(appport.UserInfoKey).(interface{ Get(string) string })
	if !ok {
		return ""
	}
	return v.Get("Id")
}
