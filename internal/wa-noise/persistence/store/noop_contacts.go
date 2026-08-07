package store

import (
	"context"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

func (n *NoopStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return false, "", n.Error
}

func (n *NoopStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return false, "", n.Error
}

func (n *NoopStore) PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error {
	return n.Error
}

func (n *NoopStore) PutAllContactNames(ctx context.Context, contacts []ContactEntry) error {
	return n.Error
}

func (n *NoopStore) PutManyRedactedPhones(ctx context.Context, entries []RedactedPhoneEntry) error {
	return n.Error
}

func (n *NoopStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	return types.ContactInfo{}, n.Error
}

func (n *NoopStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	return nil, n.Error
}

func (n *NoopStore) PutMutedUntil(ctx context.Context, chat types.JID, mutedUntil time.Time) error {
	return n.Error
}

func (n *NoopStore) PutPinned(ctx context.Context, chat types.JID, pinned bool) error {
	return n.Error
}

func (n *NoopStore) PutArchived(ctx context.Context, chat types.JID, archived bool) error {
	return n.Error
}

func (n *NoopStore) GetChatSettings(ctx context.Context, chat types.JID) (types.LocalChatSettings, error) {
	return types.LocalChatSettings{}, n.Error
}

func (n *NoopStore) PutMessageSecrets(ctx context.Context, inserts []MessageSecretInsert) error {
	return n.Error
}

func (n *NoopStore) PutMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID, secret []byte) error {
	return n.Error
}

func (n *NoopStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error) {
	return nil, types.EmptyJID, n.Error
}

func (n *NoopStore) PutPrivacyTokens(ctx context.Context, tokens ...PrivacyToken) error {
	return n.Error
}

func (n *NoopStore) GetPrivacyToken(ctx context.Context, user types.JID) (*PrivacyToken, error) {
	return nil, n.Error
}

func (n *NoopStore) PutNCTSalt(ctx context.Context, salt []byte) error {
	return n.Error
}

func (n *NoopStore) GetNCTSalt(ctx context.Context) ([]byte, error) {
	return nil, n.Error
}

func (n *NoopStore) DeleteNCTSalt(ctx context.Context) error {
	return n.Error
}

func (n *NoopStore) DeleteExpiredPrivacyTokens(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, n.Error
}
