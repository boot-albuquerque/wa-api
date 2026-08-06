package bootstrap

import (
	"os"
	"path/filepath"
	"time"

	"github.com/justinas/alice"
	"github.com/rs/zerolog"
)

type Middleware = alice.Constructor

func (s *server) routes() {
	s.Router = NewRouter(depsFromServer(s))
}

// depsFromServer builds Deps from *server, CLI flags, and package globals —
// the one place router construction still touches process state. Everything
// past this function is pure: NewRouter/buildRouter take only a Deps value.
func depsFromServer(s *server) Deps {
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

	return Deps{
		DB:             s.DB,
		UserCache:      userinfocache,
		AdminToken:     *adminToken,
		StaticDir:      s.ExPath + "/static/",
		Log:            routerLog,
		StartSession:   s.startSession,
		CustomHandlers: customHandlerSet,
	}
}
