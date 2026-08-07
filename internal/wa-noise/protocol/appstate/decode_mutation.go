package appstate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/security/cbc"
)

// patchOutput acumula o resultado de decodificar as mutacoes de um patch: as
// mutacoes ja' traduzidas e o delta de MACs a persistir.
type patchOutput struct {
	RemovedMACs [][]byte
	AddedMACs   []store.AppStateMutationMAC
	Mutations   []Mutation
}

func (out *patchOutput) RemoveMAC(indexMAC []byte) {
	out.RemovedMACs = append(out.RemovedMACs, indexMAC)
	// If the mutation was previously added in this patch, remove it from AddedMACs
	out.AddedMACs = slices.DeleteFunc(out.AddedMACs, func(mac store.AppStateMutationMAC) bool {
		return hmac.Equal(mac.IndexMAC, indexMAC)
	})
}

func (out *patchOutput) AddMAC(indexMAC, valueMAC []byte) {
	out.AddedMACs = append(out.AddedMACs, store.AppStateMutationMAC{
		IndexMAC: indexMAC,
		ValueMAC: valueMAC,
	})
}

func (proc *Processor) decodeMutation(
	ctx context.Context,
	mutation *waServerSync.SyncdMutation,
	i int,
	validateMACs bool,
) (indexMAC, valueMAC []byte, index []string, syncAction *waSyncAction.SyncActionData, keys ExpandedAppStateKeys, err error) {
	keyID := mutation.GetRecord().GetKeyID().GetID()
	keys, err = proc.getAppStateKey(ctx, keyID)
	if err != nil {
		err = fmt.Errorf("failed to get key %X to decode mutation: %w", keyID, err)
		return
	}
	content := bytes.Clone(mutation.GetRecord().GetValue().GetBlob())
	content, valueMAC = content[:len(content)-macLength], content[len(content)-macLength:]
	if validateMACs {
		expectedValueMAC := generateContentMAC(mutation.GetOperation(), content, keyID, keys.ValueMAC)
		if !hmac.Equal(expectedValueMAC, valueMAC) {
			err = fmt.Errorf("failed to verify mutation #%d: %w", i+1, ErrMismatchingContentMAC)
			return
		}
	}
	iv, content := content[:cbcIVLength], content[cbcIVLength:]
	plaintext, err := cbcutil.Decrypt(keys.ValueEncryption, iv, content)
	if err != nil {
		err = fmt.Errorf("failed to decrypt mutation #%d: %w", i+1, err)
		return
	}
	syncAction = &waSyncAction.SyncActionData{}
	err = proto.Unmarshal(plaintext, syncAction)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal mutation #%d: %w", i+1, err)
		return
	}
	indexMAC = mutation.GetRecord().GetIndex().GetBlob()
	if validateMACs {
		expectedIndexMAC := concatAndHMAC(sha256.New, keys.Index, syncAction.Index)
		if !hmac.Equal(expectedIndexMAC, indexMAC) {
			err = fmt.Errorf("failed to verify mutation #%d: %w", i+1, ErrMismatchingIndexMAC)
			return
		}
	}
	err = json.Unmarshal(syncAction.GetIndex(), &index)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal index of mutation #%d: %w", i+1, err)
	}
	return
}

func indexMACToArray(indexMAC []byte) [macLength]byte {
	if len(indexMAC) != macLength {
		return [macLength]byte{}
	}
	return *(*[macLength]byte)(indexMAC)
}

func (proc *Processor) decodeMutations(
	ctx context.Context,
	mutations []*waServerSync.SyncdMutation,
	out *patchOutput,
	validateMACs bool,
	patchVersion uint64,
	fakeIndexesToRemove map[[macLength]byte][]byte,
) error {
	for i, mutation := range mutations {
		indexMAC, valueMAC, index, syncAction, _, err := proc.decodeMutation(ctx, mutation, i, validateMACs)
		if err != nil {
			return err
		}
		if mutation.GetOperation() == waServerSync.SyncdMutation_REMOVE {
			out.RemoveMAC(indexMAC)
			altIndexMAC, ok := fakeIndexesToRemove[indexMACToArray(indexMAC)]
			if ok && len(indexMAC) == macLength {
				out.RemoveMAC(altIndexMAC)
			}
		} else if mutation.GetOperation() == waServerSync.SyncdMutation_SET {
			out.AddMAC(indexMAC, valueMAC)
		}
		out.Mutations = append(out.Mutations, Mutation{
			KeyID:     mutation.GetRecord().GetKeyID().GetID(),
			Operation: mutation.GetOperation(),
			Action:    syncAction.GetValue(),
			Version:   syncAction.GetVersion(),
			Index:     index,
			IndexMAC:  indexMAC,
			ValueMAC:  valueMAC,

			PatchVersion: patchVersion,
		})
	}
	return nil
}

func (proc *Processor) storeMACs(ctx context.Context, name WAPatchName, currentState HashState, out *patchOutput) error {
	err := proc.Store.AppState.PutAppStateVersion(ctx, string(name), currentState.Version, currentState.Hash)
	if err != nil {
		return fmt.Errorf("failed to update app state version in the database: %w", err)
	}
	err = proc.Store.AppState.DeleteAppStateMutationMACs(ctx, string(name), out.RemovedMACs)
	if err != nil {
		return fmt.Errorf("failed to remove deleted mutation MACs from the database: %w", err)
	}
	err = proc.Store.AppState.PutAppStateMutationMACs(ctx, string(name), currentState.Version, out.AddedMACs)
	if err != nil {
		return fmt.Errorf("failed to insert added mutation MACs to the database: %w", err)
	}
	return nil
}
