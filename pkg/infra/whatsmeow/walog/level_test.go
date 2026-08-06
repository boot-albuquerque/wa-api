package walog

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want zerolog.Level
	}{
		{"", zerolog.WarnLevel},
		{"INFO", zerolog.InfoLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"debug", zerolog.DebugLevel},
		{"  DEBUG  ", zerolog.DebugLevel},
		{"garbage", zerolog.WarnLevel},
		{"TRACE", zerolog.WarnLevel},
	}
	for _, tc := range cases {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, queria %v", tc.in, got, tc.want)
		}
	}
}

// TestParseLevel_NuncaSilenciaErro é a invariante do pacote: nenhum valor de
// --wadebug pode produzir um piso acima de Error.
func TestParseLevel_NuncaSilenciaErro(t *testing.T) {
	for _, in := range []string{"", "INFO", "DEBUG", "garbage", "OFF", "NONE", "FATAL"} {
		if ParseLevel(in) > zerolog.ErrorLevel {
			t.Errorf("ParseLevel(%q) = %v, silenciaria erros do SDK", in, ParseLevel(in))
		}
	}
}
