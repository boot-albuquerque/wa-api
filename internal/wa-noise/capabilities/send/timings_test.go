// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func marshalTimings(t *testing.T, mdt DebugTimings) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Info().Object("timings", mdt).Send()
	var decoded struct {
		Timings map[string]any `json:"timings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("log nao e' JSON valido (%q): %v", buf.String(), err)
	}
	return decoded.Timings
}

// Os campos opcionais (lid_fetch, get_participants, group_encrypt, retry) so'
// aparecem quando nao sao zero — sao etapas que nem todo envio executa, e
// logar zeros esconderia a diferenca entre "nao rodou" e "rodou instantaneo".
func TestMessageDebugTimingsOmitsZeroOptionalFields(t *testing.T) {
	fields := marshalTimings(t, DebugTimings{})
	for _, key := range []string{"lid_fetch", "get_participants", "group_encrypt", "retry"} {
		if _, ok := fields[key]; ok {
			t.Errorf("%q nao deveria aparecer quando zerado", key)
		}
	}
}

// Os campos obrigatorios aparecem sempre, inclusive zerados.
func TestMessageDebugTimingsAlwaysLogsMandatoryFields(t *testing.T) {
	fields := marshalTimings(t, DebugTimings{})
	for _, key := range []string{"queue", "marshal", "get_devices", "peer_encrypt", "send", "resp"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("%q deveria aparecer mesmo zerado, campos = %v", key, fields)
		}
	}
}

func TestMessageDebugTimingsLogsEveryField(t *testing.T) {
	fields := marshalTimings(t, DebugTimings{
		LIDFetch:        1 * time.Millisecond,
		Queue:           2 * time.Millisecond,
		Marshal:         3 * time.Millisecond,
		GetParticipants: 4 * time.Millisecond,
		GetDevices:      5 * time.Millisecond,
		GroupEncrypt:    6 * time.Millisecond,
		PeerEncrypt:     7 * time.Millisecond,
		Send:            8 * time.Millisecond,
		Resp:            9 * time.Millisecond,
		Retry:           10 * time.Millisecond,
	})
	want := []string{
		"lid_fetch", "queue", "marshal", "get_participants", "get_devices",
		"group_encrypt", "peer_encrypt", "send", "resp", "retry",
	}
	if len(fields) != len(want) {
		t.Fatalf("campos = %v, queria %d chaves", fields, len(want))
	}
	for _, key := range want {
		if _, ok := fields[key]; !ok {
			t.Errorf("faltou %q, campos = %v", key, fields)
		}
	}
}
