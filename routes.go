package main

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

	// Admin routes (still registered directly — not yet migrated to internal)
	adminRoutes := s.router.PathPrefix("/admin").Subrouter()
	adminRoutes.Use(s.authadmin)
	adminRoutes.Handle("/users", s.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", s.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", s.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", s.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", s.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", s.DeleteUserComplete()).Methods("DELETE")

	c := alice.New()
	c = c.Append(s.authalice)
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
