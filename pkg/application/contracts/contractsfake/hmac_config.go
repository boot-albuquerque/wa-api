package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// --- HmacKeyStore ------------------------------------------------------

// HmacKeyStoreSaveCall é uma chamada a SaveHmacKey.
type HmacKeyStoreSaveCall struct {
	Ctx          context.Context
	UserID       string
	EncryptedKey []byte
}

// HmacKeyStoreLoadCall é uma chamada a LoadHmacKey.
type HmacKeyStoreLoadCall struct {
	Ctx    context.Context
	UserID string
}

// HmacKeyStoreDeleteCall é uma chamada a DeleteHmacKey.
type HmacKeyStoreDeleteCall struct {
	Ctx    context.Context
	UserID string
}

// HmacKeyStore é o fake de port.HmacKeyStore.
//
// O zero-value guarda em memória: SaveHmacKey grava em Stored, LoadHmacKey
// devolve o que está lá e DeleteHmacKey apaga. Não é permissividade — é a
// regra REAL do adapter de produção (pkg/infra/db/hmac_config_repository.go),
// em que gravar substitui, ler ausente devolve nil sem erro e apagar é
// idempotente.
type HmacKeyStore struct {
	// Stored é o estado observável do "banco" do dublê.
	Stored map[string][]byte

	SaveHmacKeyFunc   func(ctx context.Context, userID string, encryptedKey []byte) error
	LoadHmacKeyFunc   func(ctx context.Context, userID string) ([]byte, error)
	DeleteHmacKeyFunc func(ctx context.Context, userID string) error

	SaveHmacKeyCalls   []HmacKeyStoreSaveCall
	LoadHmacKeyCalls   []HmacKeyStoreLoadCall
	DeleteHmacKeyCalls []HmacKeyStoreDeleteCall
}

var _ port.HmacKeyStore = (*HmacKeyStore)(nil)

// SaveHmacKey implementa port.HmacKeyStore.
func (f *HmacKeyStore) SaveHmacKey(ctx context.Context, userID string, encryptedKey []byte) error {
	f.SaveHmacKeyCalls = append(f.SaveHmacKeyCalls, HmacKeyStoreSaveCall{Ctx: ctx, UserID: userID, EncryptedKey: encryptedKey})
	if f.SaveHmacKeyFunc != nil {
		return f.SaveHmacKeyFunc(ctx, userID, encryptedKey)
	}
	if f.Stored == nil {
		f.Stored = map[string][]byte{}
	}
	f.Stored[userID] = encryptedKey
	return nil
}

// LoadHmacKey implementa port.HmacKeyStore.
func (f *HmacKeyStore) LoadHmacKey(ctx context.Context, userID string) ([]byte, error) {
	f.LoadHmacKeyCalls = append(f.LoadHmacKeyCalls, HmacKeyStoreLoadCall{Ctx: ctx, UserID: userID})
	if f.LoadHmacKeyFunc != nil {
		return f.LoadHmacKeyFunc(ctx, userID)
	}
	return f.Stored[userID], nil
}

// DeleteHmacKey implementa port.HmacKeyStore.
func (f *HmacKeyStore) DeleteHmacKey(ctx context.Context, userID string) error {
	f.DeleteHmacKeyCalls = append(f.DeleteHmacKeyCalls, HmacKeyStoreDeleteCall{Ctx: ctx, UserID: userID})
	if f.DeleteHmacKeyFunc != nil {
		return f.DeleteHmacKeyFunc(ctx, userID)
	}
	delete(f.Stored, userID)
	return nil
}

// --- HmacKeyEncryptor --------------------------------------------------

// HmacKeyEncryptorCall é uma chamada a EncryptHmacKey.
type HmacKeyEncryptorCall struct {
	PlainKey string
}

// HmacKeyEncryptor é o fake de port.HmacKeyEncryptor.
//
// O zero-value devolve fakeCipherPrefix + a chave em claro. O prefixo existe
// para que uma asserção de "o que foi gravado NÃO é o texto plano" tenha algo
// concreto para distinguir — um dublê identidade faria esse teste passar com
// o defeito no lugar.
type HmacKeyEncryptor struct {
	EncryptHmacKeyFunc  func(plainKey string) ([]byte, error)
	EncryptHmacKeyCalls []HmacKeyEncryptorCall
}

// FakeCipherPrefix marca a saída do cifrador dublê.
const FakeCipherPrefix = "enc:"

var _ port.HmacKeyEncryptor = (*HmacKeyEncryptor)(nil)

// EncryptHmacKey implementa port.HmacKeyEncryptor.
func (f *HmacKeyEncryptor) EncryptHmacKey(plainKey string) ([]byte, error) {
	f.EncryptHmacKeyCalls = append(f.EncryptHmacKeyCalls, HmacKeyEncryptorCall{PlainKey: plainKey})
	if f.EncryptHmacKeyFunc != nil {
		return f.EncryptHmacKeyFunc(plainKey)
	}
	return []byte(FakeCipherPrefix + plainKey), nil
}

// --- UserInfoHmacCache -------------------------------------------------

// UserInfoHmacCacheCall é uma chamada a SetHmacKey.
type UserInfoHmacCacheCall struct {
	UserID       string
	EncryptedKey []byte
}

// UserInfoHmacCache é o fake de port.UserInfoHmacCache.
type UserInfoHmacCache struct {
	SetHmacKeyFunc  func(userID string, encryptedKey []byte)
	SetHmacKeyCalls []UserInfoHmacCacheCall
}

var _ port.UserInfoHmacCache = (*UserInfoHmacCache)(nil)

// SetHmacKey implementa port.UserInfoHmacCache.
func (f *UserInfoHmacCache) SetHmacKey(userID string, encryptedKey []byte) {
	f.SetHmacKeyCalls = append(f.SetHmacKeyCalls, UserInfoHmacCacheCall{UserID: userID, EncryptedKey: encryptedKey})
	if f.SetHmacKeyFunc != nil {
		f.SetHmacKeyFunc(userID, encryptedKey)
	}
}
