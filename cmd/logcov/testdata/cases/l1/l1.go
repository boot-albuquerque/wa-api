// Package l1 e' fixture do numerador L1 (presenca) — as tres formas
// reconhecidas e seus negativos (METRIC.md 9.1.3).
package l1

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

type Svc struct {
	logger appport.Logger
}

// L1-a positivo: porta contracts.Logger, ctx como primeiro argumento.
func (s *Svc) PortPos(ctx context.Context, id string) {
	s.logger.Info(ctx, "processando", "id", id)
	n := len(id)
	_ = n
}

// L1-a negativo: mesma funcao, sem chamada de log nenhuma.
func (s *Svc) PortNeg(ctx context.Context, id string) {
	_ = ctx
	n := len(id)
	_ = n
}

// L1-b positivo: cadeia zerolog terminando em .Msg.
func ZerologPos(id string) {
	log.Info().Str("id", id).Msg("processando")
	n := len(id)
	_ = n
}

// L1-b negativo: cadeia zerolog sem terminacao .Msg/.Msgf/.Send.
func ZerologNeg(id string) {
	e := log.Info().Str("id", id)
	_ = e
	n := len(id)
	_ = n
}

// L1-c positivo: hlog.FromRequest.
func HlogPos(r *http.Request) {
	hlog.FromRequest(r).Info().Str("path", r.URL.Path).Msg("requisicao")
	n := len(r.URL.Path)
	_ = n
}

// L1-c negativo: hlog sem terminacao.
func HlogNeg(r *http.Request) {
	e := hlog.FromRequest(r).Info().Str("path", r.URL.Path)
	_ = e
	n := len(r.URL.Path)
	_ = n
}
