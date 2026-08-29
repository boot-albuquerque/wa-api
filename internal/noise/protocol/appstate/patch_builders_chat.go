package appstate

import (
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waCommon"
	"wa-api/internal/noise/protocol/proto/waSyncAction"
	"wa-api/internal/noise/protocol/types"
)

// BuildMute builds an app state patch for muting or unmuting a chat.
//
// If mute is true and the mute duration is zero, the chat is muted forever.
func BuildMute(target types.JID, mute bool, muteDuration time.Duration) PatchInfo {
	var muteEndTimestamp *int64
	if muteDuration > 0 {
		muteEndTimestamp = proto.Int64(time.Now().Add(muteDuration).UnixMilli())
	}
	return BuildMuteAbs(target, mute, muteEndTimestamp)
}

// BuildMuteAbs builds an app state patch for muting or unmuting a chat with an absolute timestamp.
func BuildMuteAbs(target types.JID, mute bool, muteEndTimestamp *int64) PatchInfo {
	if muteEndTimestamp == nil && mute {
		muteEndTimestamp = proto.Int64(muteForeverEndTimestamp)
	}
	return PatchInfo{
		Type: WAPatchRegularHigh,
		Mutations: []MutationInfo{{
			Index:   []string{IndexMute, target.String()},
			Version: mutationVersionMute,
			Value: &waSyncAction.SyncActionValue{
				MuteAction: &waSyncAction.MuteAction{
					Muted:            proto.Bool(mute),
					MuteEndTimestamp: muteEndTimestamp,
				},
			},
		}},
	}
}

func newPinMutationInfo(target types.JID, pin bool) MutationInfo {
	return MutationInfo{
		Index:   []string{IndexPin, target.String()},
		Version: mutationVersionPin,
		Value: &waSyncAction.SyncActionValue{
			PinAction: &waSyncAction.PinAction{
				Pinned: &pin,
			},
		},
	}
}

// BuildPin builds an app state patch for pinning or unpinning a chat.
func BuildPin(target types.JID, pin bool) PatchInfo {
	return PatchInfo{
		Type: WAPatchRegularLow,
		Mutations: []MutationInfo{
			newPinMutationInfo(target, pin),
		},
	}
}

// BuildArchive builds an app state patch for archiving or unarchiving a chat.
//
// The last message timestamp and last message key are optional and can be set to zero values (`time.Time{}` and `nil`).
//
// Archiving a chat will also unpin it automatically.
func BuildArchive(target types.JID, archive bool, lastMessageTimestamp time.Time, lastMessageKey *waCommon.MessageKey) PatchInfo {
	archiveMutationInfo := MutationInfo{
		Index:   []string{IndexArchive, target.String()},
		Version: mutationVersionArchive,
		Value: &waSyncAction.SyncActionValue{
			ArchiveChatAction: &waSyncAction.ArchiveChatAction{
				Archived:     &archive,
				MessageRange: newMessageRange(lastMessageTimestamp, lastMessageKey),
				// TODO set LastSystemMessageTimestamp?
			},
		},
	}

	mutations := []MutationInfo{archiveMutationInfo}
	if archive {
		mutations = append(mutations, newPinMutationInfo(target, false))
	}

	result := PatchInfo{
		Type:      WAPatchRegularLow,
		Mutations: mutations,
	}

	return result
}

// BuildMarkChatAsRead builds an app state patch for marking a chat as read or unread.
func BuildMarkChatAsRead(target types.JID, read bool, lastMessageTimestamp time.Time, lastMessageKey *waCommon.MessageKey) PatchInfo {
	action := &waSyncAction.MarkChatAsReadAction{
		Read:         proto.Bool(read),
		MessageRange: newMessageRange(lastMessageTimestamp, lastMessageKey),
	}

	return PatchInfo{
		Type: WAPatchRegularLow,
		Mutations: []MutationInfo{{
			Index:   []string{IndexMarkChatAsRead, target.String()},
			Version: mutationVersionMarkChatAsRead,
			Value: &waSyncAction.SyncActionValue{
				MarkChatAsReadAction: action,
			},
		}},
	}
}

// BuildDeleteChat builds an app state patch for deleting a chat.
func BuildDeleteChat(target types.JID, lastMessageTimestamp time.Time, lastMessageKey *waCommon.MessageKey, deleteMedia bool) PatchInfo {
	action := &waSyncAction.DeleteChatAction{
		MessageRange: newMessageRange(lastMessageTimestamp, lastMessageKey),
	}
	deleteMediaInt := indexBoolFalse
	if deleteMedia {
		deleteMediaInt = indexBoolTrue
	}

	return PatchInfo{
		Type: WAPatchRegularHigh,
		Mutations: []MutationInfo{{
			Index:   []string{IndexDeleteChat, target.String(), deleteMediaInt},
			Version: mutationVersionDeleteChat,
			Value: &waSyncAction.SyncActionValue{
				DeleteChatAction: action,
			},
		}},
	}
}

func newMessageRange(lastMessageTimestamp time.Time, lastMessageKey *waCommon.MessageKey) *waSyncAction.SyncActionMessageRange {
	if lastMessageTimestamp.IsZero() {
		lastMessageTimestamp = time.Now()
	}
	messageRange := &waSyncAction.SyncActionMessageRange{
		LastMessageTimestamp: proto.Int64(lastMessageTimestamp.Unix()),
	}
	if lastMessageKey != nil {
		messageRange.Messages = []*waSyncAction.SyncActionMessage{{
			Key:       lastMessageKey,
			Timestamp: proto.Int64(lastMessageTimestamp.Unix()),
		}}
	}
	return messageRange
}
