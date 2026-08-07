package walog

import (
	"strings"

	"github.com/rs/zerolog"
)

// Valores aceitos pela flag --wadebug, preservados do comportamento
// histórico da flag (que ligava/desligava o logger de stdout do SDK).
const (
	LevelInfo  = "INFO"
	LevelDebug = "DEBUG"
)

// ParseLevel traduz o valor de --wadebug para o piso de nível do bridge.
//
// A flag deixa de ligar/desligar o log do SDK e passa a apenas *baixar* o
// piso: Warn e Error saem sempre, com ou sem flag. Silenciar aviso e erro do
// SDK por default era o bug que este pacote existe para corrigir — um erro de
// descriptografia ou uma desconexão são sinal de produção, não ruído de
// debug. Valor desconhecido cai no default em vez de falhar: uma flag mal
// digitada não pode deixar a aplicação sem log de erro.
func ParseLevel(waDebug string) zerolog.Level {
	switch strings.ToUpper(strings.TrimSpace(waDebug)) {
	case LevelDebug:
		return zerolog.DebugLevel
	case LevelInfo:
		return zerolog.InfoLevel
	default:
		return zerolog.WarnLevel
	}
}
