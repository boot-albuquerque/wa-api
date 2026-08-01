package bootstrap

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/justinas/alice"
	"github.com/rs/zerolog"

	appport "wa-api/pkg/application/contracts"
	"github.com/rs/zerolog/hlog"
)

type Middleware = alice.Constructor

func (s *server) routes() {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	var routerLog zerolog.Logger
	logOutput := os.Stdout
	if s.Mode == Stdio {
		logOutput = os.Stderr
	}
	if *logType == "json" {
		routerLog = zerolog.New(logOutput).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Str("host", *address).
			Logger()
	} else {
		output := zerolog.ConsoleWriter{
			Out:        logOutput,
			TimeFormat: time.RFC3339,
			NoColor:    !*colorOutput,
		}
		routerLog = zerolog.New(output).
			With().
			Timestamp().
			Str("role", filepath.Base(os.Args[0])).
			Str("host", *address).
			Logger()
	}

	// Admin routes — authAdmin middleware (standalone in auth.go) validates the
	// admin token. Internal handlers from customHandlerSet handle the logic.
	adminRoutes := s.Router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(authAdmin(*adminToken))
	adminRoutes.Handle("/users", customHandlerSet.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", customHandlerSet.User.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", customHandlerSet.Misc.DeleteUserComplete).Methods("DELETE")

	c := alice.New()
	c = c.Append(authAlice(s.DB.DB, userinfocache))
	c = c.Append(hlog.NewHandler(routerLog))

	c = c.Append(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Str("method", r.Method).
			Stringer("url", r.URL).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Str("userid", r.Context().Value(appport.UserInfoKey).(interface{ Get(string) string }).Get("Id")).
			Msg("Got API Request")
	}))

	c = c.Append(hlog.RemoteAddrHandler("ip"))
	c = c.Append(hlog.UserAgentHandler("user_agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(hlog.RequestIDHandler("req_id", "Request-Id"))

	// /livez is the container-level liveness probe: no auth, no DB query, no
	// runtime.ReadMemStats — cheap enough to hit unauthenticated on every
	// HEALTHCHECK tick without becoming a DoS amplifier. /health (below) stays
	// behind auth: it fans out to a DB COUNT(*) and ReadMemStats and returns
	// sizing/version data that must not be public.
	s.Router.Handle("/livez", alice.New().Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))).Methods("GET")

	// All non-admin routes are now registered via registerCustomRoutes
	s.registerCustomRoutes(c)

	// Static files
	s.Router.PathPrefix("/").Handler(http.FileServer(http.Dir(exPath + "/static/")))
}
