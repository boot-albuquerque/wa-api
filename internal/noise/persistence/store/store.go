// Package store contains interfaces for storing data needed for WhatsApp multidevice.
package store

import (
	"context"
	"time"

	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

type IdentityStore interface {
	PutIdentity(ctx context.Context, address string, key [32]byte) error
	DeleteAllIdentities(ctx context.Context, phone string) error
	DeleteIdentity(ctx context.Context, address string) error
	IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error)
}

type SessionStore interface {
	GetSession(ctx context.Context, address string) ([]byte, error)
	HasSession(ctx context.Context, address string) (bool, error)
	GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error)
	PutSession(ctx context.Context, address string, session []byte) error
	PutManySessions(ctx context.Context, sessions map[string][]byte) error
	DeleteAllSessions(ctx context.Context, phone string) error
	DeleteSession(ctx context.Context, address string) error
	MigratePNToLID(ctx context.Context, pn, lid types.JID) error
}

type PreKeyStore interface {
	GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error)
	GenOnePreKey(ctx context.Context) (*keys.PreKey, error)
	GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error)
	RemovePreKey(ctx context.Context, id uint32) error
	MarkPreKeysAsUploaded(ctx context.Context, upToID uint32) error
	UploadedPreKeyCount(ctx context.Context) (int, error)
}

type SenderKeyStore interface {
	PutSenderKey(ctx context.Context, group, user string, session []byte) error
	GetSenderKey(ctx context.Context, group, user string) ([]byte, error)
}

type AppStateSyncKey struct {
	Data        []byte
	Fingerprint []byte
	Timestamp   int64
}

type AppStateSyncKeyStore interface {
	PutAppStateSyncKey(ctx context.Context, id []byte, key AppStateSyncKey) error
	GetAppStateSyncKey(ctx context.Context, id []byte) (*AppStateSyncKey, error)
	GetLatestAppStateSyncKeyID(ctx context.Context) ([]byte, error)
	GetAllAppStateSyncKeys(ctx context.Context) ([]*AppStateSyncKey, error)
}

type AppStateMutationMAC struct {
	IndexMAC []byte
	ValueMAC []byte
}

type AppStateStore interface {
	PutAppStateVersion(ctx context.Context, name string, version uint64, hash [128]byte) error
	GetAppStateVersion(ctx context.Context, name string) (uint64, [128]byte, error)
	DeleteAppStateVersion(ctx context.Context, name string) error

	PutAppStateMutationMACs(ctx context.Context, name string, version uint64, mutations []AppStateMutationMAC) error
	DeleteAppStateMutationMACs(ctx context.Context, name string, indexMACs [][]byte) error
	GetAppStateMutationMAC(ctx context.Context, name string, indexMAC []byte) (valueMAC []byte, err error)
}

type ContactEntry struct {
	JID       types.JID
	FirstName string
	FullName  string
}

func (ce ContactEntry) GetMassInsertValues() [3]any {
	return [...]any{ce.JID.String(), ce.FirstName, ce.FullName}
}

type RedactedPhoneEntry struct {
	JID           types.JID
	RedactedPhone string
}

func (rpe RedactedPhoneEntry) GetMassInsertValues() [2]any {
	return [...]any{rpe.JID.String(), rpe.RedactedPhone}
}

type ContactStore interface {
	PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error)
	PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error)
	PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error
	PutAllContactNames(ctx context.Context, contacts []ContactEntry) error
	PutManyRedactedPhones(ctx context.Context, entries []RedactedPhoneEntry) error
	GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error)
	GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error)
}

var MutedForever = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)

type ChatSettingsStore interface {
	PutMutedUntil(ctx context.Context, chat types.JID, mutedUntil time.Time) error
	PutPinned(ctx context.Context, chat types.JID, pinned bool) error
	PutArchived(ctx context.Context, chat types.JID, archived bool) error
	GetChatSettings(ctx context.Context, chat types.JID) (types.LocalChatSettings, error)
}

type MessageSecretInsert struct {
	Chat   types.JID
	Sender types.JID
	ID     types.MessageID
	Secret []byte
}

type MsgSecretStore interface {
	PutMessageSecrets(ctx context.Context, inserts []MessageSecretInsert) error
	PutMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID, secret []byte) error
	GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error)
}

type PrivacyToken struct {
	User            types.JID
	Token           []byte
	Timestamp       time.Time
	SenderTimestamp time.Time
}

type PrivacyTokenStore interface {
	PutPrivacyTokens(ctx context.Context, tokens ...PrivacyToken) error
	GetPrivacyToken(ctx context.Context, user types.JID) (*PrivacyToken, error)
	DeleteExpiredPrivacyTokens(ctx context.Context, cutoff time.Time) (int64, error)
}

type NCTSaltStore interface {
	PutNCTSalt(ctx context.Context, salt []byte) error
	GetNCTSalt(ctx context.Context) ([]byte, error)
	DeleteNCTSalt(ctx context.Context) error
}

type BufferedEvent struct {
	Plaintext  []byte
	InsertTime time.Time
	ServerTime time.Time
}

type EventBuffer interface {
	GetBufferedEvent(ctx context.Context, ciphertextHash [32]byte) (*BufferedEvent, error)
	PutBufferedEvent(ctx context.Context, ciphertextHash [32]byte, plaintext []byte, serverTimestamp time.Time) error
	DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error
	ClearBufferedEventPlaintext(ctx context.Context, ciphertextHash [32]byte) error
	DeleteOldBufferedHashes(ctx context.Context) error

	GetOutgoingEvent(ctx context.Context, chatJID, altChatJID types.JID, id types.MessageID) (string, []byte, error)
	AddOutgoingEvent(ctx context.Context, chatJID types.JID, id types.MessageID, format string, plaintext []byte) error
	DeleteOldOutgoingEvents(ctx context.Context) error
}

type LIDMapping struct {
	LID types.JID
	PN  types.JID
}

func (lm LIDMapping) GetMassInsertValues() [2]any {
	return [...]any{lm.LID.User, lm.PN.User}
}

type LIDStore interface {
	PutManyLIDMappings(ctx context.Context, mappings []LIDMapping) error
	PutLIDMapping(ctx context.Context, lid, jid types.JID) error
	GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error)
	GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error)
	GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error)
}

type AllSessionSpecificStores interface {
	IdentityStore
	SessionStore
	PreKeyStore
	SenderKeyStore
	AppStateSyncKeyStore
	AppStateStore
	ContactStore
	ChatSettingsStore
	MsgSecretStore
	PrivacyTokenStore
	NCTSaltStore
	EventBuffer
}

type AllGlobalStores interface {
	LIDStore
}

type AllStores interface {
	AllSessionSpecificStores
	AllGlobalStores
}
