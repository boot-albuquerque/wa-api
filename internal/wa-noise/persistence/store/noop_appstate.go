// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import "context"

func (n *NoopStore) PutAppStateSyncKey(ctx context.Context, id []byte, key AppStateSyncKey) error {
	return n.Error
}

func (n *NoopStore) GetAppStateSyncKey(ctx context.Context, id []byte) (*AppStateSyncKey, error) {
	return nil, n.Error
}

func (n *NoopStore) GetLatestAppStateSyncKeyID(ctx context.Context) ([]byte, error) {
	return nil, n.Error
}

func (n *NoopStore) GetAllAppStateSyncKeys(ctx context.Context) ([]*AppStateSyncKey, error) {
	return nil, nil
}

func (n *NoopStore) PutAppStateVersion(ctx context.Context, name string, version uint64, hash [128]byte) error {
	return n.Error
}

func (n *NoopStore) GetAppStateVersion(ctx context.Context, name string) (uint64, [128]byte, error) {
	return 0, [128]byte{}, n.Error
}

func (n *NoopStore) DeleteAppStateVersion(ctx context.Context, name string) error {
	return n.Error
}

func (n *NoopStore) PutAppStateMutationMACs(ctx context.Context, name string, version uint64, mutations []AppStateMutationMAC) error {
	return n.Error
}

func (n *NoopStore) DeleteAppStateMutationMACs(ctx context.Context, name string, indexMACs [][]byte) error {
	return n.Error
}

func (n *NoopStore) GetAppStateMutationMAC(ctx context.Context, name string, indexMAC []byte) (valueMAC []byte, err error) {
	return nil, n.Error
}
