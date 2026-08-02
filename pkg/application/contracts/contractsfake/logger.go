package contractsfake

import (
	"context"
	"sync"

	port "wa-api/pkg/application/contracts"
)

// Níveis gravados por Logger. São os três métodos da porta.
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// LogRecord é uma chamada de log capturada, com tudo que o call site
// carregava: nível, contexto, mensagem e os keyvals estruturados.
//
// Keyvals é guardado por cópia — o variádico do chamador é reaproveitável, e
// guardar o slice original faria registros distintos apontarem para o mesmo
// array.
type LogRecord struct {
	Level   string
	Ctx     context.Context
	Msg     string
	Keyvals []any
}

// IsStructured aplica a regra L3 de cmd/logcov/METRIC.md na forma da porta:
// número PAR de keyvals e no mínimo 2.
func (r LogRecord) IsStructured() bool {
	return len(r.Keyvals) >= 2 && len(r.Keyvals)%2 == 0
}

// Keyval devolve o valor associado a key, procurando nos pares (k, v) de
// Keyvals. A chave só casa quando é string. Devolve a PRIMEIRA ocorrência.
func (r LogRecord) Keyval(key string) (any, bool) {
	for i := 0; i+1 < len(r.Keyvals); i += 2 {
		if k, ok := r.Keyvals[i].(string); ok && k == key {
			return r.Keyvals[i+1], true
		}
	}
	return nil, false
}

// HasKey reporta se key aparece como chave em Keyvals.
func (r LogRecord) HasKey(key string) bool {
	_, ok := r.Keyval(key)
	return ok
}

// Logger é o fake de port.Logger, e o mecanismo do co-gate D: além de
// registrar QUE houve log, guarda os keyvals para que o teste assere QUAIS
// campos estruturados o registro carregava.
//
// É o único fake do pacote seguro para uso concorrente.
type Logger struct {
	mu      sync.Mutex
	records []LogRecord

	// InfoFunc, WarnFunc e ErrorFunc, quando não-nil, rodam DEPOIS da
	// gravação. Servem para o teste que precisa reagir ao log (falhar na
	// hora, sinalizar um canal), não para suprimi-lo.
	InfoFunc  func(ctx context.Context, msg string, keyvals ...any)
	WarnFunc  func(ctx context.Context, msg string, keyvals ...any)
	ErrorFunc func(ctx context.Context, msg string, keyvals ...any)
}

var _ port.Logger = (*Logger)(nil)

func (f *Logger) record(level string, ctx context.Context, msg string, keyvals []any) {
	cp := make([]any, len(keyvals))
	copy(cp, keyvals)
	f.mu.Lock()
	f.records = append(f.records, LogRecord{Level: level, Ctx: ctx, Msg: msg, Keyvals: cp})
	f.mu.Unlock()
}

// Info implementa port.Logger.
func (f *Logger) Info(ctx context.Context, msg string, keyvals ...any) {
	f.record(LevelInfo, ctx, msg, keyvals)
	if f.InfoFunc != nil {
		f.InfoFunc(ctx, msg, keyvals...)
	}
}

// Warn implementa port.Logger.
func (f *Logger) Warn(ctx context.Context, msg string, keyvals ...any) {
	f.record(LevelWarn, ctx, msg, keyvals)
	if f.WarnFunc != nil {
		f.WarnFunc(ctx, msg, keyvals...)
	}
}

// Error implementa port.Logger.
func (f *Logger) Error(ctx context.Context, msg string, keyvals ...any) {
	f.record(LevelError, ctx, msg, keyvals)
	if f.ErrorFunc != nil {
		f.ErrorFunc(ctx, msg, keyvals...)
	}
}

// Records devolve uma cópia de todos os registros, na ordem em que
// aconteceram.
func (f *Logger) Records() []LogRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]LogRecord, len(f.records))
	copy(out, f.records)
	return out
}

// Len devolve quantos registros houve.
func (f *Logger) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

// Reset descarta os registros acumulados.
func (f *Logger) Reset() {
	f.mu.Lock()
	f.records = nil
	f.mu.Unlock()
}

// ByLevel devolve os registros de um nível (use LevelInfo/LevelWarn/LevelError).
func (f *Logger) ByLevel(level string) []LogRecord {
	var out []LogRecord
	for _, r := range f.Records() {
		if r.Level == level {
			out = append(out, r)
		}
	}
	return out
}

// Messages devolve as mensagens, na ordem, sem nível nem keyvals. É o
// equivalente do slice de strings que os testes ad-hoc mantinham antes deste
// pacote.
func (f *Logger) Messages() []string {
	recs := f.Records()
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Msg
	}
	return out
}

// Find devolve o primeiro registro com a mensagem msg.
func (f *Logger) Find(msg string) (LogRecord, bool) {
	for _, r := range f.Records() {
		if r.Msg == msg {
			return r, true
		}
	}
	return LogRecord{}, false
}

// FindLevel devolve o primeiro registro com o nível e a mensagem informados.
func (f *Logger) FindLevel(level, msg string) (LogRecord, bool) {
	for _, r := range f.Records() {
		if r.Level == level && r.Msg == msg {
			return r, true
		}
	}
	return LogRecord{}, false
}

// Logged reporta se houve algum registro com a mensagem msg.
func (f *Logger) Logged(msg string) bool {
	_, ok := f.Find(msg)
	return ok
}

// Last devolve o registro mais recente.
func (f *Logger) Last() (LogRecord, bool) {
	recs := f.Records()
	if len(recs) == 0 {
		return LogRecord{}, false
	}
	return recs[len(recs)-1], true
}
