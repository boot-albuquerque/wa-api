package contractsfake

import (
	"context"
	"errors"
	"strings"

	port "wa-api/pkg/application/contracts"
)

// ErrFakeS3SecretNotEnveloped é o que o cifrador dublê devolve para um valor
// não-vazio sem FakeS3EnvelopePrefix, espelhando auth.ErrS3SecretNotEnveloped.
var ErrFakeS3SecretNotEnveloped = errors.New("fake: S3 secret is not enveloped")

// --- S3ConfigStore -----------------------------------------------------

// S3ConfigStoreSaveCall é uma chamada a SaveS3Config.
type S3ConfigStoreSaveCall struct {
	Ctx    context.Context
	UserID string
	Config port.S3ConfigRecord
}

// S3ConfigStoreLoadCall é uma chamada a LoadS3Config ou a
// LoadS3ConfigWithoutSecret.
type S3ConfigStoreLoadCall struct {
	Ctx    context.Context
	UserID string
}

// S3ConfigStoreDeleteCall é uma chamada a DeleteS3Config.
type S3ConfigStoreDeleteCall struct {
	Ctx    context.Context
	UserID string
}

// S3ConfigStore é o fake de port.S3ConfigStore.
//
// O zero-value guarda em memória. As três regras que ele imita são as do
// adapter de produção (pkg/infra/db/s3_config_repository.go), e não
// simplificações:
//
//  1. gravar SUBSTITUI a configuração inteira (o UPDATE cobre as dez colunas);
//  2. LoadS3ConfigWithoutSecret devolve SecretKey VAZIA, porque o SELECT dele
//     não nomeia s3_secret_key — um dublê que devolvesse o segredo aqui faria
//     o teste de vazamento passar com o defeito no lugar;
//  3. apagar é idempotente e escreve o estado limpo histórico
//     (`41bc8e2^:handlers.go:6465`), não a ausência da linha.
type S3ConfigStore struct {
	// Stored é o estado observável do "banco" do dublê.
	Stored map[string]port.S3ConfigRecord

	SaveS3ConfigFunc              func(ctx context.Context, userID string, cfg port.S3ConfigRecord) error
	LoadS3ConfigFunc              func(ctx context.Context, userID string) (*port.S3ConfigRecord, error)
	LoadS3ConfigWithoutSecretFunc func(ctx context.Context, userID string) (*port.S3ConfigRecord, error)
	DeleteS3ConfigFunc            func(ctx context.Context, userID string) error

	SaveS3ConfigCalls              []S3ConfigStoreSaveCall
	LoadS3ConfigCalls              []S3ConfigStoreLoadCall
	LoadS3ConfigWithoutSecretCalls []S3ConfigStoreLoadCall
	DeleteS3ConfigCalls            []S3ConfigStoreDeleteCall
}

var _ port.S3ConfigStore = (*S3ConfigStore)(nil)

// ClearedS3Config é o estado que o DELETE grava, com os mesmos defaults do
// UPDATE histórico: path_style true, media_delivery "base64" e 30 dias.
func ClearedS3Config() port.S3ConfigRecord {
	return port.S3ConfigRecord{
		PathStyle:     true,
		MediaDelivery: "base64",
		RetentionDays: 30,
	}
}

// SaveS3Config implementa port.S3ConfigStore.
func (f *S3ConfigStore) SaveS3Config(ctx context.Context, userID string, cfg port.S3ConfigRecord) error {
	f.SaveS3ConfigCalls = append(f.SaveS3ConfigCalls, S3ConfigStoreSaveCall{Ctx: ctx, UserID: userID, Config: cfg})
	if f.SaveS3ConfigFunc != nil {
		return f.SaveS3ConfigFunc(ctx, userID, cfg)
	}
	f.put(userID, cfg)
	return nil
}

// LoadS3Config implementa port.S3ConfigStore.
func (f *S3ConfigStore) LoadS3Config(ctx context.Context, userID string) (*port.S3ConfigRecord, error) {
	f.LoadS3ConfigCalls = append(f.LoadS3ConfigCalls, S3ConfigStoreLoadCall{Ctx: ctx, UserID: userID})
	if f.LoadS3ConfigFunc != nil {
		return f.LoadS3ConfigFunc(ctx, userID)
	}
	cfg := f.Stored[userID]
	return &cfg, nil
}

// LoadS3ConfigWithoutSecret implementa port.S3ConfigStore.
func (f *S3ConfigStore) LoadS3ConfigWithoutSecret(ctx context.Context, userID string) (*port.S3ConfigRecord, error) {
	f.LoadS3ConfigWithoutSecretCalls = append(f.LoadS3ConfigWithoutSecretCalls, S3ConfigStoreLoadCall{Ctx: ctx, UserID: userID})
	if f.LoadS3ConfigWithoutSecretFunc != nil {
		return f.LoadS3ConfigWithoutSecretFunc(ctx, userID)
	}
	cfg := f.Stored[userID]
	cfg.SecretKey = ""
	return &cfg, nil
}

// DeleteS3Config implementa port.S3ConfigStore.
func (f *S3ConfigStore) DeleteS3Config(ctx context.Context, userID string) error {
	f.DeleteS3ConfigCalls = append(f.DeleteS3ConfigCalls, S3ConfigStoreDeleteCall{Ctx: ctx, UserID: userID})
	if f.DeleteS3ConfigFunc != nil {
		return f.DeleteS3ConfigFunc(ctx, userID)
	}
	f.put(userID, ClearedS3Config())
	return nil
}

func (f *S3ConfigStore) put(userID string, cfg port.S3ConfigRecord) {
	if f.Stored == nil {
		f.Stored = map[string]port.S3ConfigRecord{}
	}
	f.Stored[userID] = cfg
}

// --- S3SecretCipher ----------------------------------------------------

// S3SecretCipherCall é uma chamada a EncryptS3Secret ou a DecryptS3Secret.
type S3SecretCipherCall struct {
	Value string
}

// S3SecretCipher é o fake de port.S3SecretCipher.
//
// O zero-value envelopa com FakeS3EnvelopePrefix e desenvelopa exigindo o
// prefixo. As regras imitadas são as REAIS de pkg/infra/auth/s3_secret.go, e o
// dublê é tão exigente quanto ela nos três pontos que importam:
//
//  1. segredo vazio produz valor vazio, e não um envelope de nada;
//  2. valor não-vazio SEM prefixo é ERRO — nunca texto claro (ADR-0009);
//  3. o valor envelopado nunca contém o texto claro por acidente: ele contém,
//     e é de propósito, para que um teste de vazamento tenha o que procurar.
//     Por isso o prefixo do dublê é DIFERENTE do de produção — asserção que
//     confunde os dois estaria medindo o dublê.
type S3SecretCipher struct {
	EncryptS3SecretFunc func(plainSecret string) (string, error)
	DecryptS3SecretFunc func(storedSecret string) (string, error)

	EncryptS3SecretCalls []S3SecretCipherCall
	DecryptS3SecretCalls []S3SecretCipherCall
}

// FakeS3EnvelopePrefix marca a saída do cifrador dublê de S3.
const FakeS3EnvelopePrefix = "fake-enc:v1:"

var _ port.S3SecretCipher = (*S3SecretCipher)(nil)

// EncryptS3Secret implementa port.S3SecretCipher.
func (f *S3SecretCipher) EncryptS3Secret(plainSecret string) (string, error) {
	f.EncryptS3SecretCalls = append(f.EncryptS3SecretCalls, S3SecretCipherCall{Value: plainSecret})
	if f.EncryptS3SecretFunc != nil {
		return f.EncryptS3SecretFunc(plainSecret)
	}
	if plainSecret == "" {
		return "", nil
	}
	return FakeS3EnvelopePrefix + plainSecret, nil
}

// DecryptS3Secret implementa port.S3SecretCipher.
func (f *S3SecretCipher) DecryptS3Secret(storedSecret string) (string, error) {
	f.DecryptS3SecretCalls = append(f.DecryptS3SecretCalls, S3SecretCipherCall{Value: storedSecret})
	if f.DecryptS3SecretFunc != nil {
		return f.DecryptS3SecretFunc(storedSecret)
	}
	if storedSecret == "" {
		return "", nil
	}
	if !strings.HasPrefix(storedSecret, FakeS3EnvelopePrefix) {
		return "", ErrFakeS3SecretNotEnveloped
	}
	return strings.TrimPrefix(storedSecret, FakeS3EnvelopePrefix), nil
}

// --- S3ClientManager ---------------------------------------------------

// S3ClientManagerInitCall é uma chamada a InitializeS3Client.
type S3ClientManagerInitCall struct {
	UserID string
	Config port.S3ConfigRecord
}

// S3ClientManagerUserCall é uma chamada a RemoveClient ou a TestConnection.
type S3ClientManagerUserCall struct {
	Ctx    context.Context
	UserID string
}

// S3ClientManager é o fake de port.S3ClientManager.
//
// Registered é o estado observável do registro em memória, e é ele que o teste
// da revogação consulta: zerar o banco sem tirar o cliente daqui é exatamente
// o defeito que a F157 descreve, e o sintoma (200) é idêntico nos dois casos.
type S3ClientManager struct {
	Registered map[string]port.S3ConfigRecord

	InitializeS3ClientFunc func(userID string, cfg port.S3ConfigRecord) error
	TestConnectionFunc     func(ctx context.Context, userID string) error

	InitializeS3ClientCalls []S3ClientManagerInitCall
	RemoveClientCalls       []S3ClientManagerUserCall
	TestConnectionCalls     []S3ClientManagerUserCall
}

var _ port.S3ClientManager = (*S3ClientManager)(nil)

// InitializeS3Client implementa port.S3ClientManager.
func (f *S3ClientManager) InitializeS3Client(userID string, cfg port.S3ConfigRecord) error {
	f.InitializeS3ClientCalls = append(f.InitializeS3ClientCalls, S3ClientManagerInitCall{UserID: userID, Config: cfg})
	if f.InitializeS3ClientFunc != nil {
		return f.InitializeS3ClientFunc(userID, cfg)
	}
	if f.Registered == nil {
		f.Registered = map[string]port.S3ConfigRecord{}
	}
	f.Registered[userID] = cfg
	return nil
}

// RemoveClient implementa port.S3ClientManager.
func (f *S3ClientManager) RemoveClient(userID string) {
	f.RemoveClientCalls = append(f.RemoveClientCalls, S3ClientManagerUserCall{UserID: userID})
	delete(f.Registered, userID)
}

// TestConnection implementa port.S3ClientManager.
func (f *S3ClientManager) TestConnection(ctx context.Context, userID string) error {
	f.TestConnectionCalls = append(f.TestConnectionCalls, S3ClientManagerUserCall{Ctx: ctx, UserID: userID})
	if f.TestConnectionFunc != nil {
		return f.TestConnectionFunc(ctx, userID)
	}
	return nil
}

// --- UserInfoS3Cache ---------------------------------------------------

// UserInfoS3CacheCall é uma chamada a SetS3Config.
type UserInfoS3CacheCall struct {
	UserID        string
	Enabled       bool
	MediaDelivery string
}

// UserInfoS3Cache é o fake de port.UserInfoS3Cache.
type UserInfoS3Cache struct {
	SetS3ConfigFunc  func(userID string, enabled bool, mediaDelivery string)
	SetS3ConfigCalls []UserInfoS3CacheCall
}

var _ port.UserInfoS3Cache = (*UserInfoS3Cache)(nil)

// SetS3Config implementa port.UserInfoS3Cache.
func (f *UserInfoS3Cache) SetS3Config(userID string, enabled bool, mediaDelivery string) {
	f.SetS3ConfigCalls = append(f.SetS3ConfigCalls, UserInfoS3CacheCall{UserID: userID, Enabled: enabled, MediaDelivery: mediaDelivery})
	if f.SetS3ConfigFunc != nil {
		f.SetS3ConfigFunc(userID, enabled, mediaDelivery)
	}
}
