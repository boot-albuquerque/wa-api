// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"

	"google.golang.org/protobuf/proto"
)

// --- SetStatusMessage ---

func TestSetStatusMessageBuildsTheIQ(t *testing.T) {
	f := newFakeTransport()
	if err := SetStatusMessage(t.Context(), f, "ola"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	iq := f.sent[0]
	if iq.Namespace != statusIQNamespace || iq.Type != IQSet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	node := iq.Content.([]waBinary.Node)[0]
	if node.Tag != statusNodeTag || node.Content != "ola" {
		t.Errorf("no = %+v", node)
	}
}

func TestSetStatusMessagePropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if err := SetStatusMessage(t.Context(), f, "x"); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

// --- IsOnWhatsApp ---

func TestIsOnWhatsAppReadsEachField(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID,
			waBinary.Node{
				Tag: businessNodeTag,
				Content: []waBinary.Node{{
					Tag:     verifiedNameNodeTag,
					Content: verifiedNameCertBytes(t, "Loja"),
				}},
			},
			waBinary.Node{
				Tag:     contactNodeTag,
				Attrs:   waBinary.Attrs{"type": contactTypeIn},
				Content: []byte("+5511999@" + types.LegacyUserServer),
			},
		),
	)}

	got, err := IsOnWhatsApp(t.Context(), f, []string{"+5511999"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if got[0].JID != userTestPNJID || !got[0].IsIn {
		t.Errorf("got %+v", got[0])
	}
	if got[0].Query != "+5511999" {
		t.Errorf("query = %q (o sufixo @c.us deveria ter sido cortado)", got[0].Query)
	}
	if got[0].VerifiedName == nil || got[0].VerifiedName.Details.GetVerifiedName() != "Loja" {
		t.Errorf("verified name = %+v", got[0].VerifiedName)
	}
	// A consulta e' feita com o numero convertido para JID legado.
	node := f.sent[0].Content.([]waBinary.Node)[0]
	list := node.Content.([]waBinary.Node)[1].Content.([]waBinary.Node)
	if list[0].Content.([]waBinary.Node)[0].Content != "+5511999@"+types.LegacyUserServer {
		t.Errorf("contato consultado = %v", list[0].Content)
	}
}

// Filho que nao e' <user>, ou <user> sem jid, e' ignorado sem derrubar a lista.
func TestIsOnWhatsAppSkipsInvalidChildren(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		waBinary.Node{Tag: "outra-tag", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
		waBinary.Node{Tag: usyncUserTag},
		usyncUser(userTestPNJID),
	)}
	got, err := IsOnWhatsApp(t.Context(), f, []string{"+5511999"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || got[0].JID != userTestPNJID {
		t.Errorf("got %v", got)
	}
}

// Nome verificado invalido loga Warn e segue: o usuario ainda entra na lista.
func TestIsOnWhatsAppTolerantesToBadVerifiedName(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, waBinary.Node{
			Tag:     businessNodeTag,
			Content: []waBinary.Node{{Tag: verifiedNameNodeTag, Content: []byte{0xff, 0xff, 0xff, 0xff}}},
		}),
	)}
	got, err := IsOnWhatsApp(t.Context(), f, []string{"+5511999"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestIsOnWhatsAppPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := IsOnWhatsApp(t.Context(), f, nil); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

// --- IsValidLIDMapping (relocado de user_test.go, Fase E lote 7) ---

// O guarda que o lote 7 acrescentou em GetInfo: so' vai para
// PutManyLIDMappings o par que o store aceita (PN em s.whatsapp.net, LID em
// lid). Espelha CachedLIDMap.PutManyLIDMappings (store/sqlstore/lidmap.go:220),
// que descarta e loga qualquer outro formato.
func TestIsValidLIDMapping(t *testing.T) {
	pn := types.NewJID("5511999", types.DefaultUserServer)
	lid := types.NewJID("8877", types.HiddenUserServer)

	cases := []struct {
		name string
		pn   types.JID
		lid  types.JID
		want bool
	}{
		{"par valido", pn, lid, true},
		{"LID no lugar do PN", lid, lid, false},
		{"PN no lugar do LID", pn, pn, false},
		{"LID vazio", pn, types.EmptyJID, false},
		{"PN vazio", types.EmptyJID, lid, false},
		{"PN legado c.us", types.NewJID("5511999", types.LegacyUserServer), lid, false},
		{"PN msgr", types.NewJID("123", types.MessengerServer), lid, false},
		{"user do PN vazio", types.NewJID("", types.DefaultUserServer), lid, false},
		{"user do LID vazio", pn, types.NewJID("", types.HiddenUserServer), false},
		{"PN com device (AD JID)", types.JID{User: pn.User, Server: pn.Server, Device: 3}, lid, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidLIDMapping(tc.pn, tc.lid); got != tc.want {
				t.Errorf("IsValidLIDMapping(%s, %s) = %v, want %v", tc.pn, tc.lid, got, tc.want)
			}
		})
	}
}

// --- GetInfo ---

func TestGetInfoReadsEachField(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID,
			waBinary.Node{Tag: statusNodeTag, Content: []byte("sobre mim")},
			waBinary.Node{Tag: pictureNodeTag, Attrs: waBinary.Attrs{"id": "pic-1"}},
			devicesNode(deviceNode("0", false), deviceNode("1", false)),
			waBinary.Node{Tag: lidNodeTag, Attrs: waBinary.Attrs{"val": userTestLIDJID}},
		),
	)}

	got, err := GetInfo(t.Context(), f, []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	info, ok := got[userTestPNJID]
	if !ok {
		t.Fatalf("got %v", got)
	}
	if info.Status != "sobre mim" || info.PictureID != "pic-1" {
		t.Errorf("got %+v", info)
	}
	if len(info.Devices) != 2 {
		t.Errorf("devices = %v", info.Devices)
	}
	if info.LID != userTestLIDJID {
		t.Errorf("LID = %s", info.LID)
	}
	// O par PN/LID valido chega ao store.
	if len(st.lidPairs) != 1 || st.lidPairs[0].PN != userTestPNJID || st.lidPairs[0].LID != userTestLIDJID {
		t.Errorf("mapeamentos gravados = %v", st.lidPairs)
	}
}

// A regressao do lote 7 travada do lado da origem: quando o `jid` de <user> e'
// um LID, o par montado seria LID/LID e NAO pode chegar ao store.
func TestGetInfoDoesNotPersistLIDLIDMappings(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestLIDJID,
			waBinary.Node{Tag: lidNodeTag, Attrs: waBinary.Attrs{"val": userTestLIDJID}},
		),
		// <user> de PN sem <lid>: LID vazio, tambem invalido.
		usyncUser(userTestPNJID),
	)}

	got, err := GetInfo(t.Context(), f, []types.JID{userTestLIDJID, userTestPNJID})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %v", got)
	}
	if len(st.lidPairs) != 0 {
		t.Errorf("nenhum par deveria ter sido gravado, veio %v", st.lidPairs)
	}
}

// Erro ao gravar os mapeamentos e' logado, nao propagado: a informacao de
// usuario ja' foi obtida e vale mais que o cache de LID.
func TestGetInfoSurvivesLIDStoreError(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.lidPutErr = errors.New("disco cheio")
	f.resp = []*waBinary.Node{usyncResponse(usyncUser(userTestPNJID))}

	got, err := GetInfo(t.Context(), f, []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("o erro do store nao deveria propagar: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

// Nome verificado presente dispara UpdateBusinessName, que grava e emite
// events.BusinessName.
func TestGetInfoUpdatesBusinessName(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, waBinary.Node{
			Tag: businessNodeTag,
			Content: []waBinary.Node{{
				Tag:     verifiedNameNodeTag,
				Content: verifiedNameCertBytes(t, "Loja Teste"),
			}},
		}),
	)}

	if _, err := GetInfo(t.Context(), f, []types.JID{userTestPNJID}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if st.businessNames[userTestPNJID] != "Loja Teste" {
		t.Errorf("nomes gravados = %v", st.businessNames)
	}
	if len(f.events) != 1 {
		t.Fatalf("eventos = %v", f.events)
	}
	if evt, ok := f.events[0].(*events.BusinessName); !ok || evt.NewBusinessName != "Loja Teste" {
		t.Errorf("evento = %+v", f.events[0])
	}
}

func TestGetInfoSkipsInvalidChildren(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{usyncResponse(
		waBinary.Node{Tag: "outra-tag", Attrs: waBinary.Attrs{"jid": userTestPNJID}},
		waBinary.Node{Tag: usyncUserTag},
	)}
	got, err := GetInfo(t.Context(), f, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

// Nome verificado invalido loga Warn e segue.
func TestGetInfoTolerantesToBadVerifiedName(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{usyncResponse(
		usyncUser(userTestPNJID, waBinary.Node{
			Tag:     businessNodeTag,
			Content: []waBinary.Node{{Tag: verifiedNameNodeTag, Content: []byte{0xff, 0xff, 0xff, 0xff}}},
		}),
	)}
	got, err := GetInfo(t.Context(), f, []types.JID{userTestPNJID})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestGetInfoPropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetInfo(t.Context(), f, nil); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

// A consulta pede os cinco campos, com o `version` do <devices>.
func TestGetInfoAsksForEveryField(t *testing.T) {
	f := newFakeTransport()
	f.withStores()
	f.resp = []*waBinary.Node{usyncResponse()}
	if _, err := GetInfo(t.Context(), f, []types.JID{userTestPNJID}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := f.sent[0].Content.([]waBinary.Node)[0]
	if node.Attrs["mode"] != ModeFull || node.Attrs["context"] != ContextBackground {
		t.Errorf("attrs = %v", node.Attrs)
	}
	query := node.Content.([]waBinary.Node)[0].Content.([]waBinary.Node)
	var tags []string
	for _, q := range query {
		tags = append(tags, q.Tag)
	}
	want := []string{businessNodeTag, statusNodeTag, pictureNodeTag, devicesNodeTag, lidNodeTag}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, queria %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("tag %d = %q, queria %q", i, tags[i], want[i])
		}
	}
}

// --- HandleHistoricalPushNames ---

func pushname(id, name string) *waHistorySync.Pushname {
	return &waHistorySync.Pushname{ID: proto.String(id), Pushname: proto.String(name)}
}

func TestHandleHistoricalPushNamesStoresEachName(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
		pushname(userTestPNJID.String(), "Alice"),
		pushname(userTestPN2JID.String(), "Bob"),
	})
	if st.pushNames[userTestPNJID] != "Alice" || st.pushNames[userTestPN2JID] != "Bob" {
		t.Errorf("gravados = %v", st.pushNames)
	}
}

// Os tres caminhos que NAO gravam: contact store ausente, o sentinela "-" e o
// JID malformado.
func TestHandleHistoricalPushNamesSkipsWhatItCannotUse(t *testing.T) {
	t.Run("sem contact store", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		f.store.Contacts = nil
		HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
			pushname(userTestPNJID.String(), "Alice"),
		})
		if len(st.pushNames) != 0 {
			t.Errorf("gravados = %v", st.pushNames)
		}
	})
	t.Run("sentinela de remocao", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
			pushname(userTestPNJID.String(), "-"),
		})
		if len(st.pushNames) != 0 {
			t.Errorf("gravados = %v", st.pushNames)
		}
	})
	t.Run("JID malformado", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
			pushname("5511999:nao-e-numero@s.whatsapp.net", "Alice"),
			pushname(userTestPNJID.String(), "Bob"),
		})
		// O item ruim nao aborta o laco.
		if st.pushNames[userTestPNJID] != "Bob" || len(st.pushNames) != 1 {
			t.Errorf("gravados = %v", st.pushNames)
		}
	})
}

func TestHandleHistoricalPushNamesLogsStoreError(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.putErr = errors.New("disco cheio")
	// Nao entra em panic nem propaga: e' um caminho de log.
	HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
		pushname(userTestPNJID.String(), "Alice"),
	})
}

// Nome que nao mudou nao loga o Debug de "got push name" — o caminho existe e
// precisa ser exercitado.
func TestHandleHistoricalPushNamesUnchangedIsQuiet(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.noChange = true
	HandleHistoricalPushNames(t.Context(), f, []*waHistorySync.Pushname{
		pushname(userTestPNJID.String(), "Alice"),
	})
}

// --- UpdatePushName ---

func TestUpdatePushNameStoresBothJIDsAndDispatches(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	withDevice := types.JID{User: userTestPNJID.User, Server: types.DefaultUserServer, Device: 5}

	UpdatePushName(t.Context(), f, withDevice, userTestLIDJID, nil, "Alice")

	// O device e' removido antes de gravar.
	if st.pushNames[userTestPNJID] != "Alice" {
		t.Errorf("PN gravado = %v", st.pushNames)
	}
	if st.pushNames[userTestLIDJID] != "Alice" {
		t.Errorf("LID gravado = %v", st.pushNames)
	}
	if len(f.events) != 1 {
		t.Fatalf("eventos = %v", f.events)
	}
	evt, ok := f.events[0].(*events.PushName)
	if !ok || evt.JID != userTestPNJID || evt.JIDAlt != userTestLIDJID || evt.NewPushName != "Alice" {
		t.Errorf("evento = %+v", f.events[0])
	}
}

// Sem JID alternativo dado, ele e' buscado no store.
func TestUpdatePushNameResolvesAltJIDFromStore(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.altJID = userTestLIDJID

	UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")

	if st.pushNames[userTestLIDJID] != "Alice" {
		t.Errorf("gravados = %v", st.pushNames)
	}
}

// Store sem JID alternativo: grava so' o principal e ainda emite o evento.
func TestUpdatePushNameWithoutAltJID(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()

	UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")

	if len(st.pushNames) != 1 {
		t.Errorf("gravados = %v", st.pushNames)
	}
	if len(f.events) != 1 {
		t.Errorf("eventos = %v", f.events)
	}
}

func TestUpdatePushNameNoOpPaths(t *testing.T) {
	t.Run("sem contact store", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.store.Contacts = nil
		UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")
		if len(f.events) != 0 {
			t.Errorf("eventos = %v", f.events)
		}
	})
	t.Run("nome nao mudou", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		st.noChange = true
		UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")
		if len(f.events) != 0 {
			t.Errorf("nome inalterado nao deveria emitir evento: %v", f.events)
		}
	})
	t.Run("erro ao gravar", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		st.putErr = errors.New("disco cheio")
		UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")
		if len(f.events) != 0 {
			t.Errorf("erro de gravacao nao deveria emitir evento: %v", f.events)
		}
	})
}

// Erro ao gravar o alternativo e' logado, e o evento sai assim mesmo: o nome
// principal ja' mudou e os handlers precisam saber.
func TestUpdatePushNameAltStoreErrorStillDispatches(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.altJID = userTestLIDJID
	f.store.Contacts = &failOnSecondPut{fakeStores: st}

	UpdatePushName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Alice")

	if len(f.events) != 1 {
		t.Errorf("eventos = %v", f.events)
	}
}

// failOnSecondPut deixa a primeira gravacao passar e falha na segunda — e' o
// que separa "falhou o principal" de "falhou so' o alternativo".
type failOnSecondPut struct {
	*fakeStores
	calls int
}

func (f *failOnSecondPut) PutPushName(
	ctx context.Context, jid types.JID, name string,
) (bool, string, error) {
	f.calls++
	if f.calls > 1 {
		return false, "", errors.New("falhou o alternativo")
	}
	return f.fakeStores.PutPushName(ctx, jid, name)
}

func (f *failOnSecondPut) PutBusinessName(
	ctx context.Context, jid types.JID, name string,
) (bool, string, error) {
	f.calls++
	if f.calls > 1 {
		return false, "", errors.New("falhou o alternativo")
	}
	return f.fakeStores.PutBusinessName(ctx, jid, name)
}
