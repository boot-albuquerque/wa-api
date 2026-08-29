package store

import (
	"context"
	"errors"

	"github.com/google/uuid"

	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/protocol/proto/waAdv"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

type DeviceContainer interface {
	PutDevice(ctx context.Context, store *Device) error
	DeleteDevice(ctx context.Context, store *Device) error
}

// Device e' a identidade persistida de um dispositivo pareado: as chaves
// Signal, o JID/LID e a agregacao de todos os stores por sessao.
type Device struct {
	Log sdklog.Logger

	NoiseKey       *keys.KeyPair
	IdentityKey    *keys.KeyPair
	SignedPreKey   *keys.PreKey
	RegistrationID uint32
	AdvSecretKey   []byte

	ID  *types.JID
	LID types.JID

	Account      *waAdv.ADVSignedDeviceIdentity
	Platform     string
	BusinessName string
	PushName     string

	LIDMigrationTimestamp int64

	FacebookUUID uuid.UUID

	Initialized   bool
	Deleted       bool
	Identities    IdentityStore
	Sessions      SessionStore
	PreKeys       PreKeyStore
	SenderKeys    SenderKeyStore
	AppStateKeys  AppStateSyncKeyStore
	AppState      AppStateStore
	Contacts      ContactStore
	ChatSettings  ChatSettingsStore
	MsgSecrets    MsgSecretStore
	PrivacyTokens PrivacyTokenStore
	NCTSalt       NCTSaltStore
	EventBuffer   EventBuffer
	LIDs          LIDStore
	Container     DeviceContainer
}

func (device *Device) GetJID() types.JID {
	if device == nil {
		return types.EmptyJID
	}
	id := device.ID
	if id == nil {
		return types.EmptyJID
	}
	return *id
}

func (device *Device) GetLID() types.JID {
	if device == nil {
		return types.EmptyJID
	}
	return device.LID
}

var ErrDeviceDeleted = errors.New("invalid use of deleted device")

func (device *Device) Save(ctx context.Context) error {
	if device.Deleted {
		return ErrDeviceDeleted
	}
	return device.Container.PutDevice(ctx, device)
}

func (device *Device) Delete(ctx context.Context) error {
	if device.Deleted {
		return nil
	}
	err := device.Container.DeleteDevice(ctx, device)
	if err != nil {
		return err
	}
	device.ID = nil
	device.LID = types.EmptyJID
	device.Deleted = true
	device.SetAllStores(&NoopStore{ErrDeviceDeleted})
	return nil
}

func (device *Device) SetAllStores(store AllSessionSpecificStores) {
	device.Identities = store
	device.Sessions = store
	device.PreKeys = store
	device.SenderKeys = store
	device.AppStateKeys = store
	device.AppState = store
	device.Contacts = store
	device.ChatSettings = store
	device.MsgSecrets = store
	device.PrivacyTokens = store
	device.NCTSalt = store
	device.EventBuffer = store
}

func (device *Device) GetAltJID(ctx context.Context, jid types.JID) (types.JID, error) {
	if device == nil {
		return types.EmptyJID, nil
	} else if jid.Server == types.DefaultUserServer {
		return device.LIDs.GetLIDForPN(ctx, jid)
	} else if jid.Server == types.HiddenUserServer {
		return device.LIDs.GetPNForLID(ctx, jid)
	} else {
		return types.EmptyJID, nil
	}
}
