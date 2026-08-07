package waLog

// Nomes dos niveis de log. Apareciam como literais em tres lugares cada (a
// chamada de outputf, a chave de colors e a chave de levelToInt), e uma
// divergencia de grafia entre eles nao quebraria a compilacao: o nivel
// simplesmente sairia sem cor e com prioridade -1, ou seja, sempre impresso.
const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

// Codigos ANSI de cor usados quando o logger de stdout roda colorido.
const (
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiReset  = "\033[0m"
)

const (
	// timestampFormat e' o layout do horario no inicio de cada linha.
	timestampFormat = "15:04:05.000"

	// moduleSeparator separa os niveis de sublogger no nome do modulo.
	moduleSeparator = "/"

	// levelUnset e' a prioridade do nivel minimo vazio. Fica ABAIXO de
	// LevelDebug de proposito: nivel minimo nao informado significa
	// "imprime tudo".
	levelUnset = -1
)
