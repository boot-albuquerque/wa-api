package domain

import "github.com/rs/zerolog"

const redactedPlaceholder = "[REDACTED]"

// Secret guarda um valor sensível cuja representação textual, JSON e de log é
// sempre redigida. O valor original só sai por Reveal.
type Secret string

func (s Secret) String() string { return redactedPlaceholder }

func (s Secret) GoString() string { return redactedPlaceholder }

func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redactedPlaceholder + `"`), nil
}

func (s Secret) MarshalZerologObject(e *zerolog.Event) {
	e.Str("value", redactedPlaceholder)
}

func (s Secret) Reveal() string { return string(s) }
