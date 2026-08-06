// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstate

import (
	"context"
	"crypto/hmac"
	"fmt"

	"wa-api/internal/wa-noise/proto/waServerSync"
)

func (proc *Processor) validateSnapshotMAC(ctx context.Context, name WAPatchName, currentState HashState, keyID, expectedSnapshotMAC []byte) (keys ExpandedAppStateKeys, err error) {
	keys, err = proc.getAppStateKey(ctx, keyID)
	if err != nil {
		err = fmt.Errorf("failed to get key %X to verify patch v%d MACs: %w", keyID, currentState.Version, err)
		return
	}
	snapshotMAC := currentState.generateSnapshotMAC(name, keys.SnapshotMAC)
	if !hmac.Equal(snapshotMAC, expectedSnapshotMAC) {
		err = fmt.Errorf("failed to verify patch v%d: %w", currentState.Version, ErrMismatchingLTHash)
	}
	return
}

func (proc *Processor) decodeSnapshot(
	ctx context.Context,
	name WAPatchName,
	ss *waServerSync.SyncdSnapshot,
	initialState HashState,
	validateMACs bool,
	newMutationsInput []Mutation,
) (newMutations []Mutation, currentState HashState, err error) {
	currentState = initialState
	currentState.Version = ss.GetVersion().GetVersion()

	encryptedMutations := make([]*waServerSync.SyncdMutation, len(ss.GetRecords()))
	for i, record := range ss.GetRecords() {
		encryptedMutations[i] = &waServerSync.SyncdMutation{
			Operation: waServerSync.SyncdMutation_SET.Enum(),
			Record:    record,
		}
	}

	var fakeIndexesToRemove map[[macLength]byte][]byte
	var warn []error
	warn, err = currentState.updateHash(encryptedMutations, func(indexMAC []byte, maxIndex int) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		err = fmt.Errorf("failed to update state hash: %w", err)
		return
	}

	if validateMACs {
		_, err = proc.validateSnapshotMAC(ctx, name, currentState, ss.GetKeyID().GetID(), ss.GetMac())
		if err != nil {
			if len(warn) > 0 {
				proc.Log.Warnf("Warnings while updating hash for %s: %+v", name, warn)
			}
			err = fmt.Errorf("failed to verify snapshot: %w", err)
			return
		}
	}

	var out patchOutput
	out.Mutations = newMutationsInput
	err = proc.decodeMutations(ctx, encryptedMutations, &out, validateMACs, currentState.Version, fakeIndexesToRemove)
	if err != nil {
		err = fmt.Errorf("failed to decode snapshot of v%d: %w", currentState.Version, err)
		return
	}
	err = proc.storeMACs(ctx, name, currentState, &out)
	if err != nil {
		return
	}
	newMutations = out.Mutations
	return
}

func (proc *Processor) validatePatch(
	ctx context.Context,
	patchName WAPatchName,
	patch *waServerSync.SyncdPatch,
	currentState HashState,
	validateMACs bool,
) (newState HashState, warn []error, err error) {
	version := patch.GetVersion().GetVersion()
	newState = currentState
	newState.Version = version
	warn, err = newState.updateHash(patch.GetMutations(), func(indexMAC []byte, maxIndex int) ([]byte, error) {
		for i := maxIndex - 1; i >= 0; i-- {
			if hmac.Equal(patch.Mutations[i].GetRecord().GetIndex().GetBlob(), indexMAC) {
				if patch.Mutations[i].GetOperation() == waServerSync.SyncdMutation_SET {
					value := patch.Mutations[i].GetRecord().GetValue().GetBlob()
					return value[len(value)-macLength:], nil
				}
				// Found a REMOVE operation, no previous value
				return nil, nil
			}
		}
		// Previous value not found in current patch, look in the database
		return proc.Store.AppState.GetAppStateMutationMAC(ctx, string(patchName), indexMAC)
	})
	if err != nil {
		err = fmt.Errorf("failed to update state hash: %w", err)
		return
	}

	if validateMACs {
		var keys ExpandedAppStateKeys
		keys, err = proc.validateSnapshotMAC(ctx, patchName, newState, patch.GetKeyID().GetID(), patch.GetSnapshotMAC())
		if err != nil {
			return
		}
		patchMAC := generatePatchMAC(patch, patchName, keys.PatchMAC, patch.GetVersion().GetVersion())
		if !hmac.Equal(patchMAC, patch.GetPatchMAC()) {
			err = fmt.Errorf("failed to verify patch v%d: %w", version, ErrMismatchingPatchMAC)
			return
		}
	}
	return
}

// DecodePatches will decode all the patches in a PatchList into a list of app state mutations.
func (proc *Processor) DecodePatches(
	ctx context.Context,
	list *PatchList,
	initialState HashState,
	validateMACs bool,
) (newMutations []Mutation, currentState HashState, err error) {
	currentState = initialState
	var expectedLength int
	if list.Snapshot != nil {
		expectedLength = len(list.Snapshot.GetRecords())
	}
	for _, patch := range list.Patches {
		expectedLength += len(patch.GetMutations())
	}
	newMutations = make([]Mutation, 0, expectedLength)

	if list.Snapshot != nil {
		newMutations, currentState, err = proc.decodeSnapshot(ctx, list.Name, list.Snapshot, currentState, validateMACs, newMutations)
		if err != nil {
			return
		}
	}

	for _, patch := range list.Patches {
		var out patchOutput
		var warn []error
		var newState HashState
		var fakeIndexesToRemove map[[macLength]byte][]byte
		newState, warn, err = proc.validatePatch(ctx, list.Name, patch, currentState, validateMACs)
		if err != nil {
			if len(warn) > 0 {
				proc.Log.Warnf("Warnings while updating hash for %s: %+v", list.Name, warn)
			}
			return
		}

		out.Mutations = newMutations
		err = proc.decodeMutations(ctx, patch.GetMutations(), &out, validateMACs, newState.Version, fakeIndexesToRemove)
		if err != nil {
			return
		}
		err = proc.storeMACs(ctx, list.Name, newState, &out)
		if err != nil {
			return
		}
		newMutations = out.Mutations
		currentState = newState
	}
	return
}
