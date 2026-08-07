package waLog

import (
	"github.com/rs/zerolog"
)

type zeroLogger struct {
	mod string
	zerolog.Logger
}

// Zerolog wraps a [zerolog.Logger] to implement the [Logger] interface.
//
// Subloggers will be created by setting the `sublogger` field in the log context.
func Zerolog(log zerolog.Logger) Logger {
	return &zeroLogger{Logger: log}
}

func (z *zeroLogger) Warnf(msg string, args ...any)  { z.Warn().Msgf(msg, args...) }
func (z *zeroLogger) Errorf(msg string, args ...any) { z.Error().Msgf(msg, args...) }
func (z *zeroLogger) Infof(msg string, args ...any)  { z.Info().Msgf(msg, args...) }
func (z *zeroLogger) Debugf(msg string, args ...any) { z.Debug().Msgf(msg, args...) }
func (z *zeroLogger) Sub(module string) Logger {
	if z.mod != "" {
		module = z.mod + moduleSeparator + module
	}
	return &zeroLogger{mod: module, Logger: z.Logger.With().Str("sublogger", module).Logger()}
}

var _ Logger = &zeroLogger{}
