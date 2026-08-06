// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/libsignal/ecc"
	"go.mau.fi/libsignal/keys/chain"
	"go.mau.fi/libsignal/state/record"

	"wa-api/internal/wa-noise/util/keys"
)

// memSessionStore e' um SessionStore em memoria: SessionStore e' interface,
// entao o cache de sessao roda inteiro sem banco.
type memSessionStore struct {
	NoopStore
	sessions       map[string][]byte
	getManyErr     error
	putManyErr     error
	putManyCalls   int
	lastPutSession map[string][]byte
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{
		NoopStore: NoopStore{Error: errors.New("nao usado")},
		sessions:  make(map[string][]byte),
	}
}

func (m *memSessionStore) GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error) {
	if m.getManyErr != nil {
		return nil, m.getManyErr
	}
	out := make(map[string][]byte, len(addresses))
	for _, addr := range addresses {
		out[addr] = m.sessions[addr]
	}
	return out, nil
}

func (m *memSessionStore) PutManySessions(ctx context.Context, sessions map[string][]byte) error {
	m.putManyCalls++
	m.lastPutSession = sessions
	if m.putManyErr != nil {
		return m.putManyErr
	}
	for addr, sess := range sessions {
		m.sessions[addr] = sess
	}
	return nil
}

// newEmptySession devolve o record que LoadSession/WithCachedSessions produzem
// para um endereco sem sessao gravada. ATENCAO: ele NAO e' serializavel — o
// libsignal panica em Serialize() porque as chaves de identidade e o root key
// ainda sao nil. Isso e' correto no dominio (uma sessao que nunca fez handshake
// nao tem o que persistir), mas significa que testes de FLUSH precisam de
// newStoredSession.
func newEmptySession() *record.Session {
	return record.NewSession(SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
}

// newStoredSession monta um record.Session com estado minimo porem COMPLETO —
// o unico tipo que sobrevive a um Serialize()/NewSessionFromBytes. Serve de
// duplo para uma sessao que ja' passou pelo handshake.
func newStoredSession(t *testing.T) *record.Session {
	t.Helper()
	identityKey := keys.NewKeyPair()
	ratchetKey := keys.NewKeyPair()
	pub := ecc.NewDjbECPublicKey(*identityKey.Pub).Serialize()

	state, err := record.NewStateFromStructure(&record.StateStructure{
		LocalIdentityPublic:  pub,
		RemoteIdentityPublic: pub,
		SenderBaseKey:        pub,
		RootKey:              make([]byte, 32),
		SessionVersion:       3,
		SenderChain: &record.ChainStructure{
			SenderRatchetKeyPublic:  ecc.NewDjbECPublicKey(*ratchetKey.Pub).Serialize(),
			SenderRatchetKeyPrivate: ratchetKey.Priv[:],
			ChainKey:                &chain.KeyStructure{Key: make([]byte, 32)},
		},
	}, SignalProtobufSerializer.State)
	if err != nil {
		t.Fatalf("montar sessao de teste: %v", err)
	}
	return record.NewSessionFromState(state, SignalProtobufSerializer.Session)
}

// TestStoredSessionFixtureRoundTrips prova que o duplo acima e' de fato
// serializavel — se o libsignal mudar a forma exigida, este teste falha antes
// dos que dependem dele.
func TestStoredSessionFixtureRoundTrips(t *testing.T) {
	sess := newStoredSession(t)
	raw := sess.Serialize()
	back, err := record.NewSessionFromBytes(raw, SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
	if err != nil {
		t.Fatalf("NewSessionFromBytes: %v", err)
	}
	if string(back.Serialize()) != string(raw) {
		t.Fatal("o round trip da sessao de teste divergiu")
	}
}

// TestFreshSessionIsNotSerializable documenta a assimetria acima: o record
// vazio devolvido para um endereco desconhecido nao pode ser persistido. Quem
// mexer no cache de sessao precisa saber disso — foi o que motivou o
// newStoredSession.
func TestFreshSessionIsNotSerializable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("esperava panic ao serializar uma sessao sem handshake")
		}
	}()
	_ = newEmptySession().Serialize()
}

func newDeviceWithSessions(m *memSessionStore) *Device {
	return &Device{Sessions: m}
}

func TestGetSessionCacheWithoutContextValue(t *testing.T) {
	if getSessionCache(context.Background()) != nil {
		t.Fatal("um contexto sem cache deveria devolver nil")
	}
	//nolint:staticcheck // testar ctx nil e' exatamente o ramo que existe no codigo
	if getSessionCache(nil) != nil {
		t.Fatal("um contexto nil deveria devolver nil")
	}
}

// O contexto e' um saco de chaves compartilhado; um valor de outro tipo na
// mesma chave nao pode virar type-assert panic.
func TestGetSessionCacheIgnoresWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKeySessionCache, "nao e' um cache")
	if getSessionCache(ctx) != nil {
		t.Fatal("um valor de tipo errado deveria ser ignorado, nao panicar")
	}
}

func TestGetAndPutCachedSessionWithoutCacheAreNoOps(t *testing.T) {
	ctx := context.Background()
	if getCachedSession(ctx, "alice:0") != nil {
		t.Fatal("sem cache no contexto, getCachedSession deveria devolver nil")
	}
	if putCachedSession(ctx, "alice:0", newStoredSession(t)) {
		t.Fatal("sem cache no contexto, putCachedSession deveria devolver false")
	}
}

func TestWithCachedSessionsEmptyAddressesIsNoOp(t *testing.T) {
	ctx := context.Background()
	existing, newCtx, err := newDeviceWithSessions(newMemSessionStore()).WithCachedSessions(ctx, nil)
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	if existing != nil {
		t.Fatalf("esperava nil, veio %v", existing)
	}
	if newCtx != ctx {
		t.Fatal("sem enderecos, o contexto deveria voltar inalterado")
	}
}

// WithCachedSessions devolve, por endereco, se a sessao JA' EXISTIA. Quem chama
// usa isso para decidir se precisa buscar prekeys do servidor.
func TestWithCachedSessionsReportsWhichSessionsExisted(t *testing.T) {
	m := newMemSessionStore()
	m.sessions["alice:0"] = newStoredSession(t).Serialize()
	m.sessions["bob:0"] = nil

	existing, ctx, err := newDeviceWithSessions(m).WithCachedSessions(
		context.Background(), []string{"alice:0", "bob:0"},
	)
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	if !existing["alice:0"] {
		t.Fatal("alice:0 tinha sessao gravada e deveria vir como existente")
	}
	if existing["bob:0"] {
		t.Fatal("bob:0 nao tinha sessao e nao deveria vir como existente")
	}
	// Ambos ficam no cache: o ausente com um record novo e vazio.
	if getCachedSession(ctx, "alice:0") == nil {
		t.Fatal("alice:0 deveria estar no cache")
	}
	if getCachedSession(ctx, "bob:0") == nil {
		t.Fatal("bob:0 deveria estar no cache com uma sessao vazia")
	}
}

func TestWithCachedSessionsPropagatesStoreError(t *testing.T) {
	m := newMemSessionStore()
	boom := errors.New("boom")
	m.getManyErr = boom
	_, _, err := newDeviceWithSessions(m).WithCachedSessions(context.Background(), []string{"alice:0"})
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

// Uma sessao corrompida no banco e' logada e PULADA, nao aborta o lote — senao
// um unico registro invalido travaria o envio para todo o grupo.
func TestWithCachedSessionsSkipsUndeserializableSessions(t *testing.T) {
	m := newMemSessionStore()
	m.sessions["quebrada:0"] = []byte("nao e' um record valido")
	m.sessions["boa:0"] = newStoredSession(t).Serialize()

	existing, ctx, err := newDeviceWithSessions(m).WithCachedSessions(
		context.Background(), []string{"quebrada:0", "boa:0"},
	)
	if err != nil {
		t.Fatalf("WithCachedSessions nao deveria falhar por uma sessao corrompida: %v", err)
	}
	if _, ok := existing["quebrada:0"]; ok {
		t.Fatal("a sessao corrompida nao deveria entrar no mapa de existentes")
	}
	if getCachedSession(ctx, "quebrada:0") != nil {
		t.Fatal("a sessao corrompida nao deveria entrar no cache")
	}
	if !existing["boa:0"] {
		t.Fatal("a sessao boa deveria ter sido carregada normalmente")
	}
}

func TestPutCachedSessionMarksDirtyAndIsReadable(t *testing.T) {
	m := newMemSessionStore()
	_, ctx, err := newDeviceWithSessions(m).WithCachedSessions(context.Background(), []string{"alice:0"})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	sess := newStoredSession(t)
	if !putCachedSession(ctx, "alice:0", sess) {
		t.Fatal("putCachedSession deveria devolver true com cache no contexto")
	}
	if getCachedSession(ctx, "alice:0") != sess {
		t.Fatal("getCachedSession deveria devolver o record recem-gravado")
	}
}

// So' as entradas marcadas Dirty (ou seja, escritas via putCachedSession) vao
// para o banco. As meramente lidas nao sao reescritas.
func TestPutCachedSessionsFlushesOnlyDirtyEntries(t *testing.T) {
	ctx := context.Background()
	m := newMemSessionStore()
	m.sessions["lida:0"] = newStoredSession(t).Serialize()
	device := newDeviceWithSessions(m)

	_, ctx, err := device.WithCachedSessions(ctx, []string{"lida:0", "escrita:0"})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	putCachedSession(ctx, "escrita:0", newStoredSession(t))

	if err = device.PutCachedSessions(ctx); err != nil {
		t.Fatalf("PutCachedSessions: %v", err)
	}
	if m.putManyCalls != 1 {
		t.Fatalf("PutManySessions chamado %d vezes", m.putManyCalls)
	}
	if len(m.lastPutSession) != 1 {
		t.Fatalf("deveria persistir so' a entrada suja, veio %v", m.lastPutSession)
	}
	if _, ok := m.lastPutSession["escrita:0"]; !ok {
		t.Fatalf("a entrada suja deveria ter sido persistida, veio %v", m.lastPutSession)
	}
}

func TestPutCachedSessionsWithoutDirtyEntriesSkipsStore(t *testing.T) {
	ctx := context.Background()
	m := newMemSessionStore()
	m.sessions["lida:0"] = newStoredSession(t).Serialize()
	device := newDeviceWithSessions(m)

	_, ctx, err := device.WithCachedSessions(ctx, []string{"lida:0"})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	if err = device.PutCachedSessions(ctx); err != nil {
		t.Fatalf("PutCachedSessions: %v", err)
	}
	if m.putManyCalls != 0 {
		t.Fatalf("sem entrada suja, PutManySessions nao deveria ser chamado (calls=%d)", m.putManyCalls)
	}
}

func TestPutCachedSessionsWithoutCacheIsNoOp(t *testing.T) {
	m := newMemSessionStore()
	if err := newDeviceWithSessions(m).PutCachedSessions(context.Background()); err != nil {
		t.Fatalf("PutCachedSessions sem cache: %v", err)
	}
	if m.putManyCalls != 0 {
		t.Fatal("sem cache no contexto nao deveria haver escrita")
	}
}

func TestPutCachedSessionsPropagatesStoreError(t *testing.T) {
	ctx := context.Background()
	m := newMemSessionStore()
	boom := errors.New("boom")
	m.putManyErr = boom
	device := newDeviceWithSessions(m)

	_, ctx, err := device.WithCachedSessions(ctx, []string{"alice:0"})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	putCachedSession(ctx, "alice:0", newStoredSession(t))
	if err = device.PutCachedSessions(ctx); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

// Depois do flush o cache e' limpo: manter as entradas faria uma segunda
// chamada reescrever tudo de novo, e faria leituras posteriores servirem estado
// que ja' foi para o banco.
func TestPutCachedSessionsClearsCache(t *testing.T) {
	ctx := context.Background()
	m := newMemSessionStore()
	device := newDeviceWithSessions(m)

	_, ctx, err := device.WithCachedSessions(ctx, []string{"alice:0"})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	putCachedSession(ctx, "alice:0", newStoredSession(t))
	if err = device.PutCachedSessions(ctx); err != nil {
		t.Fatalf("PutCachedSessions 1: %v", err)
	}
	if getCachedSession(ctx, "alice:0") != nil {
		t.Fatal("o cache deveria ter sido limpo apos o flush")
	}
	if err = device.PutCachedSessions(ctx); err != nil {
		t.Fatalf("PutCachedSessions 2: %v", err)
	}
	if m.putManyCalls != 1 {
		t.Fatalf("o segundo flush nao deveria escrever nada (calls=%d)", m.putManyCalls)
	}
}
