// Package waLog contains a simple logger interface used by the other wa-noise packages.
package waLog

import (
	"fmt"
	"strings"
	"time"
)

// Logger is a simple logger interface that can have subloggers for specific areas.
type Logger interface {
	Warnf(msg string, args ...interface{})
	Errorf(msg string, args ...interface{})
	Infof(msg string, args ...interface{})
	Debugf(msg string, args ...interface{})
	Sub(module string) Logger
}

type noopLogger struct{}

func (n *noopLogger) Errorf(_ string, _ ...interface{}) {}
func (n *noopLogger) Warnf(_ string, _ ...interface{})  {}
func (n *noopLogger) Infof(_ string, _ ...interface{})  {}
func (n *noopLogger) Debugf(_ string, _ ...interface{}) {}
func (n *noopLogger) Sub(_ string) Logger               { return n }

// Noop is a no-op Logger implementation that silently drops everything.
var Noop Logger = &noopLogger{}

type stdoutLogger struct {
	mod   string
	color bool
	min   int
}

var colors = map[string]string{
	LevelInfo:  ansiCyan,
	LevelWarn:  ansiYellow,
	LevelError: ansiRed,
}

// levelToInt ordena os niveis para a comparacao com o minimo configurado.
var levelToInt = map[string]int{
	"":         levelUnset,
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

func (s *stdoutLogger) outputf(level, msg string, args ...interface{}) {
	if levelToInt[level] < s.min {
		return
	}
	var colorStart, colorReset string
	if s.color {
		colorStart = colors[level]
		colorReset = ansiReset
	}
	fmt.Printf("%s%s [%s %s] %s%s\n", time.Now().Format(timestampFormat), colorStart, s.mod, level, fmt.Sprintf(msg, args...), colorReset)
}

func (s *stdoutLogger) Errorf(msg string, args ...interface{}) { s.outputf(LevelError, msg, args...) }
func (s *stdoutLogger) Warnf(msg string, args ...interface{})  { s.outputf(LevelWarn, msg, args...) }
func (s *stdoutLogger) Infof(msg string, args ...interface{})  { s.outputf(LevelInfo, msg, args...) }
func (s *stdoutLogger) Debugf(msg string, args ...interface{}) { s.outputf(LevelDebug, msg, args...) }
func (s *stdoutLogger) Sub(mod string) Logger {
	return &stdoutLogger{mod: s.mod + moduleSeparator + mod, color: s.color, min: s.min}
}

// Stdout is a simple Logger implementation that outputs to stdout. The module name given is included in log lines.
//
// minLevel specifies the minimum log level to output. An empty string will output all logs.
//
// If color is true, then info, warn and error logs will be colored cyan, yellow and red respectively using ANSI color escape codes.
func Stdout(module string, minLevel string, color bool) Logger {
	return &stdoutLogger{mod: module, color: color, min: levelToInt[strings.ToUpper(minLevel)]}
}
