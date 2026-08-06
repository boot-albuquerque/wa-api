// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"time"

	"wa-api/internal/wa-noise/types"
)

func (n *NoopStore) GetBufferedEvent(ctx context.Context, ciphertextHash [32]byte) (*BufferedEvent, error) {
	return nil, nil
}

func (n *NoopStore) PutBufferedEvent(ctx context.Context, ciphertextHash [32]byte, plaintext []byte, serverTimestamp time.Time) error {
	return nil
}

func (n *NoopStore) DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (n *NoopStore) ClearBufferedEventPlaintext(ctx context.Context, ciphertextHash [32]byte) error {
	return nil
}

func (n *NoopStore) DeleteOldBufferedHashes(ctx context.Context) error {
	return nil
}

func (n *NoopStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	return types.JID{}, n.Error
}

func (n *NoopStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	return nil, n.Error
}

func (n *NoopStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	return types.JID{}, n.Error
}

func (n *NoopStore) PutManyLIDMappings(ctx context.Context, mappings []LIDMapping) error {
	return n.Error
}

func (n *NoopStore) PutLIDMapping(ctx context.Context, lid types.JID, jid types.JID) error {
	return n.Error
}

func (n *NoopStore) DeleteOldOutgoingEvents(ctx context.Context) error {
	return nil
}

func (n *NoopStore) GetOutgoingEvent(ctx context.Context, chatJID, altChatJID types.JID, id types.MessageID) (string, []byte, error) {
	return "", nil, nil
}

func (n *NoopStore) AddOutgoingEvent(ctx context.Context, chatJID types.JID, id types.MessageID, format string, plaintext []byte) error {
	return nil
}
