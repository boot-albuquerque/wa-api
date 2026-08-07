package appstate

import (
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/protocol/types"
)

func newSettingPushNameMutation(pushName string) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexSettingPushName},
		Version: mutationVersionSettingPushName,
		Value: &waSyncAction.SyncActionValue{
			PushNameSetting: &waSyncAction.PushNameSetting{
				Name: &pushName,
			},
		},
	}
}

// BuildSettingPushName builds an app state patch for setting the push name.
func BuildSettingPushName(pushName string) PatchInfo {
	return PatchInfo{
		Type: WAPatchCriticalBlock,
		Mutations: []MutationInfo{
			newSettingPushNameMutation(pushName),
		},
	}
}

func newStarMutation(targetJID, senderJID string, messageID types.MessageID, fromMe string, starred bool) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexStar, targetJID, messageID, fromMe, senderJID},
		Version: mutationVersionStar,
		Value: &waSyncAction.SyncActionValue{
			StarAction: &waSyncAction.StarAction{
				Starred: &starred,
			},
		},
	}
}

// BuildStar builds an app state patch for starring or unstarring a message.
func BuildStar(target, sender types.JID, messageID types.MessageID, fromMe, starred bool) PatchInfo {
	isFromMe := indexBoolFalse
	if fromMe {
		isFromMe = indexBoolTrue
	}
	targetJID, senderJID := target.String(), sender.String()
	if target.User == sender.User {
		senderJID = selfSenderIndexValue
	}
	return PatchInfo{
		Type: WAPatchRegularHigh,
		Mutations: []MutationInfo{
			newStarMutation(targetJID, senderJID, messageID, isFromMe, starred),
		},
	}
}
