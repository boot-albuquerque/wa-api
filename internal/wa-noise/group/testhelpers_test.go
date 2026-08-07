// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

var (
	groupTestJID    = types.NewJID("55511", types.GroupServer)
	groupTestPNJID  = types.NewJID("5511999", types.DefaultUserServer)
	groupTestLIDJID = types.NewJID("8877", types.HiddenUserServer)
	groupTestPN2JID = types.NewJID("5511888", types.DefaultUserServer)
	groupTestLID2   = types.NewJID("7766", types.HiddenUserServer)
)

// testIQErrors espelha os sentinelas de IQ da raiz. Sao errors.New proprios de
// proposito: os testes deste pacote so' precisam que errors.Is case por
// identidade, e depender dos valores da raiz reintroduziria o import que a
// extracao removeu. O contrato de que a raiz entrega os ponteiros certos e'
// travado do outro lado, pelo `var _ group.Transport = groupTransport{}` em
// group_transport.go.
var testIQErrors = IQErrors{
	NotAuthorized: errors.New("iq 401"),
	Forbidden:     errors.New("iq 403"),
	NotFound:      errors.New("iq 404"),
	NotAcceptable: errors.New("iq 406"),
	Gone:          errors.New("iq 410"),
}

// testElementMissing espelha *whatsmeow.ElementMissingError.
type testElementMissing struct {
	Tag string
	In  string
}

func (e *testElementMissing) Error() string {
	return fmt.Sprintf("missing <%s> element in %s", e.Tag, e.In)
}

// testWrappedIQError espelha o *wrappedIQError da raiz: Is() casa contra o erro
// humano, Unwrap() devolve o erro de IQ.
type testWrappedIQError struct {
	human error
	iq    error
}

func (e *testWrappedIQError) Error() string { return e.human.Error() }
func (e *testWrappedIQError) Is(other error) bool {
	return errors.Is(other, e.human)
}
func (e *testWrappedIQError) Unwrap() error { return e.iq }

// fakeTransport e' o duble de group.Transport. Registra os <iq> enviados e
// devolve respostas/erros programados, sem socket nem sessao Noise.
type fakeTransport struct {
	cache Cache
	store *store.Device

	sent []IQ
	// resp e' consultado por indice de chamada; um nil deixa o zero.
	resp []*waBinary.Node
	err  []error

	msgID types.MessageID
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{msgID: "3EB0GERADO"}
}

func (f *fakeTransport) SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error) {
	i := len(f.sent)
	f.sent = append(f.sent, query)
	var resp *waBinary.Node
	var err error
	if i < len(f.resp) {
		resp = f.resp[i]
	}
	if i < len(f.err) {
		err = f.err[i]
	}
	if resp == nil && err == nil {
		resp = &waBinary.Node{}
	}
	return resp, err
}

func (f *fakeTransport) Store() *store.Device               { return f.store }
func (f *fakeTransport) Cache() *Cache                      { return &f.cache }
func (f *fakeTransport) Log() waLog.Logger                  { return waLog.Noop }
func (f *fakeTransport) GenerateMessageID() types.MessageID { return f.msgID }
func (f *fakeTransport) TrimMessageIDPrefix(id types.MessageID) string {
	const webPrefix = "3EB0"
	if len(id) >= len(webPrefix) && string(id)[:len(webPrefix)] == webPrefix {
		return string(id)[len(webPrefix):]
	}
	return string(id)
}
func (f *fakeTransport) ElementMissing(tag, in string) error {
	return &testElementMissing{Tag: tag, In: in}
}
func (f *fakeTransport) WrapIQError(human, iq error) error {
	return &testWrappedIQError{human: human, iq: iq}
}
func (f *fakeTransport) IQErrors() IQErrors { return testIQErrors }

var _ Transport = (*fakeTransport)(nil)

// cached devolve a entrada de jid no cache do duble, tomando o lock.
func (f *fakeTransport) cached(jid types.JID) (*Meta, bool) {
	f.cache.Lock()
	defer f.cache.Unlock()
	return f.cache.GetLocked(jid)
}

// putCached grava uma entrada no cache do duble, tomando o lock.
func (f *fakeTransport) putCached(jid types.JID, meta *Meta) {
	f.cache.Lock()
	defer f.cache.Unlock()
	f.cache.SetLocked(jid, meta)
}

func participantNode(jid types.JID, extra waBinary.Attrs) waBinary.Node {
	attrs := waBinary.Attrs{"jid": jid}
	for k, v := range extra {
		attrs[k] = v
	}
	return waBinary.Node{Tag: participantTag, Attrs: attrs}
}

// --- dubles de store ---

// fakeStores implementa de uma vez as tres fatias de store.Device que este
// dominio toca (LIDs, Contacts, PrivacyTokens), registrando o que foi gravado.
// Embute store.NoopStore para satisfazer o resto de cada interface: qualquer
// metodo nao previsto devolve erro em vez de compilar por acidente.
type fakeStores struct {
	*store.NoopStore

	lidPairs  []store.LIDMapping
	redacted  []store.RedactedPhoneEntry
	lidPutErr error
	redErr    error

	pnForLID    types.JID
	pnForLIDErr error

	token    *store.PrivacyToken
	tokenErr error
}

func newFakeStores() *fakeStores {
	return &fakeStores{NoopStore: &store.NoopStore{Error: errors.New("nao previsto neste teste")}}
}

func (f *fakeStores) PutManyLIDMappings(_ context.Context, m []store.LIDMapping) error {
	f.lidPairs = append(f.lidPairs, m...)
	return f.lidPutErr
}

func (f *fakeStores) PutManyRedactedPhones(_ context.Context, e []store.RedactedPhoneEntry) error {
	f.redacted = append(f.redacted, e...)
	return f.redErr
}

func (f *fakeStores) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return f.pnForLID, f.pnForLIDErr
}

func (f *fakeStores) GetPrivacyToken(context.Context, types.JID) (*store.PrivacyToken, error) {
	return f.token, f.tokenErr
}

// withStores liga um fakeStores ao duble de transporte e o devolve.
func (f *fakeTransport) withStores() *fakeStores {
	st := newFakeStores()
	f.store = &store.Device{LIDs: st, Contacts: st, PrivacyTokens: st}
	return st
}

// groupNodeWith monta um <group> minimo valido (id + creation) com os filhos
// dados.
func groupNodeWith(children ...waBinary.Node) waBinary.Node {
	return waBinary.Node{
		Tag:     nodeTag,
		Attrs:   waBinary.Attrs{"id": groupTestJID.User, "creation": "1699999999"},
		Content: children,
	}
}
