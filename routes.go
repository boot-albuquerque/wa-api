package wuzapi

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/justinas/alice"
	"github.com/rs/zerolog"
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
	if s.mode == Stdio {
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
	adminRoutes := s.router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(authAdmin(*adminToken))
	adminRoutes.Handle("/users", customHandlerSet.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", customHandlerSet.User.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", customHandlerSet.User.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", customHandlerSet.Misc.DeleteUserComplete).Methods("DELETE")

	c := alice.New()
	c = c.Append(authAlice(s.db.DB, userinfocache))
	c = c.Append(hlog.NewHandler(routerLog))

	c = c.Append(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Str("method", r.Method).
			Stringer("url", r.URL).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Str("userid", r.Context().Value("userinfo").(Values).Get("Id")).
			Msg("Got API Request")
	}))

	c = c.Append(hlog.RemoteAddrHandler("ip"))
	c = c.Append(hlog.UserAgentHandler("user_agent"))
	c = c.Append(hlog.RefererHandler("referer"))
	c = c.Append(hlog.RequestIDHandler("req_id", "Request-Id"))

	// All non-admin routes are now registered via registerCustomRoutes
	s.registerCustomRoutes(c)

	// Static files
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir(exPath + "/static/")))
}
