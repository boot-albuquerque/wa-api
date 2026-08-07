// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// fakeTransport e' o duble de Transport usado por todos os testes deste
// pacote. Foi exatamente o ganho de testabilidade que motivou a extracao: antes
// da Fase F/G era preciso um *Client com socket e sessao Noise para chegar
// nestes caminhos.
type fakeTransport struct {
	store *store.Device
	proc  *appstate.Processor
	state State

	emitOnFullSync bool
	debugLogs      bool

	// iq responde a cada SendIQ. Se iqErr for nao-nil, e' devolvido no lugar.
	iq     *waBinary.Node
	iqErr  error
	iqSeq  []*waBinary.Node
	iqErrs []error

	sentIQs []IQ

	dispatched   []any
	handlerFails bool

	peerMsgs   []*waE2E.Message
	peerMsgErr error

	blob    []byte
	blobErr error

	nctSalt      []byte
	nctCleared   bool
	nctStoreErr  error
	nctClearErr  error
	mu           sync.Mutex
	elementCalls []string
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{store: &store.Device{}}
}

func (f *fakeTransport) Store() *store.Device       { return f.store }
func (f *fakeTransport) Proc() *appstate.Processor  { return f.proc }
func (f *fakeTransport) State() *State              { return &f.state }
func (f *fakeTransport) Log() waLog.Logger          { return waLog.Noop }
func (f *fakeTransport) EmitEventsOnFullSync() bool { return f.emitOnFullSync }
func (f *fakeTransport) DebugLogs() bool            { return f.debugLogs }

func (f *fakeTransport) DispatchEvent(evt any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatched = append(f.dispatched, evt)
	return f.handlerFails
}

func (f *fakeTransport) SendIQ(_ context.Context, query IQ) (*waBinary.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentIQs = append(f.sentIQs, query)
	if len(f.iqSeq) > 0 {
		node, err := f.iqSeq[0], f.iqErrs[0]
		f.iqSeq, f.iqErrs = f.iqSeq[1:], f.iqErrs[1:]
		return node, err
	}
	return f.iq, f.iqErr
}

func (f *fakeTransport) SendPeerMessage(_ context.Context, msg *waE2E.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peerMsgs = append(f.peerMsgs, msg)
	return f.peerMsgErr
}

func (f *fakeTransport) DownloadExternalBlob(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error) {
	return f.blob, f.blobErr
}

func (f *fakeTransport) StoreNCTSalt(_ context.Context, salt []byte) error {
	f.nctSalt = salt
	return f.nctStoreErr
}

func (f *fakeTransport) ClearNCTSalt(context.Context) error {
	f.nctCleared = true
	return f.nctClearErr
}

// ElementMissing devolve um erro simples: o tipo concreto de verdade
// (*whatsmeow.ElementMissingError) vive na raiz, que este pacote nao importa.
func (f *fakeTransport) ElementMissing(tag, in string) error {
	f.elementCalls = append(f.elementCalls, tag+"/"+in)
	return fmt.Errorf("elemento <%s> ausente em %s", tag, in)
}

// --- dubles de sub-store ---

// fakeAppStateStore implementa store.AppStateStore guardando versao/hash em
// memoria. Os metodos de MAC nao sao exercitados por este pacote (sao do
// appstate/, camada de baixo) e devolvem zero.
type fakeAppStateStore struct {
	version uint64
	hash    [128]byte
	getErr  error
	delErr  error
	macErr  error
	// keepOnDelete simula um store que aceita o DELETE mas nao zera o estado.
	keepOnDelete bool
	deleted      []string
	puts         int
}

func (s *fakeAppStateStore) PutAppStateVersion(_ context.Context, _ string, version uint64, hash [128]byte) error {
	s.puts++
	s.version, s.hash = version, hash
	return nil
}

func (s *fakeAppStateStore) GetAppStateVersion(context.Context, string) (uint64, [128]byte, error) {
	return s.version, s.hash, s.getErr
}

func (s *fakeAppStateStore) DeleteAppStateVersion(_ context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	if s.delErr != nil {
		return s.delErr
	}
	if s.keepOnDelete {
		return nil
	}
	s.version, s.hash = 0, [128]byte{}
	return nil
}

func (s *fakeAppStateStore) PutAppStateMutationMACs(context.Context, string, uint64, []store.AppStateMutationMAC) error {
	return nil
}
func (s *fakeAppStateStore) DeleteAppStateMutationMACs(context.Context, string, [][]byte) error {
	return nil
}
func (s *fakeAppStateStore) GetAppStateMutationMAC(context.Context, string, []byte) ([]byte, error) {
	return nil, s.macErr
}

// fakeKeyStore implementa store.AppStateSyncKeyStore.
type fakeKeyStore struct {
	latestID  []byte
	latestErr error
	// key, quando nao-nil, e' devolvida para qualquer ID pedido — o suficiente
	// para EncodePatch derivar as chaves de verdade e o envio seguir adiante.
	key *store.AppStateSyncKey
}

func (s *fakeKeyStore) PutAppStateSyncKey(context.Context, []byte, store.AppStateSyncKey) error {
	return nil
}
func (s *fakeKeyStore) GetAppStateSyncKey(context.Context, []byte) (*store.AppStateSyncKey, error) {
	return s.key, nil
}

// keyStoreComChave devolve um key store que aceita qualquer ID, com material de
// chave deterministico.
func keyStoreComChave() *fakeKeyStore {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}
	return &fakeKeyStore{
		latestID: []byte{1, 2, 3},
		key:      &store.AppStateSyncKey{Data: data, Fingerprint: []byte{9}, Timestamp: 1},
	}
}
func (s *fakeKeyStore) GetLatestAppStateSyncKeyID(context.Context) ([]byte, error) {
	return s.latestID, s.latestErr
}
func (s *fakeKeyStore) GetAllAppStateSyncKeys(context.Context) ([]*store.AppStateSyncKey, error) {
	return nil, nil
}

// fakeContactStore implementa store.ContactStore registrando o que recebeu.
type fakeContactStore struct {
	allNames   []store.ContactEntry
	allErr     error
	names      []types.JID
	putNameErr error
}

func (s *fakeContactStore) PutPushName(context.Context, types.JID, string) (bool, string, error) {
	return false, "", nil
}
func (s *fakeContactStore) PutBusinessName(context.Context, types.JID, string) (bool, string, error) {
	return false, "", nil
}
func (s *fakeContactStore) PutContactName(_ context.Context, user types.JID, _, _ string) error {
	s.names = append(s.names, user)
	return s.putNameErr
}
func (s *fakeContactStore) PutAllContactNames(_ context.Context, contacts []store.ContactEntry) error {
	s.allNames = append(s.allNames, contacts...)
	return s.allErr
}
func (s *fakeContactStore) PutManyRedactedPhones(context.Context, []store.RedactedPhoneEntry) error {
	return nil
}
func (s *fakeContactStore) GetContact(context.Context, types.JID) (types.ContactInfo, error) {
	return types.ContactInfo{}, nil
}
func (s *fakeContactStore) GetAllContacts(context.Context) (map[types.JID]types.ContactInfo, error) {
	return nil, nil
}

// fakeChatSettings implementa store.ChatSettingsStore.
type fakeChatSettings struct {
	mutedUntil map[types.JID]time.Time
	pinned     map[types.JID]bool
	archived   map[types.JID]bool
	err        error
}

func newFakeChatSettings() *fakeChatSettings {
	return &fakeChatSettings{
		mutedUntil: map[types.JID]time.Time{},
		pinned:     map[types.JID]bool{},
		archived:   map[types.JID]bool{},
	}
}

func (s *fakeChatSettings) PutMutedUntil(_ context.Context, chat types.JID, until time.Time) error {
	s.mutedUntil[chat] = until
	return s.err
}
func (s *fakeChatSettings) PutPinned(_ context.Context, chat types.JID, pinned bool) error {
	s.pinned[chat] = pinned
	return s.err
}
func (s *fakeChatSettings) PutArchived(_ context.Context, chat types.JID, archived bool) error {
	s.archived[chat] = archived
	return s.err
}
func (s *fakeChatSettings) GetChatSettings(context.Context, types.JID) (types.LocalChatSettings, error) {
	return types.LocalChatSettings{}, nil
}

// fakeContainer implementa o minimo de store.DeviceContainer para que
// Device.Save funcione (usado pelo ramo de setting_pushName).
type fakeContainer struct {
	saved int
	err   error
}

func (c *fakeContainer) PutDevice(context.Context, *store.Device) error {
	c.saved++
	return c.err
}
func (c *fakeContainer) DeleteDevice(context.Context, *store.Device) error { return nil }
