package capabilityregistry

import "wa-api/pkg/domain"

// PortMethod identifies one method of one application port interface, in
// pkg/application/contracts, e.g. {"TextMessenger", "SendText"}.
type PortMethod struct {
	Interface string
	Method    string
}

// PortCoverage maps every port method this worktree treats as a business
// CAPABILITY to its domain.Capability identifier. It exists so that
// coverage_gate_test.go can fail loudly when a new provider-dependent port
// method appears in pkg/application/contracts without a capability mapping
// — see that test's doc comment for what "provider-dependent" means and why
// it is the enumeration boundary.
//
// This map is the single source of truth for "which capability does this
// port method represent." pkg/capabilityregistry/matrix.go's rows use the
// domain.Capability values from here, but do not themselves re-derive the
// port→capability mapping — that traceability lives only here.
var PortCoverage = map[PortMethod]domain.Capability{
	{"TextMessenger", "SendText"}: domain.CapSendText,

	{"SimpleMessenger", "SendLocation"}: domain.CapSendLocation,
	{"SimpleMessenger", "SendContact"}:  domain.CapSendContact,
	{"SimpleMessenger", "SendPoll"}:     domain.CapSendPoll,
	{"SimpleMessenger", "SendTemplate"}: domain.CapSendTemplate,
	{"SimpleMessenger", "SendList"}:     domain.CapSendList,

	{"MediaMessenger", "SendImage"}:    domain.CapSendImage,
	{"MediaMessenger", "SendDocument"}: domain.CapSendDocument,
	{"MediaMessenger", "SendAudio"}:    domain.CapSendAudio,
	{"MediaMessenger", "SendVideo"}:    domain.CapSendVideo,
	{"MediaMessenger", "SendSticker"}:  domain.CapSendSticker,

	{"InteractiveMessenger", "SendButtons"}:  domain.CapSendButtons,
	{"InteractiveMessenger", "SendCarousel"}: domain.CapSendCarousel,

	{"ForwardedMessageSender", "SendForwardedMessage"}: domain.CapForwardMessage,

	{"ChatMessenger", "MarkRead"}:      domain.CapMarkRead,
	{"ChatMessenger", "SendReaction"}:  domain.CapSendReaction,
	{"ChatMessenger", "RevokeMessage"}: domain.CapRevokeMessage,
	{"ChatMessenger", "EditMessage"}:   domain.CapEditMessage,
	{"ChatMessenger", "SendPollVote"}:  domain.CapVotePoll,

	{"MessageStarrer", "StarMessage"}: domain.CapStarMessage,

	{"PresenceAnnouncer", "SendPresence"}:       domain.CapSendPresence,
	{"PresenceAnnouncer", "SendChatPresence"}:   domain.CapSendChatPresence,
	{"PresenceSubscriber", "SubscribePresence"}: domain.CapSubscribePresence,

	{"ChatArchiver", "ArchiveChat"}:                                   domain.CapArchiveChat,
	{"ChatMuter", "MuteChat"}:                                         domain.CapMuteChat,
	{"ChatPinner", "PinChat"}:                                         domain.CapPinChat,
	{"CallRejecter", "RejectCall"}:                                    domain.CapRejectCall,
	{"UnavailableMessageRequester", "RequestUnavailableMessage"}:      domain.CapRequestUnavailableMessage,
	{"UnavailableMessageRequester", "SetDisappearingTimer"}:           domain.CapSetDisappearingTimerChat,
	{"DefaultDisappearingTimerSetter", "SetDefaultDisappearingTimer"}: domain.CapSetDefaultDisappearingTimer,
	{"StatusMessageSetter", "SetStatusMessage"}:                       domain.CapSetStatusMessage,
	{"HistorySyncRequester", "RequestHistorySync"}:                    domain.CapRequestHistorySync,
	{"AppStateSyncer", "SyncContactRoster"}:                           domain.CapSyncContactRoster,
	{"ProfileAccessProvider", "ProfileAccess"}:                        domain.CapAccessProfileData,

	{"MediaDownloader", "Download"}: domain.CapDownloadMedia,

	{"NewsletterReader", "ListSubscribed"}:                 domain.CapListSubscribedNewsletters,
	{"NewsletterReader", "CreateNewsletter"}:               domain.CapCreateNewsletter,
	{"NewsletterReader", "NewsletterInfo"}:                 domain.CapGetNewsletterInfo,
	{"NewsletterReader", "NewsletterInfoWithInvite"}:       domain.CapGetNewsletterInfoByInvite,
	{"NewsletterReader", "FollowNewsletter"}:               domain.CapFollowNewsletter,
	{"NewsletterReader", "UnfollowNewsletter"}:             domain.CapUnfollowNewsletter,
	{"NewsletterReader", "ToggleNewsletterMute"}:           domain.CapMuteNewsletter,
	{"NewsletterReader", "NewsletterMessages"}:             domain.CapListNewsletterMessages,
	{"NewsletterReader", "NewsletterMessageUpdates"}:       domain.CapListNewsletterMessageUpdates,
	{"NewsletterReader", "MarkNewsletterViewed"}:           domain.CapMarkNewsletterViewed,
	{"NewsletterReader", "SendNewsletterReaction"}:         domain.CapReactToNewsletterMessage,
	{"NewsletterReader", "SubscribeNewsletterLiveUpdates"}: domain.CapSubscribeNewsletterLiveUpdates,
	{"NewsletterReader", "DemoteNewsletterAdmin"}:          domain.CapDemoteNewsletterAdmin,
	{"NewsletterReader", "ChangeNewsletterOwner"}:          domain.CapChangeNewsletterOwner,
	{"NewsletterReader", "DeleteNewsletter"}:               domain.CapDeleteNewsletter,
	{"NewsletterReader", "CreateNewsletterAdminInvite"}:    domain.CapInviteNewsletterAdmin,
	{"NewsletterReader", "AcceptNewsletterAdminInvite"}:    domain.CapAcceptNewsletterAdminInvite,
	{"NewsletterReader", "RevokeNewsletterAdminInvite"}:    domain.CapRevokeNewsletterAdminInvite,

	{"GroupDirectory", "GetGroupInfo"}:         domain.CapGetGroupInfo,
	{"GroupDirectory", "GetGroupInfoFromLink"}: domain.CapGetGroupInfoFromInviteLink,
	{"GroupDirectory", "GetGroupInviteLink"}:   domain.CapGetGroupInviteLink,
	{"GroupDirectory", "GroupNames"}:           domain.CapListGroupNames,
	{"GroupDirectory", "ListJoinedGroups"}:     domain.CapListJoinedGroups,

	{"GroupLifecycle", "CreateGroup"}: domain.CapCreateGroup,
	{"GroupLifecycle", "JoinGroup"}:   domain.CapJoinGroupViaInvite,
	{"GroupLifecycle", "LeaveGroup"}:  domain.CapLeaveGroup,

	{"GroupInfoSettings", "SetGroupName"}:     domain.CapSetGroupName,
	{"GroupInfoSettings", "SetGroupTopic"}:    domain.CapSetGroupTopic,
	{"GroupInfoSettings", "SetGroupAnnounce"}: domain.CapSetGroupAnnounceMode,
	{"GroupInfoSettings", "SetGroupLocked"}:   domain.CapSetGroupLocked,

	{"GroupParticipants", "UpdateGroupParticipants"}: domain.CapUpdateGroupParticipants,
	{"GroupPhotoSetter", "SetGroupPhoto"}:            domain.CapSetGroupPhoto,
	{"GroupEphemeralSetter", "SetDisappearingTimer"}: domain.CapSetGroupEphemeralTimer,

	{"CommunityDirectory", "GetSubGroups"}:                domain.CapGetSubGroups,
	{"CommunityDirectory", "GetLinkedGroupsParticipants"}: domain.CapGetLinkedGroupsParticipants,
	{"CommunityLifecycle", "LinkGroup"}:                   domain.CapLinkGroupToCommunity,
	{"CommunityLifecycle", "UnlinkGroup"}:                 domain.CapUnlinkGroupFromCommunity,

	{"GroupRequests", "GetRequestParticipants"}:    domain.CapListGroupJoinRequests,
	{"GroupRequests", "UpdateRequestParticipants"}: domain.CapUpdateGroupJoinRequests,
	{"GroupRequests", "SetJoinApprovalMode"}:       domain.CapSetGroupJoinApprovalMode,

	{"IdentityResolver", "IsOnWhatsApp"}:      domain.CapCheckIsOnWhatsApp,
	{"IdentityResolver", "GetLIDForPN"}:       domain.CapResolveLIDForPN,
	{"IdentityResolver", "GetPNForLID"}:       domain.CapResolvePNForLID,
	{"IdentityResolver", "GetManyLIDsForPNs"}: domain.CapResolveManyLIDs,

	{"AvatarReader", "GetProfilePicture"}: domain.CapGetProfilePicture,

	{"ContactRoster", "GetAllContacts"}: domain.CapListContacts,
	{"ContactRoster", "ContactNames"}:   domain.CapListContactNames,
	{"ContactRoster", "GetUserInfo"}:    domain.CapGetUserInfo,

	{"BlocklistManager", "GetBlocklist"}:    domain.CapGetBlocklist,
	{"BlocklistManager", "UpdateBlocklist"}: domain.CapUpdateBlocklist,

	{"PrivacyManager", "GetPrivacySettings"}: domain.CapGetPrivacySettings,
	{"PrivacyManager", "SetPrivacySetting"}:  domain.CapSetPrivacySetting,

	{"PhonePairer", "IsPaired"}:           domain.CapCheckPairingStatus,
	{"PhonePairer", "RequestPairingCode"}: domain.CapRequestPairingCode,

	{"SessionDisconnector", "Disconnect"}: domain.CapDisconnectSession,
	{"SessionLogouter", "Logout"}:         domain.CapLogoutSession,

	{"AccountTypeDetector", "Detect"}: domain.CapDetectAccountType,
}

// EngineAgnosticPorts lists provider-dependent port methods that are
// DELIBERATELY exempt from PortCoverage because they are not, themselves, a
// distinct business capability (e.g. a pure passthrough with no engine- or
// account-type-conditioned behavior). Empty today: every SessionGuard-
// embedding port method found so far maps to a capability. Kept as a named,
// checked escape hatch instead of silently skipping unmapped methods, so a
// future exemption is a deliberate, visible decision rather than a gate
// that quietly stopped checking something.
var EngineAgnosticPorts = map[PortMethod]bool{}
