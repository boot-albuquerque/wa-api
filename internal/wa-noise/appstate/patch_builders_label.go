package appstate

import (
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/types"
)

func newLabelChatMutation(target types.JID, labelID string, labeled bool) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexLabelAssociationChat, labelID, target.String()},
		Version: mutationVersionLabelAssocChat,
		Value: &waSyncAction.SyncActionValue{
			LabelAssociationAction: &waSyncAction.LabelAssociationAction{
				Labeled: &labeled,
			},
		},
	}
}

// BuildLabelChat builds an app state patch for labeling or un(labeling) a chat.
func BuildLabelChat(target types.JID, labelID string, labeled bool) PatchInfo {
	return PatchInfo{
		Type: WAPatchRegular,
		Mutations: []MutationInfo{
			newLabelChatMutation(target, labelID, labeled),
		},
	}
}

func newLabelMessageMutation(target types.JID, labelID, messageID string, labeled bool) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexLabelAssociationMessage, labelID, target.String(), messageID, indexBoolFalse, indexBoolFalse},
		Version: mutationVersionLabelAssocMsg,
		Value: &waSyncAction.SyncActionValue{
			LabelAssociationAction: &waSyncAction.LabelAssociationAction{
				Labeled: &labeled,
			},
		},
	}
}

// BuildLabelMessage builds an app state patch for labeling or un(labeling) a message.
func BuildLabelMessage(target types.JID, labelID, messageID string, labeled bool) PatchInfo {
	return PatchInfo{
		Type: WAPatchRegular,
		Mutations: []MutationInfo{
			newLabelMessageMutation(target, labelID, messageID, labeled),
		},
	}
}

func newLabelEditMutation(labelID string, labelName string, labelColor int32, deleted bool) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexLabelEdit, labelID},
		Version: mutationVersionLabelEdit,
		Value: &waSyncAction.SyncActionValue{
			LabelEditAction: &waSyncAction.LabelEditAction{
				Name:    &labelName,
				Color:   &labelColor,
				Deleted: &deleted,
			},
		},
	}
}

// BuildLabelEdit builds an app state patch for editing a label.
func BuildLabelEdit(labelID string, labelName string, labelColor int32, deleted bool) PatchInfo {
	return PatchInfo{
		Type: WAPatchRegular,
		Mutations: []MutationInfo{
			newLabelEditMutation(labelID, labelName, labelColor, deleted),
		},
	}
}
