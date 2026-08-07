// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
)

// --- nodeContentString ---

// O contrato que segura o fix do lote 7: `GetChildByTag` devolve o proprio no
// quando nao acha o filho, e nesse caso `Content` nunca e' `[]byte`. Antes,
// `.Content.([]byte)` direto em `GetBotProfiles` era panic remoto.
func TestNodeContentStringHandlesEveryContentShape(t *testing.T) {
	cases := map[string]struct {
		node waBinary.Node
		want string
	}{
		"bytes":                  {waBinary.Node{Content: []byte("hello")}, "hello"},
		"bytes vazio":            {waBinary.Node{Content: []byte{}}, ""},
		"nil":                    {waBinary.Node{}, ""},
		"string em vez de bytes": {waBinary.Node{Content: "hello"}, ""},
		"filhos em vez de bytes": {
			waBinary.Node{Content: []waBinary.Node{{Tag: "child"}}},
			"",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := nodeContentString(tc.node); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Reproducao direta do panic corrigido: um `<profile>` sem nenhum dos filhos
// textuais que `GetBotProfiles` le. Antes do fix, cada uma destas leituras era
// `nil.([]byte)` e derrubava o processo.
func TestNodeContentStringOnMissingChildDoesNotPanic(t *testing.T) {
	profile := waBinary.Node{Tag: profileNodeTag}
	for _, tag := range []string{"name", "attributes", "description", businessCategoryTag} {
		if got := nodeContentString(profile.GetChildByTag(tag)); got != "" {
			t.Errorf("tag %q: got %q, want empty", tag, got)
		}
	}
	// O caso mais traicoeiro: GetChildByTag devolve o proprio <profile>, cujo
	// Content e' a lista de filhos.
	withChildren := waBinary.Node{
		Tag:     profileNodeTag,
		Content: []waBinary.Node{{Tag: "other", Content: []byte("x")}},
	}
	if got := nodeContentString(withChildren.GetChildByTag("name")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
