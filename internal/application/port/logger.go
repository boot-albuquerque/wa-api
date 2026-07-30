package port

// Logger abstrai logging estruturado para os usecases.
// A implementação concreta usa zerolog (compatível com o upstream).
type Logger interface {
	Info(msg string, keyvals ...any)
	Warn(msg string, keyvals ...any)
	Error(msg string, keyvals ...any)
}
