package whatsmeow

import (
	appport "wa-api/internal/application/contracts"

	"github.com/rs/zerolog"
)

// ZerologAdapter implementa appport.Logger usando zerolog (compatível com upstream).
type ZerologAdapter struct {
	logger zerolog.Logger
}

// NewZerologAdapter cria o adapter de logger.
func NewZerologAdapter(logger zerolog.Logger) *ZerologAdapter {
	return &ZerologAdapter{logger: logger}
}

func (a *ZerologAdapter) Info(msg string, keyvals ...any) {
	a.logger.Info().Fields(keyvals).Msg(msg)
}

func (a *ZerologAdapter) Warn(msg string, keyvals ...any) {
	a.logger.Warn().Fields(keyvals).Msg(msg)
}

func (a *ZerologAdapter) Error(msg string, keyvals ...any) {
	a.logger.Error().Fields(keyvals).Msg(msg)
}

// Verificação em tempo de compilação.
var _ appport.Logger = (*ZerologAdapter)(nil)
