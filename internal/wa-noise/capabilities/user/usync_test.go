// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// Relocado de user_usync_test.go (Fase E lote 7). A guarda de cliente nil ficou
// na fachada da raiz e continua com teste la'.

func TestUSyncRejectsMoreThanOneExtra(t *testing.T) {
	f := newFakeTransport()
	_, err := USync(t.Context(), f, nil, ModeQuery, ContextMessage, nil, QueryExtras{}, QueryExtras{})
	if err == nil || !strings.Contains(err.Error(), "only one extra parameter") {
		t.Errorf("got %v, want the 'only one extra parameter' error", err)
	}
	if len(f.sent) != 0 {
		t.Error("nao deveria ter mandado nenhum <iq>")
	}
}

// Servidor de JID que USync nao sabe montar aborta a consulta inteira em vez de
// mandar um <user> incompleto.
func TestUSyncRejectsUnknownServer(t *testing.T) {
	cases := map[string]types.JID{
		"grupo":      types.NewJID("55511", types.GroupServer),
		"newsletter": types.NewJID("123", types.NewsletterServer),
		"msgr":       userTestFBJID,
		"vazio":      types.EmptyJID,
	}
	for name, jid := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeTransport()
			_, err := USync(t.Context(), f, []types.JID{jid}, ModeQuery, ContextMessage, nil)
			if err == nil || !strings.Contains(err.Error(), "unknown user server") {
				t.Errorf("got %v, want the 'unknown user server' error", err)
			}
			if len(f.sent) != 0 {
				t.Error("nao deveria ter mandado nenhum <iq>")
			}
		})
	}
}

// O envelope do <iq>: namespace, tipo, destino e os atributos fixos do <usync>.
func TestUSyncBuildsTheQueryNode(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}
	query := []waBinary.Node{{Tag: statusNodeTag}}

	if _, err := USync(t.Context(), f, []types.JID{userTestPNJID}, ModeFull, ContextBackground, query); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("mandou %d <iq>, queria 1", len(f.sent))
	}
	iq := f.sent[0]
	if iq.Namespace != usyncIQNamespace || iq.Type != IQGet || iq.To != types.ServerJID {
		t.Errorf("envelope errado: %+v", iq)
	}
	node := iq.Content.([]waBinary.Node)[0]
	if node.Tag != usyncNodeTag {
		t.Fatalf("tag do no = %q", node.Tag)
	}
	wantAttrs := waBinary.Attrs{
		"sid": f.reqID, "mode": ModeFull, "last": usyncLastValue,
		"index": usyncIndexValue, "context": ContextBackground,
	}
	for k, want := range wantAttrs {
		if got := node.Attrs[k]; got != want {
			t.Errorf("attr %q = %v, queria %v", k, got, want)
		}
	}
	children := node.Content.([]waBinary.Node)
	if children[0].Tag != usyncQueryTag || children[1].Tag != usyncListTag {
		t.Errorf("filhos = %v", children)
	}
	list := children[1].Content.([]waBinary.Node)
	if len(list) != 1 || list[0].Tag != usyncUserTag || list[0].Attrs["jid"] != userTestPNJID {
		t.Errorf("lista de usuarios = %v", list)
	}
}

// JID legado vai como <contact> textual; PN e LID vao como atributo `jid`. O
// device e' sempre removido (ToNonAD).
func TestUSyncEncodesEachServerShape(t *testing.T) {
	legacy := types.NewJID("5511999", types.LegacyUserServer)
	withDevice := types.JID{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 7}
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}

	if _, err := USync(
		t.Context(), f, []types.JID{legacy, withDevice, userTestLIDJID}, ModeQuery, ContextMessage, nil,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	list := node.Content.([]waBinary.Node)[1].Content.([]waBinary.Node)

	if list[0].Attrs != nil {
		t.Errorf("JID legado nao deveria ter attrs: %v", list[0].Attrs)
	}
	contact := list[0].Content.([]waBinary.Node)[0]
	if contact.Tag != contactNodeTag || contact.Content != legacy.String() {
		t.Errorf("<contact> = %+v", contact)
	}
	if got := list[1].Attrs["jid"]; got != userTestPNJID {
		t.Errorf("PN com device deveria virar %s, veio %v", userTestPNJID, got)
	}
	if got := list[2].Attrs["jid"]; got != userTestLIDJID {
		t.Errorf("LID = %v", got)
	}
}

// Bot JID ganha o sub-no <bot><profile persona_id=...>, com o persona_id
// casado pelo `User` do JID.
func TestUSyncAttachesBotPersonaID(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	if !botJID.IsBot() {
		t.Skipf("%s nao e' reconhecido como JID de bot", botJID)
	}
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}

	_, err := USync(t.Context(), f, []types.JID{botJID}, ModeQuery, ContextInteractive, nil, QueryExtras{
		BotListInfo: []types.BotListInfo{
			{BotJID: types.NewJID("999", types.DefaultUserServer), PersonaID: "outro"},
			{BotJID: botJID, PersonaID: "persona-certa"},
		},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	list := node.Content.([]waBinary.Node)[1].Content.([]waBinary.Node)
	bot := list[0].Content.([]waBinary.Node)[0]
	if bot.Tag != botIQNamespace {
		t.Fatalf("tag = %q", bot.Tag)
	}
	profile := bot.Content.([]waBinary.Node)[0]
	if profile.Tag != profileNodeTag || profile.Attrs["persona_id"] != "persona-certa" {
		t.Errorf("<profile> = %+v", profile)
	}
}

// Bot sem entrada correspondente em BotListInfo ainda manda o sub-no, com
// persona_id vazio — nao aborta a consulta.
func TestUSyncBotWithoutMatchingPersonaSendsEmptyID(t *testing.T) {
	botJID := types.NewJID("13135550002", types.DefaultUserServer)
	if !botJID.IsBot() {
		t.Skipf("%s nao e' reconhecido como JID de bot", botJID)
	}
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse()}
	if _, err := USync(t.Context(), f, []types.JID{botJID}, ModeQuery, ContextInteractive, nil); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	list := node.Content.([]waBinary.Node)[1].Content.([]waBinary.Node)
	profile := list[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if profile.Attrs["persona_id"] != "" {
		t.Errorf("persona_id = %v, queria vazio", profile.Attrs["persona_id"])
	}
}

func TestUSyncWrapsTransportError(t *testing.T) {
	boom := errors.New("socket morreu")
	f := newFakeTransport()
	f.err = []error{boom}
	_, err := USync(t.Context(), f, nil, ModeQuery, ContextMessage, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("erro nao embrulha o original: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to send usync query") {
		t.Errorf("mensagem = %q", err.Error())
	}
}

// Resposta sem <usync><list> vira ElementMissing, nao lista vazia — um parse
// silencioso aqui esconderia mudanca de esquema do servidor.
func TestUSyncMissingListIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := USync(t.Context(), f, nil, ModeQuery, ContextMessage, nil)
	var missing *testElementMissing
	if !errors.As(err, &missing) {
		t.Fatalf("erro = %v, queria ElementMissing", err)
	}
	if missing.Tag != usyncListTag {
		t.Errorf("tag = %q", missing.Tag)
	}
}

func TestUSyncReturnsTheListNode(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(usyncUser(userTestPNJID))}
	list, err := USync(t.Context(), f, nil, ModeQuery, ContextMessage, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if list.Tag != usyncListTag || len(list.GetChildren()) != 1 {
		t.Errorf("lista = %+v", list)
	}
}
