package appstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/proto/waSyncAction"
	"wa-api/internal/wa-noise/util/cbcutil"
)

// MutationInfo contains information about a single mutation to the app state.
type MutationInfo struct {
	// Index contains the thing being mutated (like `mute` or `pin_v1`), followed by parameters like the target JID.
	Index []string
	// Version is a static number that depends on the thing being mutated.
	Version int32
	// Value contains the data for the mutation.
	Value *waSyncAction.SyncActionValue
}

// PatchInfo contains information about a patch to the app state.
// A patch can contain multiple mutations, as long as all mutations are in the same app state type.
type PatchInfo struct {
	// Timestamp is the time when the patch was created. This will be filled automatically in EncodePatch if it's zero.
	Timestamp time.Time
	// Type is the app state type being mutated.
	Type WAPatchName
	// Mutations contains the individual mutations to apply to the app state in this patch.
	Mutations []MutationInfo
}

func (proc *Processor) EncodePatch(ctx context.Context, keyID []byte, state HashState, patchInfo PatchInfo) ([]byte, error) {
	keys, err := proc.getAppStateKey(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app state key details with key ID %x: %w", keyID, err)
	}

	if patchInfo.Timestamp.IsZero() {
		patchInfo.Timestamp = time.Now()
	}

	mutations := make([]*waServerSync.SyncdMutation, 0, len(patchInfo.Mutations))
	for _, mutationInfo := range patchInfo.Mutations {
		mutationInfo.Value.Timestamp = proto.Int64(patchInfo.Timestamp.UnixMilli())

		indexBytes, err := json.Marshal(mutationInfo.Index)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal mutation index: %w", err)
		}

		pbObj := &waSyncAction.SyncActionData{
			Index:   indexBytes,
			Value:   mutationInfo.Value,
			Padding: []byte{},
			Version: &mutationInfo.Version,
		}

		content, err := proto.Marshal(pbObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal mutation: %w", err)
		}

		encryptedContent, err := cbcutil.Encrypt(keys.ValueEncryption, nil, content)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt mutation: %w", err)
		}

		valueMac := generateContentMAC(waServerSync.SyncdMutation_SET, encryptedContent, keyID, keys.ValueMAC)
		indexMac := concatAndHMAC(sha256.New, keys.Index, indexBytes)

		mutations = append(mutations, &waServerSync.SyncdMutation{
			Operation: waServerSync.SyncdMutation_SET.Enum(),
			Record: &waServerSync.SyncdRecord{
				Index: &waServerSync.SyncdIndex{Blob: indexMac},
				Value: &waServerSync.SyncdValue{Blob: append(encryptedContent, valueMac...)},
				KeyID: &waServerSync.KeyId{ID: keyID},
			},
		})
	}

	warn, err := state.updateHash(mutations, func(indexMAC []byte, _ int) ([]byte, error) {
		return proc.Store.AppState.GetAppStateMutationMAC(ctx, string(patchInfo.Type), indexMAC)
	})
	if len(warn) > 0 {
		proc.Log.Warnf("Warnings while updating hash for %s (sending new app state): %+v", patchInfo.Type, warn)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update state hash: %w", err)
	}

	state.Version += 1

	syncdPatch := &waServerSync.SyncdPatch{
		SnapshotMAC: state.generateSnapshotMAC(patchInfo.Type, keys.SnapshotMAC),
		KeyID:       &waServerSync.KeyId{ID: keyID},
		Mutations:   mutations,
	}
	syncdPatch.PatchMAC = generatePatchMAC(syncdPatch, patchInfo.Type, keys.PatchMAC, state.Version)

	result, err := proto.Marshal(syncdPatch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compiled patch: %w", err)
	}

	return result, nil
}
