package store

import (
	"context"
	"errors"
	"testing"

	groupRecord "go.mau.fi/libsignal/groups/state/record"
	"go.mau.fi/libsignal/protocol"

	"wa-api/internal/wa-noise/security/keys"
)

// signalDevice monta um Device com chaves reais e stores em memoria — o
// adaptador Signal e' pura traducao entre os tipos do libsignal e as
// interfaces de store, entao nao precisa de banco.
func signalDevice(t *testing.T) (*Device, *memSignalStore) {
	t.Helper()
	m := newMemSignalStore()
	device := &Device{
		IdentityKey:    keys.NewKeyPair(),
		RegistrationID: 4242,
		Identities:     m,
		Sessions:       m,
		PreKeys:        m,
		SenderKeys:     m,
	}
	device.SignedPreKey = device.IdentityKey.CreateSignedPreKey(1)
	return device, m
}

type memSignalStore struct {
	NoopStore
	identities map[string][32]byte
	sessions   map[string][]byte
	preKeys    map[uint32]*keys.PreKey
	senderKeys map[string][]byte

	identityErr  error
	sessionErr   error
	preKeyErr    error
	senderKeyErr error
}

func newMemSignalStore() *memSignalStore {
	return &memSignalStore{
		NoopStore:  NoopStore{Error: errors.New("nao usado")},
		identities: make(map[string][32]byte),
		sessions:   make(map[string][]byte),
		preKeys:    make(map[uint32]*keys.PreKey),
		senderKeys: make(map[string][]byte),
	}
}

func (m *memSignalStore) PutIdentity(ctx context.Context, address string, key [32]byte) error {
	if m.identityErr != nil {
		return m.identityErr
	}
	m.identities[address] = key
	return nil
}

func (m *memSignalStore) IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error) {
	if m.identityErr != nil {
		return false, m.identityErr
	}
	existing, ok := m.identities[address]
	return !ok || existing == key, nil
}

func (m *memSignalStore) GetSession(ctx context.Context, address string) ([]byte, error) {
	if m.sessionErr != nil {
		return nil, m.sessionErr
	}
	return m.sessions[address], nil
}

func (m *memSignalStore) HasSession(ctx context.Context, address string) (bool, error) {
	if m.sessionErr != nil {
		return false, m.sessionErr
	}
	_, ok := m.sessions[address]
	return ok, nil
}

func (m *memSignalStore) PutSession(ctx context.Context, address string, session []byte) error {
	if m.sessionErr != nil {
		return m.sessionErr
	}
	m.sessions[address] = session
	return nil
}

func (m *memSignalStore) GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error) {
	if m.sessionErr != nil {
		return nil, m.sessionErr
	}
	out := make(map[string][]byte, len(addresses))
	for _, addr := range addresses {
		out[addr] = m.sessions[addr]
	}
	return out, nil
}

func (m *memSignalStore) PutManySessions(ctx context.Context, sessions map[string][]byte) error {
	if m.sessionErr != nil {
		return m.sessionErr
	}
	for addr, sess := range sessions {
		m.sessions[addr] = sess
	}
	return nil
}

func (m *memSignalStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	if m.preKeyErr != nil {
		return nil, m.preKeyErr
	}
	return m.preKeys[id], nil
}

func (m *memSignalStore) RemovePreKey(ctx context.Context, id uint32) error {
	if m.preKeyErr != nil {
		return m.preKeyErr
	}
	delete(m.preKeys, id)
	return nil
}

func (m *memSignalStore) PutSenderKey(ctx context.Context, group, user string, session []byte) error {
	if m.senderKeyErr != nil {
		return m.senderKeyErr
	}
	m.senderKeys[group+"|"+user] = session
	return nil
}

func (m *memSignalStore) GetSenderKey(ctx context.Context, group, user string) ([]byte, error) {
	if m.senderKeyErr != nil {
		return nil, m.senderKeyErr
	}
	return m.senderKeys[group+"|"+user], nil
}

func TestDeviceImplementsSignalProtocolStore(t *testing.T) {
	// Redundante com o assert de compilacao em signal.go, mas deixa explicito
	// que este e' o contrato que o libsignal consome.
	device, _ := signalDevice(t)
	if device.GetLocalRegistrationID() != 4242 {
		t.Fatalf("GetLocalRegistrationID = %d", device.GetLocalRegistrationID())
	}
}

func TestGetIdentityKeyPairMirrorsDeviceKeys(t *testing.T) {
	device, _ := signalDevice(t)
	pair := device.GetIdentityKeyPair()
	if pair.PublicKey().PublicKey().PublicKey() != *device.IdentityKey.Pub {
		t.Fatal("a chave publica do par Signal nao bate com a do device")
	}
	if pair.PrivateKey().Serialize() != *device.IdentityKey.Priv {
		t.Fatal("a chave privada do par Signal nao bate com a do device")
	}
}

func signalAddr() *protocol.SignalAddress {
	return protocol.NewSignalAddress("5511999999999", 0)
}

func TestSaveIdentityAndIsTrustedIdentity(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)
	addr := signalAddr()
	identityKey := device.GetIdentityKeyPair().PublicKey()

	trusted, err := device.IsTrustedIdentity(ctx, addr, identityKey)
	if err != nil {
		t.Fatalf("IsTrustedIdentity antes: %v", err)
	}
	if !trusted {
		t.Fatal("identidade desconhecida deveria ser confiavel")
	}

	if err = device.SaveIdentity(ctx, addr, identityKey); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}
	trusted, err = device.IsTrustedIdentity(ctx, addr, identityKey)
	if err != nil {
		t.Fatalf("IsTrustedIdentity depois: %v", err)
	}
	if !trusted {
		t.Fatal("a identidade salva deveria continuar confiavel")
	}
}

func TestSaveIdentityWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.identityErr = boom
	err := device.SaveIdentity(context.Background(), signalAddr(), device.GetIdentityKeyPair().PublicKey())
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store encapsulado, veio %v", err)
	}
	if err.Error() == boom.Error() {
		t.Fatal("o erro deveria ser encapsulado com contexto do endereco")
	}
}

func TestIsTrustedIdentityWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.identityErr = boom
	_, err := device.IsTrustedIdentity(context.Background(), signalAddr(), device.GetIdentityKeyPair().PublicKey())
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

// LoadSession de um endereco sem sessao devolve um record NOVO e vazio, nao
// nil: o libsignal espera poder popular esse record durante o handshake.
func TestLoadSessionWithoutStoredSessionReturnsFreshRecord(t *testing.T) {
	device, _ := signalDevice(t)
	sess, err := device.LoadSession(context.Background(), signalAddr())
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if sess == nil {
		t.Fatal("LoadSession deveria devolver um record vazio, nao nil")
	}
}

func TestStoreAndLoadSessionRoundTrip(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)
	addr := signalAddr()

	original := newStoredSession(t)
	if err := device.StoreSession(ctx, addr, original); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	loaded, err := device.LoadSession(ctx, addr)
	if err != nil {
		t.Fatalf("LoadSession depois: %v", err)
	}
	if string(loaded.Serialize()) != string(original.Serialize()) {
		t.Fatal("a sessao nao voltou igual")
	}
}

func TestLoadSessionRejectsCorruptedBytes(t *testing.T) {
	device, m := signalDevice(t)
	m.sessions[signalAddr().String()] = []byte("nao e' um record valido")
	_, err := device.LoadSession(context.Background(), signalAddr())
	if err == nil {
		t.Fatal("uma sessao corrompida deveria produzir erro de desserializacao")
	}
}

func TestLoadSessionWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.sessionErr = boom
	if _, err := device.LoadSession(context.Background(), signalAddr()); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

// LoadSession consulta primeiro o cache por contexto (sessioncache.go) e so'
// entao o store. StoreSession, por simetria, escreve no cache quando ele
// existe e nao toca no banco — o flush e' feito por PutCachedSessions.
func TestLoadSessionPrefersContextCache(t *testing.T) {
	ctx := context.Background()
	device, m := signalDevice(t)
	addr := signalAddr()
	m.sessions[addr.String()] = newStoredSession(t).Serialize()

	_, ctx, err := device.WithCachedSessions(ctx, []string{addr.String()})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	cached := getCachedSession(ctx, addr.String())
	loaded, err := device.LoadSession(ctx, addr)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded != cached {
		t.Fatal("LoadSession deveria devolver o record do cache de contexto")
	}
}

func TestStoreSessionWritesToContextCacheInsteadOfStore(t *testing.T) {
	ctx := context.Background()
	device, m := signalDevice(t)
	addr := signalAddr()

	_, ctx, err := device.WithCachedSessions(ctx, []string{addr.String()})
	if err != nil {
		t.Fatalf("WithCachedSessions: %v", err)
	}
	sess := newStoredSession(t)
	if err = device.StoreSession(ctx, addr, sess); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	if _, wrote := m.sessions[addr.String()]; wrote {
		t.Fatal("com cache no contexto, StoreSession nao deveria escrever direto no store")
	}
	if getCachedSession(ctx, addr.String()) != sess {
		t.Fatal("StoreSession deveria ter gravado no cache")
	}
}

func TestContainsSession(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)
	addr := signalAddr()

	has, err := device.ContainsSession(ctx, addr)
	if err != nil {
		t.Fatalf("ContainsSession: %v", err)
	}
	if has {
		t.Fatal("nao deveria haver sessao ainda")
	}

	if err = device.StoreSession(ctx, addr, newStoredSession(t)); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	if has, err = device.ContainsSession(ctx, addr); err != nil || !has {
		t.Fatalf("ContainsSession = (%v, %v), esperava (true, nil)", has, err)
	}
}

func TestContainsSessionWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.sessionErr = boom
	if _, err := device.ContainsSession(context.Background(), signalAddr()); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

func TestLoadPreKeyMissingReturnsNil(t *testing.T) {
	device, _ := signalDevice(t)
	preKey, err := device.LoadPreKey(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadPreKey: %v", err)
	}
	if preKey != nil {
		t.Fatal("prekey inexistente deveria devolver nil")
	}
}

func TestLoadPreKeyRoundTrip(t *testing.T) {
	device, m := signalDevice(t)
	stored := keys.NewPreKey(7)
	m.preKeys[7] = stored

	preKey, err := device.LoadPreKey(context.Background(), 7)
	if err != nil {
		t.Fatalf("LoadPreKey: %v", err)
	}
	if preKey == nil {
		t.Fatal("LoadPreKey devolveu nil")
	}
	if preKey.ID().Value != 7 {
		t.Fatalf("ID = %d", preKey.ID().Value)
	}
	if preKey.KeyPair().PublicKey().PublicKey() != *stored.Pub {
		t.Fatal("a chave publica nao bate")
	}
}

func TestLoadPreKeyWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.preKeyErr = boom
	if _, err := device.LoadPreKey(context.Background(), 1); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

func TestRemovePreKey(t *testing.T) {
	device, m := signalDevice(t)
	m.preKeys[7] = keys.NewPreKey(7)
	if err := device.RemovePreKey(context.Background(), 7); err != nil {
		t.Fatalf("RemovePreKey: %v", err)
	}
	if _, ok := m.preKeys[7]; ok {
		t.Fatal("a prekey deveria ter sido removida")
	}
}

func TestRemovePreKeyWrapsStoreError(t *testing.T) {
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.preKeyErr = boom
	if err := device.RemovePreKey(context.Background(), 1); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do store, veio %v", err)
	}
}

// A signed pre key nao vem do store: vive no proprio Device. Um ID diferente do
// atual devolve nil (a chave antiga nao e' guardada).
func TestLoadSignedPreKey(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)

	spk, err := device.LoadSignedPreKey(ctx, device.SignedPreKey.KeyID)
	if err != nil {
		t.Fatalf("LoadSignedPreKey: %v", err)
	}
	if spk == nil {
		t.Fatal("LoadSignedPreKey do ID atual nao deveria ser nil")
	}
	if spk.KeyPair().PublicKey().PublicKey() != *device.SignedPreKey.Pub {
		t.Fatal("a chave publica nao bate")
	}

	spk, err = device.LoadSignedPreKey(ctx, device.SignedPreKey.KeyID+1)
	if err != nil {
		t.Fatalf("LoadSignedPreKey outro ID: %v", err)
	}
	if spk != nil {
		t.Fatal("um ID diferente do atual deveria devolver nil")
	}
}

func senderKeyName() *protocol.SenderKeyName {
	return protocol.NewSenderKeyName("grupo@g.us", protocol.NewSignalAddress("5511999999999", 0))
}

func TestLoadSenderKeyWithoutStoredKeyReturnsFreshRecord(t *testing.T) {
	device, _ := signalDevice(t)
	key, err := device.LoadSenderKey(context.Background(), senderKeyName())
	if err != nil {
		t.Fatalf("LoadSenderKey: %v", err)
	}
	if key == nil {
		t.Fatal("LoadSenderKey deveria devolver um record vazio, nao nil")
	}
}

func TestStoreAndLoadSenderKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)
	name := senderKeyName()

	original := groupRecord.NewSenderKey(
		SignalProtobufSerializer.SenderKeyRecord, SignalProtobufSerializer.SenderKeyState,
	)
	if err := device.StoreSenderKey(ctx, name, original); err != nil {
		t.Fatalf("StoreSenderKey: %v", err)
	}
	loaded, err := device.LoadSenderKey(ctx, name)
	if err != nil {
		t.Fatalf("LoadSenderKey: %v", err)
	}
	if string(loaded.Serialize()) != string(original.Serialize()) {
		t.Fatal("a sender key nao voltou igual")
	}
}

func TestLoadSenderKeyRejectsCorruptedBytes(t *testing.T) {
	device, m := signalDevice(t)
	name := senderKeyName()
	m.senderKeys[name.GroupID()+"|"+name.Sender().String()] = []byte("invalido")
	if _, err := device.LoadSenderKey(context.Background(), name); err == nil {
		t.Fatal("uma sender key corrompida deveria produzir erro de desserializacao")
	}
}

func TestSenderKeyWrapsStoreErrors(t *testing.T) {
	ctx := context.Background()
	device, m := signalDevice(t)
	boom := errors.New("boom")
	m.senderKeyErr = boom
	name := senderKeyName()

	record := groupRecord.NewSenderKey(
		SignalProtobufSerializer.SenderKeyRecord, SignalProtobufSerializer.SenderKeyState,
	)
	if err := device.StoreSenderKey(ctx, name, record); !errors.Is(err, boom) {
		t.Fatalf("StoreSenderKey: esperava o erro do store, veio %v", err)
	}
	if _, err := device.LoadSenderKey(ctx, name); !errors.Is(err, boom) {
		t.Fatalf("LoadSenderKey: esperava o erro do store, veio %v", err)
	}
}

// Os metodos abaixo sao declaradamente nao implementados: o wa-noise nunca os
// chama, mas eles precisam existir para satisfazer store.SignalProtocol. O
// teste trava que a ausencia e' um panic explicito, nao um retorno silencioso
// que pareceria sucesso.
func TestUnimplementedSignalMethodsPanic(t *testing.T) {
	ctx := context.Background()
	device, _ := signalDevice(t)

	cases := map[string]func(){
		"StorePreKey":          func() { _ = device.StorePreKey(ctx, 1, nil) },
		"ContainsPreKey":       func() { _, _ = device.ContainsPreKey(ctx, 1) },
		"GetSubDeviceSessions": func() { _, _ = device.GetSubDeviceSessions(ctx, "x") },
		"DeleteSession":        func() { _ = device.DeleteSession(ctx, signalAddr()) },
		"DeleteAllSessions":    func() { _ = device.DeleteAllSessions(ctx) },
		"LoadSignedPreKeys":    func() { _, _ = device.LoadSignedPreKeys(ctx) },
		"StoreSignedPreKey":    func() { _ = device.StoreSignedPreKey(ctx, 1, nil) },
		"ContainsSignedPreKey": func() { _, _ = device.ContainsSignedPreKey(ctx, 1) },
		"RemoveSignedPreKey":   func() { _ = device.RemoveSignedPreKey(ctx, 1) },
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s deveria panicar com \"not implemented\"", name)
				}
			}()
			call()
		})
	}
}
