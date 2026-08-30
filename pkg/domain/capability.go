package domain

// Capability names a WHATSAPP BUSINESS OPERATION, independent of which HTTP
// route or application port implements it.
//
// # Why a separate identifier from port/endpoint names
//
// A port method (e.g. TextMessenger.SendText) is an INFRASTRUCTURE SEAM,
// shaped by Go interface ergonomics and by transport differences (decisions
// 82/87/92 split ports by what a transport can serve, not by business
// meaning). A capability is the opposite axis: it names what a human asked
// for ("send a text message"), and it is stable even if the port that serves
// it is later split, merged, or renamed.
//
// Today every capability maps to exactly one port method — there is no
// measured case of two endpoints collapsing into one business capability yet
// (see docs/CAPABILITIES.md). Inventing such a collapse without evidence
// would hide a real difference behind a false sameness, so this file keeps
// the 1:1 mapping until a concrete case demands otherwise.
type Capability string

// String makes Capability printable without a conversion at every call site.
func (c Capability) String() string { return string(c) }

// Messaging capabilities — sending new messages.
const (
	CapSendText       Capability = "send_text"
	CapSendLocation   Capability = "send_location"
	CapSendContact    Capability = "send_contact"
	CapSendPoll       Capability = "send_poll"
	CapSendTemplate   Capability = "send_template"
	CapSendList       Capability = "send_list"
	CapSendImage      Capability = "send_image"
	CapSendDocument   Capability = "send_document"
	CapSendAudio      Capability = "send_audio"
	CapSendVideo      Capability = "send_video"
	CapSendSticker    Capability = "send_sticker"
	CapSendButtons    Capability = "send_buttons"
	CapSendCarousel   Capability = "send_carousel"
	CapForwardMessage Capability = "forward_message"
)

// Capabilities on messages that already exist in a chat.
const (
	CapMarkRead      Capability = "mark_read"
	CapSendReaction  Capability = "send_reaction"
	CapRevokeMessage Capability = "revoke_message"
	CapEditMessage   Capability = "edit_message"
	CapVotePoll      Capability = "vote_poll"
	CapStarMessage   Capability = "star_message"
)

// Presence capabilities.
const (
	CapSendPresence      Capability = "send_presence"
	CapSendChatPresence  Capability = "send_chat_presence"
	CapSubscribePresence Capability = "subscribe_presence"
)

// Chat-level operations.
const (
	CapArchiveChat                 Capability = "archive_chat"
	CapMuteChat                    Capability = "mute_chat"
	CapPinChat                     Capability = "pin_chat"
	CapRejectCall                  Capability = "reject_call"
	CapRequestUnavailableMessage   Capability = "request_unavailable_message"
	CapSetDisappearingTimerChat    Capability = "set_disappearing_timer_chat"
	CapSetDefaultDisappearingTimer Capability = "set_default_disappearing_timer"
	CapSetStatusMessage            Capability = "set_status_message"
	CapRequestHistorySync          Capability = "request_history_sync"
	CapSyncContactRoster           Capability = "sync_contact_roster"
	CapAccessProfileData           Capability = "access_profile_data"
)

// Media.
const (
	CapDownloadMedia Capability = "download_media"
)

// Newsletter (channel) capabilities.
const (
	CapListSubscribedNewsletters      Capability = "list_subscribed_newsletters"
	CapCreateNewsletter               Capability = "create_newsletter"
	CapGetNewsletterInfo              Capability = "get_newsletter_info"
	CapGetNewsletterInfoByInvite      Capability = "get_newsletter_info_by_invite"
	CapFollowNewsletter               Capability = "follow_newsletter"
	CapUnfollowNewsletter             Capability = "unfollow_newsletter"
	CapMuteNewsletter                 Capability = "mute_newsletter"
	CapListNewsletterMessages         Capability = "list_newsletter_messages"
	CapListNewsletterMessageUpdates   Capability = "list_newsletter_message_updates"
	CapMarkNewsletterViewed           Capability = "mark_newsletter_viewed"
	CapReactToNewsletterMessage       Capability = "react_to_newsletter_message"
	CapSubscribeNewsletterLiveUpdates Capability = "subscribe_newsletter_live_updates"
	CapDemoteNewsletterAdmin          Capability = "demote_newsletter_admin"
	CapChangeNewsletterOwner          Capability = "change_newsletter_owner"
	CapDeleteNewsletter               Capability = "delete_newsletter"
	CapInviteNewsletterAdmin          Capability = "invite_newsletter_admin"
	CapAcceptNewsletterAdminInvite    Capability = "accept_newsletter_admin_invite"
	CapRevokeNewsletterAdminInvite    Capability = "revoke_newsletter_admin_invite"
)

// Group capabilities.
const (
	CapGetGroupInfo                Capability = "get_group_info"
	CapGetGroupInfoFromInviteLink  Capability = "get_group_info_from_invite_link"
	CapGetGroupInviteLink          Capability = "get_group_invite_link"
	CapListGroupNames              Capability = "list_group_names"
	CapListJoinedGroups            Capability = "list_joined_groups"
	CapCreateGroup                 Capability = "create_group"
	CapJoinGroupViaInvite          Capability = "join_group_via_invite"
	CapLeaveGroup                  Capability = "leave_group"
	CapSetGroupName                Capability = "set_group_name"
	CapSetGroupTopic               Capability = "set_group_topic"
	CapSetGroupAnnounceMode        Capability = "set_group_announce_mode"
	CapSetGroupLocked              Capability = "set_group_locked"
	CapUpdateGroupParticipants     Capability = "update_group_participants"
	CapSetGroupPhoto               Capability = "set_group_photo"
	CapSetGroupEphemeralTimer      Capability = "set_group_ephemeral_timer"
	CapGetSubGroups                Capability = "get_subgroups"
	CapGetLinkedGroupsParticipants Capability = "get_linked_groups_participants"
	CapLinkGroupToCommunity        Capability = "link_group_to_community"
	CapUnlinkGroupFromCommunity    Capability = "unlink_group_from_community"
	CapListGroupJoinRequests       Capability = "list_group_join_requests"
	CapUpdateGroupJoinRequests     Capability = "update_group_join_requests"
	CapSetGroupJoinApprovalMode    Capability = "set_group_join_approval_mode"
)

// User / contact capabilities.
const (
	CapCheckIsOnWhatsApp  Capability = "check_is_on_whatsapp"
	CapResolveLIDForPN    Capability = "resolve_lid_for_pn"
	CapResolvePNForLID    Capability = "resolve_pn_for_lid"
	CapResolveManyLIDs    Capability = "resolve_many_lids_for_pns"
	CapGetProfilePicture  Capability = "get_profile_picture"
	CapListContacts       Capability = "list_contacts"
	CapListContactNames   Capability = "list_contact_names"
	CapGetUserInfo        Capability = "get_user_info"
	CapGetBlocklist       Capability = "get_blocklist"
	CapUpdateBlocklist    Capability = "update_blocklist"
	CapGetPrivacySettings Capability = "get_privacy_settings"
	CapSetPrivacySetting  Capability = "set_privacy_setting"
)

// Pairing / session lifecycle capabilities.
const (
	CapCheckPairingStatus Capability = "check_pairing_status"
	CapRequestPairingCode Capability = "request_pairing_code"
	CapDisconnectSession  Capability = "disconnect_session"
	CapLogoutSession      Capability = "logout_session"

	// CapGetPairingQR is offering the QR code a human points a phone at.
	//
	// It is engine-conditioned and not a plain database read, even though the
	// code itself is stored in users.qrcode: the column only ever has a value
	// because ONE engine's lifecycle writes it there. An engine with no such
	// writer serves an empty string forever, which is why the read lives
	// behind PairingQRReader instead of behind the user repository.
	CapGetPairingQR Capability = "get_pairing_qr"

	// CapConnectSession is starting the transport a pairing flow needs.
	//
	// Unlike every other entry in this block it does NOT presuppose a live
	// session — it is what produces one — so its port deliberately does not
	// embed SessionGuard. See port.SessionStarter.
	CapConnectSession Capability = "connect_session"

	// CapDetectAccountType asks the transport whether the session's own
	// account is personal or Business (worktree feature/account-type-
	// detection, items 33-35). It is engine-conditioned like every other
	// entry in this block — noise and headless each read a different
	// real signal — which is exactly the coverage_gate_test.go boundary for
	// belonging in this registry, even though it has no HTTP route today.
	CapDetectAccountType Capability = "detect_account_type"
)

// AccountType names the kind of WhatsApp account a session runs as.
//
// It is a plain domain enum with NO detection logic behind it — detecting the
// real account type from noise/headless data is explicitly out of scope
// for the capability-registry worktree (it depends on data this worktree does
// not have access to). Callers that know the account type pass it in; callers
// that do not pass AccountTypeUnknown, which the registry treats as
// "propagate the uncertainty," never as "assume personal."
type AccountType string

const (
	// AccountTypePersonal is a regular consumer WhatsApp account.
	AccountTypePersonal AccountType = "personal"

	// AccountTypeBusiness is a WhatsApp Business (or Business API/WABA-linked)
	// account. Some capabilities behave differently or are unavailable on
	// this account type (e.g. catalog/products live only under a business
	// account with Cloud API/WABA, which this project's engines do not
	// reach at all — see docs/CAPACIDADES.md).
	AccountTypeBusiness AccountType = "business"

	// AccountTypeUnknown means the caller could not determine the account
	// type. It is a legitimate steady state, not an error: it is the honest
	// answer until account-type detection exists (future worktree).
	AccountTypeUnknown AccountType = "unknown"
)

// String makes AccountType printable without a conversion at every call site.
func (a AccountType) String() string { return string(a) }

// IsKnown reports whether this is a concrete account type, as opposed to the
// "we do not know" placeholder.
func (a AccountType) IsKnown() bool {
	return a == AccountTypePersonal || a == AccountTypeBusiness
}
