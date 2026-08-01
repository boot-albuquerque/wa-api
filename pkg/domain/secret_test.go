package domain_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"wa-api/pkg/domain"
)

func TestNoSecretInOutput(t *testing.T) {
	const raw = "sk-live-a1b2c3d4e5f6-known-secret"
	sec := domain.Secret(raw)

	holder := struct {
		Token domain.Secret `json:"token"`
		Name  string        `json:"name"`
	}{Token: sec, Name: "alice"}

	structJSON, err := json.Marshal(holder)
	if err != nil {
		t.Fatalf("json.Marshal(holder): %v", err)
	}
	secretJSON, err := json.Marshal(sec)
	if err != nil {
		t.Fatalf("json.Marshal(sec): %v", err)
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Info().
		Object("secret", sec).
		EmbedObject(sec).
		Interface("iface", sec).
		Str("stringer", sec.String()).
		Msgf("token=%v", sec)

	outputs := map[string]string{
		"struct %+v":    fmt.Sprintf("%+v", holder),
		"pointer %v":    fmt.Sprintf("%v", &sec),
		"Sprint":        fmt.Sprint(sec),
		"json struct":   string(structJSON),
		"json secret":   string(secretJSON),
		"zerolog event": buf.String(),
	}
	for _, verb := range []string{"%v", "%+v", "%s", "%q", "%#v"} {
		outputs[verb] = fmt.Sprintf(verb, sec)
	}

	for name, got := range outputs {
		if strings.Contains(got, raw) {
			t.Errorf("%s leaked the secret: %s", name, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s is not redacted: %s", name, got)
		}
	}

	if sec.Reveal() != raw {
		t.Errorf("Reveal() = %q, want %q", sec.Reveal(), raw)
	}
}
