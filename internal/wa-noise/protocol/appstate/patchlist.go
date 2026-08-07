package appstate

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
)

// PatchList represents a decoded response to getting app state patches from the WhatsApp servers.
type PatchList struct {
	Name           WAPatchName
	HasMorePatches bool
	Patches        []*waServerSync.SyncdPatch
	Snapshot       *waServerSync.SyncdSnapshot
}

// DownloadExternalFunc is a function that can download a blob of external app state patches.
type DownloadExternalFunc func(context.Context, *waServerSync.ExternalBlobReference) ([]byte, error)

func parseSnapshotInternal(ctx context.Context, collection *waBinary.Node, downloadExternal DownloadExternalFunc) (*waServerSync.SyncdSnapshot, error) {
	snapshotNode := collection.GetChildByTag(snapshotNodeTag)
	rawSnapshot, ok := snapshotNode.Content.([]byte)
	if snapshotNode.Tag != snapshotNodeTag || !ok {
		return nil, nil
	}
	var snapshot waServerSync.ExternalBlobReference
	err := proto.Unmarshal(rawSnapshot, &snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	var rawData []byte
	rawData, err = downloadExternal(ctx, &snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to download external mutations: %w", err)
	}
	var downloaded waServerSync.SyncdSnapshot
	err = proto.Unmarshal(rawData, &downloaded)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal mutation list: %w", err)
	}
	return &downloaded, nil
}

func parsePatchListInternal(ctx context.Context, collection *waBinary.Node, downloadExternal DownloadExternalFunc) ([]*waServerSync.SyncdPatch, error) {
	patchesNode := collection.GetChildByTag(patchesNodeTag)
	patchNodes := patchesNode.GetChildren()
	patches := make([]*waServerSync.SyncdPatch, 0, len(patchNodes))
	for i, patchNode := range patchNodes {
		rawPatch, ok := patchNode.Content.([]byte)
		if patchNode.Tag != patchNodeTag || !ok {
			continue
		}
		var patch waServerSync.SyncdPatch
		err := proto.Unmarshal(rawPatch, &patch)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal patch #%d: %w", i+1, err)
		}
		if patch.GetExternalMutations() != nil && downloadExternal != nil {
			var rawData []byte
			rawData, err = downloadExternal(ctx, patch.GetExternalMutations())
			if err != nil {
				return nil, fmt.Errorf("failed to download external mutations: %w", err)
			}
			var downloaded waServerSync.SyncdMutations
			err = proto.Unmarshal(rawData, &downloaded)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal mutation list: %w", err)
			} else if len(downloaded.GetMutations()) == 0 {
				return nil, fmt.Errorf("didn't get any mutations from download")
			}
			patch.Mutations = downloaded.Mutations
		}
		patches = append(patches, &patch)
	}
	return patches, nil
}

// ParsePatchList will decode an XML node containing app state patches, including downloading any external blobs.
func ParsePatchList(ctx context.Context, collection *waBinary.Node, downloadExternal DownloadExternalFunc) (*PatchList, error) {
	ag := collection.AttrGetter()
	snapshot, err := parseSnapshotInternal(ctx, collection, downloadExternal)
	if err != nil {
		return nil, err
	}
	patches, err := parsePatchListInternal(ctx, collection, downloadExternal)
	if err != nil {
		return nil, err
	}
	list := &PatchList{
		Name:           WAPatchName(ag.String(patchListAttrName)),
		HasMorePatches: ag.OptionalBool(patchListAttrHasMore),
		Patches:        patches,
		Snapshot:       snapshot,
	}
	return list, ag.Error()
}
