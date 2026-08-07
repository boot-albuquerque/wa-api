// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package appstate implements encoding and decoding WhatsApp's app state patches.
package appstate

import (
	"context"
	"encoding/base64"
	"sync"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/security/hkdf"
	waLog "wa-api/internal/wa-noise/observability/log"
)

type Processor struct {
	keyCache     map[string]ExpandedAppStateKeys
	keyCacheLock sync.Mutex
	Store        *store.Device
	Log          waLog.Logger
}

func NewProcessor(store *store.Device, log waLog.Logger) *Processor {
	return &Processor{
		keyCache: make(map[string]ExpandedAppStateKeys),
		Store:    store,
		Log:      log,
	}
}

type ExpandedAppStateKeys struct {
	Index           []byte
	ValueEncryption []byte
	ValueMAC        []byte
	SnapshotMAC     []byte
	PatchMAC        []byte
}

// appStateKeyHKDFInfo e' o rotulo HKDF que o protocolo usa para expandir a
// chave de app state sync nas cinco subchaves de `ExpandedAppStateKeys`.
const appStateKeyHKDFInfo = "WhatsApp Mutation Keys"

// appStateKeyPartLength e' o tamanho de cada uma das cinco subchaves
// derivadas, e appStateKeyExpandedLength o total pedido ao HKDF.
const (
	appStateKeyPartLength     = 32
	appStateKeyPartCount      = 5
	appStateKeyExpandedLength = appStateKeyPartLength * appStateKeyPartCount
)

// appStateKeyPart devolve a i-esima subchave (base zero) do material expandido.
func appStateKeyPart(expanded []byte, i int) []byte {
	return expanded[i*appStateKeyPartLength : (i+1)*appStateKeyPartLength]
}

func expandAppStateKeys(keyData []byte) (keys ExpandedAppStateKeys) {
	expanded := hkdfutil.SHA256(keyData, nil, []byte(appStateKeyHKDFInfo), appStateKeyExpandedLength)
	return ExpandedAppStateKeys{
		Index:           appStateKeyPart(expanded, 0),
		ValueEncryption: appStateKeyPart(expanded, 1),
		ValueMAC:        appStateKeyPart(expanded, 2),
		SnapshotMAC:     appStateKeyPart(expanded, 3),
		PatchMAC:        appStateKeyPart(expanded, 4),
	}
}

func (proc *Processor) getAppStateKey(ctx context.Context, keyID []byte) (keys ExpandedAppStateKeys, err error) {
	keyCacheID := base64.RawStdEncoding.EncodeToString(keyID)
	var ok bool

	proc.keyCacheLock.Lock()
	defer proc.keyCacheLock.Unlock()

	keys, ok = proc.keyCache[keyCacheID]
	if !ok {
		var keyData *store.AppStateSyncKey
		keyData, err = proc.Store.AppStateKeys.GetAppStateSyncKey(ctx, keyID)
		if keyData != nil {
			keys = expandAppStateKeys(keyData.Data)
			proc.keyCache[keyCacheID] = keys
		} else if err == nil {
			err = ErrKeyNotFound
		}
	}
	return
}

func (proc *Processor) GetMissingKeyIDs(ctx context.Context, pl *PatchList) [][]byte {
	cache := make(map[string]bool)
	var missingKeys [][]byte
	checkMissing := func(keyID []byte) {
		if keyID == nil {
			return
		}
		stringKeyID := base64.RawStdEncoding.EncodeToString(keyID)
		_, alreadyAdded := cache[stringKeyID]
		if !alreadyAdded {
			keyData, err := proc.Store.AppStateKeys.GetAppStateSyncKey(ctx, keyID)
			if err != nil {
				proc.Log.Warnf("Error fetching key %X while checking if it's missing: %v", keyID, err)
			}
			missing := keyData == nil && err == nil
			cache[stringKeyID] = missing
			if missing {
				missingKeys = append(missingKeys, keyID)
			}
		}
	}
	if pl.Snapshot != nil {
		checkMissing(pl.Snapshot.GetKeyID().GetID())
		for _, record := range pl.Snapshot.GetRecords() {
			checkMissing(record.GetKeyID().GetID())
		}
	}
	for _, patch := range pl.Patches {
		checkMissing(patch.GetKeyID().GetID())
	}
	return missingKeys
}
