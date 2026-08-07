// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// Relocado de user_bots_test.go (Fase E lote 7), adaptado aos dubles.

// --- NodeContentString ---

// O contrato que segura o fix do lote 7: GetChildByTag devolve o proprio no
// quando nao acha o filho, e nesse caso Content nunca e' []byte. Antes,
// .Content.([]byte) direto em GetBotProfiles era panic remoto.
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
			if got := NodeContentString(tc.node); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Reproducao direta do panic corrigido: um <profile> sem nenhum dos filhos
// textuais que GetBotProfiles le. Antes do fix, cada uma destas leituras era
// nil.([]byte) e derrubava o processo.
func TestNodeContentStringOnMissingChildDoesNotPanic(t *testing.T) {
	profile := waBinary.Node{Tag: profileNodeTag}
	for _, tag := range []string{"name", "attributes", "description", businessCategoryTag} {
		if got := NodeContentString(profile.GetChildByTag(tag)); got != "" {
			t.Errorf("tag %q: got %q, want empty", tag, got)
		}
	}
	// O caso mais traicoeiro: GetChildByTag devolve o proprio <profile>, cujo
	// Content e' a lista de filhos.
	withChildren := waBinary.Node{
		Tag:     profileNodeTag,
		Content: []waBinary.Node{{Tag: "other", Content: []byte("x")}},
	}
	if got := NodeContentString(withChildren.GetChildByTag("name")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// O fix inteiro, ponta a ponta: uma resposta de perfil de bot completamente
// vazia atravessa GetBotProfiles sem panic e produz campos vazios.
func TestGetBotProfilesOnEmptyProfileDoesNotPanic(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, waBinary.Node{
			Tag:     botIQNamespace,
			Content: []waBinary.Node{{Tag: profileNodeTag}},
		}),
	)}

	got, err := GetBotProfiles(t.Context(), f, []types.BotListInfo{{BotJID: userTestPNJID}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	p := got[0]
	if p.Name != "" || p.Attributes != "" || p.Description != "" || p.Category != "" ||
		p.CommandsDescription != "" || len(p.Commands) != 0 || len(p.Prompts) != 0 {
		t.Errorf("got %+v, want every text field empty", p)
	}
}

// --- GetBotListV2 ---

func botListResponse(sections ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     botIQNamespace,
			Content: sections,
		}},
	}
}

func TestGetBotListV2ReadsTheAllSection(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	f := newFakeTransport()
	f.resp = []*waBinary.Node{botListResponse(
		waBinary.Node{
			Tag:   botSectionTag,
			Attrs: waBinary.Attrs{"type": botSectionTypeAll},
			Content: []waBinary.Node{{
				Tag:   botIQNamespace,
				Attrs: waBinary.Attrs{"persona_id": "p1", "jid": botJID},
			}},
		},
		// Secao de outro tipo e' ignorada por completo.
		waBinary.Node{
			Tag:   botSectionTag,
			Attrs: waBinary.Attrs{"type": "destaques"},
			Content: []waBinary.Node{{
				Tag:   botIQNamespace,
				Attrs: waBinary.Attrs{"persona_id": "p2", "jid": botJID},
			}},
		},
	)}

	got, err := GetBotListV2(t.Context(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].PersonaID != "p1" || got[0].BotJID != botJID {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != botIQNamespace || iq.Type != IQGet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	if iq.Content.([]waBinary.Node)[0].Attrs["v"] != botListVersion {
		t.Errorf("version = %v", iq.Content.([]waBinary.Node)[0].Attrs)
	}
}

// Entrada com atributo invalido (jid ilegivel) e' descartada sem contaminar as
// seguintes: o AttrGetter e' recriado por bot.
func TestGetBotListV2SkipsInvalidEntries(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	f := newFakeTransport()
	f.resp = []*waBinary.Node{botListResponse(waBinary.Node{
		Tag:   botSectionTag,
		Attrs: waBinary.Attrs{"type": botSectionTypeAll},
		Content: []waBinary.Node{
			{Tag: botIQNamespace, Attrs: waBinary.Attrs{"persona_id": "ruim"}}, // sem jid
			{Tag: botIQNamespace, Attrs: waBinary.Attrs{"persona_id": "bom", "jid": botJID}},
		},
	})}

	got, err := GetBotListV2(t.Context(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].PersonaID != "bom" {
		t.Errorf("got %+v", got)
	}
}

func TestGetBotListV2PropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetBotListV2(t.Context(), f); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestGetBotListV2MissingNodeIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetBotListV2(t.Context(), f)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != botIQNamespace {
		t.Errorf("got %v", err)
	}
}

// --- GetBotProfiles ---

func TestGetBotProfilesReadsEveryField(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(botJID, waBinary.Node{
			Tag: botIQNamespace,
			Content: []waBinary.Node{{
				Tag:   profileNodeTag,
				Attrs: waBinary.Attrs{"persona_id": "p1"},
				Content: []waBinary.Node{
					{Tag: "name", Content: []byte("Meta AI")},
					{Tag: "attributes", Content: []byte("attrs")},
					{Tag: "description", Content: []byte("desc")},
					{Tag: businessCategoryTag, Content: []byte("cat")},
					{Tag: "default"},
					{Tag: botCommandsTag, Content: []waBinary.Node{
						{Tag: "description", Content: []byte("cmds")},
						{Tag: botCommandTag, Content: []waBinary.Node{
							{Tag: "name", Content: []byte("/oi")},
							{Tag: "description", Content: []byte("cumprimenta")},
						}},
					}},
					{Tag: botPromptsTag, Content: []waBinary.Node{
						{Tag: botPromptTag, Content: []waBinary.Node{
							{Tag: "emoji", Content: []byte("🎯")},
							{Tag: "text", Content: []byte("me ajude")},
						}},
					}},
				},
			}},
		}),
	)}

	got, err := GetBotProfiles(t.Context(), f, []types.BotListInfo{{BotJID: botJID, PersonaID: "p1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	p := got[0]
	if p.JID != botJID || p.Name != "Meta AI" || p.Attributes != "attrs" ||
		p.Description != "desc" || p.Category != "cat" || !p.IsDefault || p.PersonaID != "p1" {
		t.Errorf("got %+v", p)
	}
	if p.CommandsDescription != "cmds" {
		t.Errorf("commands description = %q", p.CommandsDescription)
	}
	if len(p.Commands) != 1 || p.Commands[0].Name != "/oi" || p.Commands[0].Description != "cumprimenta" {
		t.Errorf("commands = %+v", p.Commands)
	}
	if len(p.Prompts) != 1 || p.Prompts[0] != "🎯 me ajude" {
		t.Errorf("prompts = %v", p.Prompts)
	}
}

// A consulta manda o <bot><profile v=...> e propaga o BotListInfo como extras
// (e' assim que o persona_id chega em cada <user>).
func TestGetBotProfilesBuildsTheQuery(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}

	if _, err := GetBotProfiles(t.Context(), f, []types.BotListInfo{
		{BotJID: botJID, PersonaID: "p1"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	if node.Attrs["mode"] != ModeQuery || node.Attrs["context"] != ContextInteractive {
		t.Errorf("attrs = %v", node.Attrs)
	}
	query := node.Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	if len(query) != 1 || query[0].Tag != botIQNamespace {
		t.Fatalf("query = %v", query)
	}
	profile := query[0].Content.([]waBinary.Node)[0]
	if profile.Tag != profileNodeTag || profile.Attrs["v"] != botProfileVersion {
		t.Errorf("<profile> = %+v", profile)
	}
	// O persona_id do extras chegou na lista de usuarios.
	list := node.Content.([]waBinary.Node)[1].Content.([]waBinary.Node)
	inner := list[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if inner.Attrs["persona_id"] != "p1" {
		t.Errorf("persona_id = %v", inner.Attrs)
	}
}

func TestGetBotProfilesPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetBotProfiles(t.Context(), f, nil); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestGetBotProfilesEmptyResponse(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}
	got, err := GetBotProfiles(t.Context(), f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}
