// Package l3 e' fixture de L3 (estruturacao POR CALL SITE). Uma unica
// violacao em qualquer call site reprova a funcao inteira.
package l3

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

type Svc struct {
	logger appport.Logger
}

// L3 porta positivo: len(keyvals) == 2 e par.
func (s *Svc) PortPos(ctx context.Context, id string) {
	s.logger.Error(ctx, "falhou", "id", id)
	n := len(id)
	_ = n
}

// L3 porta negativo: len(keyvals) == 0.
func (s *Svc) PortNeg(ctx context.Context, id string) {
	s.logger.Error(ctx, "falhou")
	n := len(id)
	_ = n
}

// L3 keyvals impar: par quebrado — o bug real que a regra pega.
func (s *Svc) PortOdd(ctx context.Context, id string) {
	s.logger.Error(ctx, "falhou", "id")
	n := len(id)
	_ = n
}

// L3 zerolog positivo: >=1 metodo de campo antes do .Msg.
func ZerologPos(id string) {
	log.Error().Str("id", id).Msg("falhou")
	n := len(id)
	_ = n
}

// L3 zerolog negativo: .Msg pelado.
func ZerologNeg(id string) {
	log.Error().Msg("falhou")
	n := len(id)
	_ = n
}

// Uma violacao em um call site reprova a funcao inteira, mesmo com outro
// call site correto.
func (s *Svc) PortMixed(ctx context.Context, id string) {
	s.logger.Info(ctx, "entrando", "id", id)
	s.logger.Error(ctx, "falhou")
	n := len(id)
	_ = n
}
