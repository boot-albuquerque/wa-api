package walog

import (
	"fmt"

	"github.com/rs/zerolog"

	waLog "wa-api/internal/wa-noise/util/log"
)

// Bridge implementa waLog.Logger emitindo no zerolog da aplicação.
//
// Todo registro carrega FieldModule com o módulo do SDK que o emitiu, de
// forma que "Client/Recv" e "Database" sejam filtráveis no sink sem parsing
// de mensagem.
type Bridge struct {
	logger zerolog.Logger
	module string
	min    zerolog.Level
}

// New devolve o bridge de module sobre base, emitindo a partir de min.
//
// module vazio vira ModuleRoot: um campo wa_module vazio no JSON seria pior
// que um genérico, porque não dá para distinguir de ausência de campo.
func New(base zerolog.Logger, module string, min zerolog.Level) *Bridge {
	if module == "" {
		module = ModuleRoot
	}
	return &Bridge{logger: base, module: module, min: min}
}

// format aplica os verbos de Sprintf só quando há argumentos. args vazio
// devolve msg literal: o SDK também chama estes métodos sem verbos, e passar
// por Sprintf transformaria um "100% done" em "100%!d(MISSING)one".
func format(msg string, args []any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// Os quatro métodos abaixo repetem a cadeia zerolog em vez de delegarem a um
// helper com WithLevel: o nível precisa ser o método nomeado (Error(), Warn()
// …) para que o log continue rastreável até o call site pelas mesmas regras
// que valem no resto do repositório. O teste de piso vem antes de format
// porque Sprintf no caminho de Debug desligado — o default, e onde o
// whatsmeow é mais verboso — seria custo puro.

func (b *Bridge) Errorf(msg string, args ...any) {
	if zerolog.ErrorLevel < b.min {
		return
	}
	b.logger.Error().Str(FieldModule, b.module).Msg(format(msg, args))
}

func (b *Bridge) Warnf(msg string, args ...any) {
	if zerolog.WarnLevel < b.min {
		return
	}
	b.logger.Warn().Str(FieldModule, b.module).Msg(format(msg, args))
}

func (b *Bridge) Infof(msg string, args ...any) {
	if zerolog.InfoLevel < b.min {
		return
	}
	b.logger.Info().Str(FieldModule, b.module).Msg(format(msg, args))
}

func (b *Bridge) Debugf(msg string, args ...any) {
	if zerolog.DebugLevel < b.min {
		return
	}
	b.logger.Debug().Str(FieldModule, b.module).Msg(format(msg, args))
}

// Sub devolve um bridge para um submódulo, herdando sink e piso. O SDK chama
// isto na construção do cliente (internal/wa-noise/client.go) para separar
// Recv/Send/Pair, e o resultado tem de continuar sendo um Bridge — devolver
// waLog.Noop aqui reintroduziria o silêncio que este pacote elimina.
func (b *Bridge) Sub(module string) waLog.Logger {
	if module == "" {
		return b
	}
	return &Bridge{
		logger: b.logger,
		module: b.module + SubSeparator + module,
		min:    b.min,
	}
}

// Verificação em tempo de compilação, no mesmo padrão de logger.go.
var _ waLog.Logger = (*Bridge)(nil)
