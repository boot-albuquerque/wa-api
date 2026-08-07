// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"strings"
	"testing"

	"wa-api/internal/wa-noise/types"
)

// Os tres caminhos de `usync` que retornam ANTES de tocar a rede. O resto da
// funcao (montagem do no e leitura da resposta) exige socket — ver a secao de
// lacunas do lote 7 em PATCHES.md.

func TestUsyncNilClient(t *testing.T) {
	var cli *Client
	if _, err := cli.usync(t.Context(), nil, usyncModeQuery, usyncContextMessage, nil); err != ErrClientIsNil {
		t.Errorf("got %v, want ErrClientIsNil", err)
	}
}

func TestUsyncRejectsMoreThanOneExtra(t *testing.T) {
	_, err := userTestClient().usync(
		t.Context(), nil, usyncModeQuery, usyncContextMessage, nil,
		UsyncQueryExtras{}, UsyncQueryExtras{},
	)
	if err == nil || !strings.Contains(err.Error(), "only one extra parameter") {
		t.Errorf("got %v, want the 'only one extra parameter' error", err)
	}
}

// Servidor de JID que `usync` nao sabe montar aborta a consulta inteira em vez
// de mandar um `<user>` incompleto.
func TestUsyncRejectsUnknownServer(t *testing.T) {
	cases := map[string]types.JID{
		"grupo":      types.NewJID("55511", types.GroupServer),
		"newsletter": types.NewJID("123", types.NewsletterServer),
		"msgr":       userTestFBJID,
		"vazio":      types.EmptyJID,
	}
	for name, jid := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := userTestClient().usync(
				t.Context(), []types.JID{jid}, usyncModeQuery, usyncContextMessage, nil,
			)
			if err == nil || !strings.Contains(err.Error(), "unknown user server") {
				t.Errorf("got %v, want the 'unknown user server' error", err)
			}
		})
	}
}
