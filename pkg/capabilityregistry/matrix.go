package capabilityregistry

import (
	"fmt"

	"wa-api/pkg/domain"

	"github.com/rs/zerolog/log"
)

// cell is one square of the Engine × AccountType matrix for a single
// capability: what the status is, and how confident that status is.
type cell struct {
	status   domain.CapabilityStatus
	evidence domain.EvidenceStatus
	note     string
}

// key identifies one cell.
type key struct {
	capability domain.Capability
	engine     domain.Engine
	account    domain.AccountType
}

// matrix is the static, hand-verified Engine × AccountType table. It is
// built once (NewDefaultMatrix) from source-reading evidence, not
// recomputed at request time — see the package doc comment on why the
// registry never re-derives policy.
type matrix struct {
	cells map[key]cell
	// byEngine indexes which capabilities have ANY entry for a given engine,
	// for CapabilityProvider.Capabilities.
	byEngine map[domain.Engine]map[domain.Capability]bool
}

func newMatrix() *matrix {
	return &matrix{
		cells: make(map[key]cell),
		// Pre-seeded with both known engines so set() never needs a
		// lazy-init branch — see set()'s own comment on why that keeps it
		// trivial by construction, not by hiding a branch.
		byEngine: map[domain.Engine]map[domain.Capability]bool{
			domain.EngineNoise:    make(map[domain.Capability]bool),
			domain.EngineHeadless: make(map[domain.Capability]bool),
		},
	}
}

// set records one cell. It is intentionally two statements and no
// branches: byEngine is pre-seeded for both known engines in newMatrix, so
// there is nothing here to initialize lazily.
func (m *matrix) set(capability domain.Capability, engine domain.Engine, account domain.AccountType, c cell) {
	m.cells[key{capability, engine, account}] = c
	m.byEngine[engine][capability] = true
}

func (m *matrix) capabilitiesForEngine(engine domain.Engine) []domain.Capability {
	out := make([]domain.Capability, 0, len(m.byEngine[engine]))
	for c := range m.byEngine[engine] {
		out = append(out, c)
	}
	log.Debug().Str("engine", engine.String()).Int("count", len(out)).
		Msg("capabilityregistry: listed capabilities for engine")
	return out
}

// decide looks up one cell and turns it into a CapabilityDecision. A missing
// cell — a (capability, engine, account_type) nobody ever recorded — is
// PROPAGATED as unknown, never silently treated as unsupported or supported:
// the whole point of a registry with static evidence is that absence of
// evidence is visible, not swallowed.
func (m *matrix) decide(capability domain.Capability, engine domain.Engine, account domain.AccountType) CapabilityDecision {
	c, ok := m.cells[key{capability, engine, account}]
	if !ok {
		log.Warn().Str("capability", capability.String()).Str("engine", engine.String()).
			Str("account_type", account.String()).
			Msg("capabilityregistry: no matrix entry — propagating unknown instead of assuming support")
		return CapabilityDecision{
			Capability:  capability,
			Supported:   false,
			Status:      domain.StatusUnknown,
			Reason:      fmt.Sprintf("no matrix entry for capability %q on engine %q, account type %q", capability, engine, account),
			Engine:      engine,
			AccountType: account,
			Evidence:    domain.EvidenceUnknown,
		}
	}
	return CapabilityDecision{
		Capability:  capability,
		Supported:   c.status.IsSupported(),
		Status:      c.status,
		Reason:      c.note,
		Engine:      engine,
		AccountType: account,
		Evidence:    c.evidence,
	}
}

// expand records the same (status, evidence, note) for a capability on one
// engine across all three domain.AccountType values, EXCEPT that
// personal/business get their evidence downgraded relative to
// accountTypeUnknown's evidence:
//
//   - if the unknown-account-type evidence is domain.EvidenceUnknown, all
//     three stay EvidenceUnknown — there is nothing account-type-specific to
//     downgrade from.
//   - otherwise personal/business get domain.EvidenceNotTested: the engine
//     dimension was checked by reading adapter code, but account-type
//     detection does not exist yet (see domain.AccountType doc comment), so
//     nothing has ever exercised this capability against a KNOWN account
//     type specifically. Marking personal/business as EvidenceNotTested
//     rather than copying the unknown-type evidence keeps that gap visible
//     instead of borrowing confidence from a dimension that was never
//     evaluated for account type.
func (m *matrix) expand(capability domain.Capability, engine domain.Engine, status domain.CapabilityStatus, evidence domain.EvidenceStatus, note string) {
	m.set(capability, engine, domain.AccountTypeUnknown, cell{status, evidence, note})

	perTypeEvidence := domain.EvidenceNotTested
	if evidence == domain.EvidenceUnknown {
		perTypeEvidence = domain.EvidenceUnknown
	}
	log.Debug().Str("capability", capability.String()).Str("engine", engine.String()).
		Str("status", status.String()).Str("evidence_unknown_type", evidence.String()).
		Str("evidence_known_type", perTypeEvidence.String()).
		Msg("capabilityregistry: expanded matrix row across account types")
	m.set(capability, engine, domain.AccountTypePersonal, cell{status, perTypeEvidence, note})
	m.set(capability, engine, domain.AccountTypeBusiness, cell{status, perTypeEvidence, note})
}

// row is one line of the hand-verified table in NewDefaultMatrix: a
// capability's status on each engine, with the evidence for each and a
// shared note explaining the verification (or its absence).
type row struct {
	capability domain.Capability

	noiseStatus   domain.CapabilityStatus
	noiseEvidence domain.EvidenceStatus

	headlessStatus   domain.CapabilityStatus
	headlessEvidence domain.EvidenceStatus

	note string
}

// probableAdapter is the note used for the common case: an adapter file
// exists under pkg/infra/<engine>/... with a method matching the port, and
// reading it shows real logic (not a stub), but nobody has photographed a
// device or run it against a live account — see ARMADILHAS.md #1/#2 on why a
// method existing is not proof of behavior.
const probableAdapter = "adapter method found with real logic (not a stub); not field-verified"

// absentAdapter is the note for the common negative case: grep found no
// method of this name under the engine's adapter tree. Absence here is
// deliberately reported as unknown, not engine_unsupported: for headless
// specifically, "no adapter written" and "the SPA cannot do this at all" are
// different claims, and this pass verified only the former.
const absentAdapter = "no adapter method found for this engine; ambiguous between not_implemented and engine_unsupported — nobody has checked whether the transport itself could serve it"

// NewDefaultMatrix builds the capability matrix this worktree could verify
// by reading code (grep for the port's method name under
// pkg/infra/noise/... and pkg/infra/headless/..., then reading the
// hit to confirm it is not a stub). It does NOT reflect field verification
// (a real device receiving the message) except where a row's note cites a
// specific HOUSEKEEP.md/docs/CAPACIDADES.md entry that recorded one.
//
// Every row was produced against a name from pkg/application/contracts —
// see pkg/capabilityregistry/coverage.go for the mapping from port method to
// capability, and pkg/capabilityregistry/coverage_gate_test.go for the test
// that fails if a new port method has no row here.
func NewDefaultMatrix() *matrix {
	m := newMatrix()
	rows := []row{
		// --- Messaging: new messages -------------------------------------
		// headless has no adapter under pkg/infra/headless/messenger
		// (or any sibling package) implementing message CREATION — that
		// package only covers actions on messages that already exist
		// (MarkRead/SendReaction/EditMessage/RevokeMessage/SendPollVote, see
		// its own doc comment). Every "new message" capability is therefore
		// unknown for headless, not confirmed-absent.
		{domain.CapSendText, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: pkg/infra/noise/adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendLocation, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendContact, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendPoll, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendTemplate, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendList, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendImage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendDocument, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendAudio, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendVideo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendSticker, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendButtons, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/messenger.go. headless: " + absentAdapter},
		{domain.CapSendCarousel, domain.StatusSupported, domain.EvidenceConfirmed, domain.StatusUnknown, domain.EvidenceUnknown, "noise: field-verified, HOUSEKEEP.md F216 (photographed on device). headless: " + absentAdapter},
		{domain.CapForwardMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/forward.go. headless: " + absentAdapter},

		// --- Actions on existing messages ---------------------------------
		{domain.CapMarkRead, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise: adapters/chat/messenger.go. headless: pkg/infra/headless/messenger/messenger.go (local ack only — receipt-to-sender NOT confirmed, see H160 cited in that file's doc comment)"},
		{domain.CapSendReaction, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter},
		{domain.CapRevokeMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter},
		{domain.CapEditMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter},
		{domain.CapVotePoll, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter},
		{domain.CapStarMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildStar) — NOTE this contradicts docs/CAPACIDADES.md's stale claim that StarMessage 'não existe'; that document is self-flagged desatualizado since 2026-08-24. headless: " + absentAdapter},

		// --- Presence -------------------------------------------------------
		{domain.CapSendPresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/presence)"},
		{domain.CapSendChatPresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/presence)"},
		{domain.CapSubscribePresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/presence/controller.go. headless: " + absentAdapter},

		// --- Chat-level operations -------------------------------------
		{domain.CapArchiveChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter},
		{domain.CapMuteChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildMute) — contradicts stale docs/CAPACIDADES.md claim, see CapStarMessage note. headless: " + absentAdapter},
		{domain.CapPinChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildPin). headless: " + absentAdapter},
		{domain.CapRejectCall, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapRequestUnavailableMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapSetDisappearingTimerChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapSetDefaultDisappearingTimer, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapSetStatusMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapRequestHistorySync, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapSyncContactRoster, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/roster or appstate)"},
		{domain.CapAccessProfileData, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/profile)"},

		// --- Media -----------------------------------------------------
		{domain.CapDownloadMedia, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/chat/downloader.go. headless: " + absentAdapter},

		// --- Newsletter (channel) ---------------------------------------
		{domain.CapListSubscribedNewsletters, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/newsletter)"},
		{domain.CapCreateNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/newsletter)"},
		{domain.CapGetNewsletterInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapGetNewsletterInfoByInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter + " (grep matched only NewsletterInfo, not the ByInvite variant, in noise too — recorded together with NewsletterInfo since both live in the same adapter method family; treat as same confidence)"},
		{domain.CapFollowNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapUnfollowNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapMuteNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapListNewsletterMessages, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapListNewsletterMessageUpdates, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapMarkNewsletterViewed, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapReactToNewsletterMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapSubscribeNewsletterLiveUpdates, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapDemoteNewsletterAdmin, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapChangeNewsletterOwner, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapDeleteNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapInviteNewsletterAdmin, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go (CreateNewsletterAdminInvite). headless: " + absentAdapter},
		{domain.CapAcceptNewsletterAdminInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},
		{domain.CapRevokeNewsletterAdminInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/misc/adapter.go. headless: " + absentAdapter},

		// --- Group -------------------------------------------------------
		{domain.CapGetGroupInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupdir)"},
		{domain.CapGetGroupInfoFromInviteLink, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupdir)"},
		{domain.CapGetGroupInviteLink, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupdir)"},
		{domain.CapListGroupNames, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupdir)"},
		{domain.CapListJoinedGroups, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupdir)"},
		{domain.CapCreateGroup, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/grouplife)"},
		{domain.CapJoinGroupViaInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/grouplife)"},
		{domain.CapLeaveGroup, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/grouplife)"},
		{domain.CapSetGroupName, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupset)"},
		{domain.CapSetGroupTopic, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupset)"},
		{domain.CapSetGroupAnnounceMode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupset)"},
		{domain.CapSetGroupLocked, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupset)"},
		{domain.CapUpdateGroupParticipants, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupmembers) — NOTE the port's own doc comment records that on headless the write can succeed while the acting session cannot read it back (H58/H65); Supported here means the OPERATION is served, not that every observable side effect is"},
		{domain.CapSetGroupPhoto, domain.StatusSupported, domain.EvidenceProbable, domain.StatusEngineUnsupported, domain.EvidenceConfirmed, "noise: adapters/group/adapter.go. headless: CONFIRMED absent — docs/CAPACIDADES.md H140, cited verbatim in port doc comment (pkg/application/contracts/group_ports.go GroupPhotoSetter): 'no build headless os módulos de foto da página estão ausentes'"},
		{domain.CapSetGroupEphemeralTimer, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/group/adapter.go. headless: " + absentAdapter + " — port doc comment (GroupEphemeralSetter) says explicitly this one has NOT been measured on headless, unlike GroupPhotoSetter"},
		{domain.CapGetSubGroups, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/group/adapter.go. headless: " + absentAdapter},
		{domain.CapGetLinkedGroupsParticipants, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/group/adapter.go. headless: " + absentAdapter},
		{domain.CapLinkGroupToCommunity, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/group/adapter.go. headless: " + absentAdapter},
		{domain.CapUnlinkGroupFromCommunity, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/group/adapter.go. headless: " + absentAdapter},
		{domain.CapListGroupJoinRequests, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupreq)"},
		{domain.CapUpdateGroupJoinRequests, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupreq)"},
		{domain.CapSetGroupJoinApprovalMode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/groupreq)"},

		// --- User / contacts ---------------------------------------------
		{domain.CapCheckIsOnWhatsApp, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/identity)"},
		{domain.CapResolveLIDForPN, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/identity)"},
		{domain.CapResolvePNForLID, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/identity)"},
		{domain.CapResolveManyLIDs, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/identity)"},
		{domain.CapGetProfilePicture, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/avatar)"},
		{domain.CapListContacts, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/roster)"},
		{domain.CapListContactNames, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/roster)"},
		{domain.CapGetUserInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/roster)"},
		{domain.CapGetBlocklist, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/blocklist)"},
		{domain.CapUpdateBlocklist, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/blocklist)"},
		{domain.CapGetPrivacySettings, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/user/adapter.go. headless: " + absentAdapter},
		{domain.CapSetPrivacySetting, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/user/adapter.go. headless: " + absentAdapter},

		// --- Pairing / session lifecycle ----------------------------------
		{domain.CapCheckPairingStatus, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "noise: adapters/pairing/adapter.go. headless: " + absentAdapter + " — plausible this is engine_unsupported (headless auth is QR/cookie-based, not a pairing code), but that has not been confirmed by reading the auth flow, so it stays unknown rather than asserted"},
		{domain.CapRequestPairingCode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/pairing/phonepairer.go — call sequence MEASURED against a real, unpaired session, F380: reached WhatsApp's server, got a structured IQErrorBadRequest for a fake test phone number, not a crash; a full successful pairing was not completed, no real phone available to receive the code)"},
		{domain.CapDisconnectSession, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "noise+headless: " + probableAdapter + " (pkg/infra/headless/session)"},

		// The two rows below were measured on 2026-08-27 for the
		// engine-explicit pairing work, and they are the only rows in this
		// table whose headless side is not_implemented rather than unknown.
		// The difference is evidence, not opinion: absentAdapter says "nobody
		// checked whether the transport itself could serve it", and for these
		// two somebody did.
		//
		//   - The SPA demonstrably shows a pairing QR: the headless session
		//     pool has a whole quota class for it
		//     (pkg/infra/headless/registry/registry.go, KindPairing —
		//     "a session showing a QR code, waiting for a human"), with its
		//     own deadline and its own test (registry/pairing_test.go).
		//   - The SPA demonstrably starts sessions: internal/headless/
		//     runtime plus pkg/infra/headless/sessions.go (Sessions.Acquire).
		//
		// So the transport CAN do both, and the ambiguity absentAdapter exists
		// to preserve is resolved. What is missing is the adapter that exposes
		// either one through pkg/application/contracts, and beyond that nothing
		// in pkg/bootstrap constructs ANY headless object at all: grep for
		// NewDisconnector across pkg/bootstrap returns zero non-test hits.
		// That is why the evidence is confirmed — an absence in our own tree is
		// something grep can settle — while the status is not_implemented, the
		// honest name for "we could, and we have not".
		{domain.CapGetPairingQR, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceConfirmed, "noise: pkg/infra/noise/adapters/pairing/qr.go, reading users.qrcode as written by the QR listener in pkg/bootstrap/lifecycle.go. headless (2026-08-29, HOUSEKEEP H145): pkg/infra/headless/pairing.QRReader, over core.StartPairingSession (new boot primitive, core/session.go) and internal/headless/capabilities/qr — the wwebjs-derived QR construction chain (WAWebSignalStoreApi/WAWebUserPrefsInfoStore/WABase64/WAWebUserPrefsMultiDevice/WAWebCompanionRegClientUtils/WAWebConnModel.Conn.ref) MEASURED end-to-end against .lab/test-account-profile (TestProbeQRConstructionSurface), every step resolving to a real value"},
		{domain.CapConnectSession, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceConfirmed, "noise: pkg/bootstrap/pairing_providers.go (noiseSessionStarter over the SessionOrchestrator). headless (2026-08-29, HOUSEKEEP H145): pkg/infra/headless/pairing.Starter, over Sessions.EvaluatorForPairing/core.StartPairingSession — fire-and-forget boot into the registry.KindPairing quota, same contract as noise's Starter"},
		{domain.CapLogoutSession, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceConfirmed, "noise: adapters/user/adapter.go (or session teardown path). headless: pkg/infra/headless/session/disconnector.go — Socket.logout() MEASURED against a real, paired, disposable session (F381, reopening H122): called without throwing, and the page settled from CONNECTED to UNPAIRED with a fresh QR within ~18s"},

		// noise: DetectOwnAccountKind (internal/noise/capabilities/user/
		// accounttype.go) reads a verified-name certificate via usync — a
		// structured protocol field, confirmed by unit tests with a real
		// (marshaled) certificate fixture. headless: Conn.canSetMyPushname()
		// (WAWebConnModel), the same getter capabilities/profile already
		// measured live on the lab account (HOUSEKEEP.md: canSetMyPushname()
		// = false there, confirming that account is Business) — confirmed
		// that the getter resolves and answers, not re-confirmed against a
		// live personal account by this worktree.
		{domain.CapDetectAccountType, domain.StatusSupported, domain.EvidenceConfirmed, domain.StatusSupported, domain.EvidenceProbable, "noise: pkg/infra/noise/adapters/accounttype (internal/noise/capabilities/user/accounttype.go). headless: pkg/infra/headless/accounttype (internal/headless/capabilities/accounttype), reusing the getter measured live in capabilities/profile per HOUSEKEEP.md"},
	}

	for _, r := range rows {
		m.expand(r.capability, domain.EngineNoise, r.noiseStatus, r.noiseEvidence, r.note)
		m.expand(r.capability, domain.EngineHeadless, r.headlessStatus, r.headlessEvidence, r.note)
	}
	log.Info().Int("rows", len(rows)).
		Msg("capabilityregistry: default matrix built from source-reading evidence")
	return m
}
