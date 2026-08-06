// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"errors"

	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/util/keys"
)

type NoopStore struct {
	Error error
}

var nilStore = &NoopStore{Error: errors.New("store is nil")}
var nilKey = &keys.KeyPair{Priv: &[32]byte{}, Pub: &[32]byte{}}
var NoopDevice = &Device{
	ID:          &types.EmptyJID,
	NoiseKey:    nilKey,
	IdentityKey: nilKey,

	Identities:    nilStore,
	Sessions:      nilStore,
	PreKeys:       nilStore,
	SenderKeys:    nilStore,
	AppStateKeys:  nilStore,
	AppState:      nilStore,
	Contacts:      nilStore,
	ChatSettings:  nilStore,
	MsgSecrets:    nilStore,
	PrivacyTokens: nilStore,
	NCTSalt:       nilStore,
	EventBuffer:   nilStore,
	LIDs:          nilStore,
	Container:     nilStore,
}

var _ AllStores = (*NoopStore)(nil)
var _ DeviceContainer = (*NoopStore)(nil)

func (n *NoopStore) PutIdentity(ctx context.Context, address string, key [32]byte) error {
	return n.Error
}

func (n *NoopStore) DeleteAllIdentities(ctx context.Context, phone string) error {
	return n.Error
}

func (n *NoopStore) DeleteIdentity(ctx context.Context, address string) error {
	return n.Error
}

func (n *NoopStore) IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error) {
	return false, n.Error
}

func (n *NoopStore) GetSession(ctx context.Context, address string) ([]byte, error) {
	return nil, n.Error
}

func (n *NoopStore) HasSession(ctx context.Context, address string) (bool, error) {
	return false, n.Error
}

func (n *NoopStore) GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error) {
	return nil, n.Error
}

func (n *NoopStore) PutSession(ctx context.Context, address string, session []byte) error {
	return n.Error
}

func (n *NoopStore) PutManySessions(ctx context.Context, sessions map[string][]byte) error {
	return n.Error
}

func (n *NoopStore) DeleteAllSessions(ctx context.Context, phone string) error {
	return n.Error
}

func (n *NoopStore) DeleteSession(ctx context.Context, address string) error {
	return n.Error
}

func (n *NoopStore) MigratePNToLID(ctx context.Context, pn, lid types.JID) error {
	return n.Error
}

func (n *NoopStore) GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error) {
	return nil, n.Error
}

func (n *NoopStore) GenOnePreKey(ctx context.Context) (*keys.PreKey, error) {
	return nil, n.Error
}

func (n *NoopStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	return nil, n.Error
}

func (n *NoopStore) RemovePreKey(ctx context.Context, id uint32) error {
	return n.Error
}

func (n *NoopStore) MarkPreKeysAsUploaded(ctx context.Context, upToID uint32) error {
	return n.Error
}

func (n *NoopStore) UploadedPreKeyCount(ctx context.Context) (int, error) {
	return 0, n.Error
}

func (n *NoopStore) PutSenderKey(ctx context.Context, group, user string, session []byte) error {
	return n.Error
}

func (n *NoopStore) GetSenderKey(ctx context.Context, group, user string) ([]byte, error) {
	return nil, n.Error
}

func (n *NoopStore) PutDevice(ctx context.Context, store *Device) error {
	return n.Error
}

func (n *NoopStore) DeleteDevice(ctx context.Context, store *Device) error {
	return n.Error
}
