package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"

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

// from resolve o logger de requisição gravado no contexto pela cadeia
// hlog (hlog.NewHandler grava via zerolog.Logger.WithContext, e
// RequestIDHandler adiciona req_id nesse mesmo logger com UpdateContext).
// zerolog.Ctx nunca retorna nil: quando o contexto não tem logger, devolve
// o disabledLogger — daí o teste de nível. Sem logger de requisição (jobs de
// background, testes), cai no logger base do adapter.
func (a *ZerologAdapter) from(ctx context.Context) *zerolog.Logger {
	if ctx == nil {
		return &a.logger
	}
	if l := zerolog.Ctx(ctx); l != nil && l.GetLevel() != zerolog.Disabled {
		return l
	}
	return &a.logger
}

func (a *ZerologAdapter) Info(ctx context.Context, msg string, keyvals ...any) {
	a.from(ctx).Info().Fields(keyvals).Msg(msg)
}

func (a *ZerologAdapter) Warn(ctx context.Context, msg string, keyvals ...any) {
	a.from(ctx).Warn().Fields(keyvals).Msg(msg)
}

func (a *ZerologAdapter) Error(ctx context.Context, msg string, keyvals ...any) {
	a.from(ctx).Error().Fields(keyvals).Msg(msg)
}

// Verificação em tempo de compilação.
var _ appport.Logger = (*ZerologAdapter)(nil)
