package events

import (
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// JoinedGroup is emitted when you join or are added to a group.
type JoinedGroup struct {
	Reason    string          // If the event was triggered by you using an invite link, this will be "invite".
	Type      string          // "new" if it's a newly created group.
	CreateKey types.MessageID // If you created the group, this is the same message ID you passed to CreateGroup.
	// For type new, the user who created the group and added you to it
	Sender   *types.JID
	SenderPN *types.JID
	Notify   string

	types.GroupInfo
}

// GroupInfo is emitted when the metadata of a group changes.
type GroupInfo struct {
	JID       types.JID  // The group ID in question
	Notify    string     // Seems like a top-level type for the invite
	Sender    *types.JID // The user who made the change. Doesn't seem to be present when notify=invite
	SenderPN  *types.JID // The phone number of the user who made the change, if Sender is a LID.
	Timestamp time.Time  // The time when the change occurred

	Name      *types.GroupName      // Group name change
	Topic     *types.GroupTopic     // Group topic (description) change
	Locked    *types.GroupLocked    // Group locked status change (can only admins edit group info?)
	Announce  *types.GroupAnnounce  // Group announce status change (can only admins send messages?)
	Ephemeral *types.GroupEphemeral // Disappearing messages change

	MembershipApprovalMode *types.GroupMembershipApprovalMode // Membership approval mode change

	Delete *types.GroupDelete

	Link   *types.GroupLinkChange
	Unlink *types.GroupLinkChange

	NewInviteLink *string // Group invite link change

	PrevParticipantVersionID string
	ParticipantVersionID     string

	JoinReason string // This will be "invite" if the user joined via invite link

	Join  []types.JID // Users who joined or were added the group
	Leave []types.JID // Users who left or were removed from the group

	Promote        []types.JID // Users who were promoted to admins
	Demote         []types.JID // Users who were demoted to normal users
	Suspended      bool        // whether the group is suspended
	Unsuspended    bool        // whether the group is unsuspended
	UnknownChanges []*waBinary.Node
}
