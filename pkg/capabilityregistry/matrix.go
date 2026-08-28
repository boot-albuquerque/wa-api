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
	engine     string
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
	byEngine map[string]map[domain.Capability]bool
}

func newMatrix() *matrix {
	return &matrix{
		cells: make(map[key]cell),
		// Pre-seeded with both known engines so set() never needs a
		// lazy-init branch — see set()'s own comment on why that keeps it
		// trivial by construction, not by hiding a branch.
		byEngine: map[string]map[domain.Capability]bool{
			domain.EngineNoise:      make(map[domain.Capability]bool),
			domain.EngineWaHeadless: make(map[domain.Capability]bool),
		},
	}
}

// set records one cell. It is intentionally two statements and no
// branches: byEngine is pre-seeded for both known engines in newMatrix, so
// there is nothing here to initialize lazily.
func (m *matrix) set(capability domain.Capability, engine string, account domain.AccountType, c cell) {
	m.cells[key{capability, engine, account}] = c
	m.byEngine[engine][capability] = true
}

func (m *matrix) capabilitiesForEngine(engine string) []domain.Capability {
	out := make([]domain.Capability, 0, len(m.byEngine[engine]))
	for c := range m.byEngine[engine] {
		out = append(out, c)
	}
	log.Debug().Str("engine", engine).Int("count", len(out)).
		Msg("capabilityregistry: listed capabilities for engine")
	return out
}

// decide looks up one cell and turns it into a CapabilityDecision. A missing
// cell — a (capability, engine, account_type) nobody ever recorded — is
// PROPAGATED as unknown, never silently treated as unsupported or supported:
// the whole point of a registry with static evidence is that absence of
// evidence is visible, not swallowed.
func (m *matrix) decide(capability domain.Capability, engine string, account domain.AccountType) CapabilityDecision {
	c, ok := m.cells[key{capability, engine, account}]
	if !ok {
		log.Warn().Str("capability", capability.String()).Str("engine", engine).
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
func (m *matrix) expand(capability domain.Capability, engine string, status domain.CapabilityStatus, evidence domain.EvidenceStatus, note string) {
	m.set(capability, engine, domain.AccountTypeUnknown, cell{status, evidence, note})

	perTypeEvidence := domain.EvidenceNotTested
	if evidence == domain.EvidenceUnknown {
		perTypeEvidence = domain.EvidenceUnknown
	}
	log.Debug().Str("capability", capability.String()).Str("engine", engine).
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

	waNoiseStatus   domain.CapabilityStatus
	waNoiseEvidence domain.EvidenceStatus

	waHeadlessStatus   domain.CapabilityStatus
	waHeadlessEvidence domain.EvidenceStatus

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
// deliberately reported as unknown, not engine_unsupported: for wa-headless
// specifically, "no adapter written" and "the SPA cannot do this at all" are
// different claims, and this pass verified only the former.
const absentAdapter = "no adapter method found for this engine; ambiguous between not_implemented and engine_unsupported — nobody has checked whether the transport itself could serve it"

// NewDefaultMatrix builds the capability matrix this worktree could verify
// by reading code (grep for the port's method name under
// pkg/infra/wa-noise/... and pkg/infra/wa-headless/..., then reading the
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
		// wa-headless has no adapter under pkg/infra/wa-headless/messenger
		// (or any sibling package) implementing message CREATION — that
		// package only covers actions on messages that already exist
		// (MarkRead/SendReaction/EditMessage/RevokeMessage/SendPollVote, see
		// its own doc comment). Every "new message" capability is therefore
		// unknown for wa_headless, not confirmed-absent.
		{domain.CapSendText, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: pkg/infra/wa-noise/adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendLocation, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendContact, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendPoll, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendTemplate, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendList, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendImage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendDocument, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendAudio, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendVideo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendSticker, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendButtons, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/messenger.go. wa_headless: " + absentAdapter},
		{domain.CapSendCarousel, domain.StatusSupported, domain.EvidenceConfirmed, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: field-verified, HOUSEKEEP.md F216 (photographed on device). wa_headless: " + absentAdapter},
		{domain.CapForwardMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/forward.go. wa_headless: " + absentAdapter},

		// --- Actions on existing messages ---------------------------------
		{domain.CapMarkRead, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise: adapters/chat/messenger.go. wa_headless: pkg/infra/wa-headless/messenger/messenger.go (local ack only — receipt-to-sender NOT confirmed, see H160 cited in that file's doc comment)"},
		{domain.CapSendReaction, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter},
		{domain.CapRevokeMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter},
		{domain.CapEditMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter},
		{domain.CapVotePoll, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter},
		{domain.CapStarMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildStar) — NOTE this contradicts docs/CAPACIDADES.md's stale claim that StarMessage 'não existe'; that document is self-flagged desatualizado since 2026-08-24. wa_headless: " + absentAdapter},

		// --- Presence -------------------------------------------------------
		{domain.CapSendPresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/presence)"},
		{domain.CapSendChatPresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/presence)"},
		{domain.CapSubscribePresence, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/presence/controller.go. wa_headless: " + absentAdapter},

		// --- Chat-level operations -------------------------------------
		{domain.CapArchiveChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter},
		{domain.CapMuteChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildMute) — contradicts stale docs/CAPACIDADES.md claim, see CapStarMessage note. wa_headless: " + absentAdapter},
		{domain.CapPinChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go, via app-state patch (appstate.BuildPin). wa_headless: " + absentAdapter},
		{domain.CapRejectCall, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapRequestUnavailableMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSetDisappearingTimerChat, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSetDefaultDisappearingTimer, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSetStatusMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapRequestHistorySync, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSyncContactRoster, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/roster or appstate)"},
		{domain.CapAccessProfileData, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/profile)"},

		// --- Media -----------------------------------------------------
		{domain.CapDownloadMedia, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/chat/downloader.go. wa_headless: " + absentAdapter},

		// --- Newsletter (channel) ---------------------------------------
		{domain.CapListSubscribedNewsletters, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/newsletter)"},
		{domain.CapCreateNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/newsletter)"},
		{domain.CapGetNewsletterInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapGetNewsletterInfoByInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter + " (grep matched only NewsletterInfo, not the ByInvite variant, in wa_noise too — recorded together with NewsletterInfo since both live in the same adapter method family; treat as same confidence)"},
		{domain.CapFollowNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapUnfollowNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapMuteNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapListNewsletterMessages, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapListNewsletterMessageUpdates, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapMarkNewsletterViewed, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapReactToNewsletterMessage, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSubscribeNewsletterLiveUpdates, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapDemoteNewsletterAdmin, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapChangeNewsletterOwner, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapDeleteNewsletter, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapInviteNewsletterAdmin, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go (CreateNewsletterAdminInvite). wa_headless: " + absentAdapter},
		{domain.CapAcceptNewsletterAdminInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapRevokeNewsletterAdminInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/misc/adapter.go. wa_headless: " + absentAdapter},

		// --- Group -------------------------------------------------------
		{domain.CapGetGroupInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupdir)"},
		{domain.CapGetGroupInfoFromInviteLink, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupdir)"},
		{domain.CapGetGroupInviteLink, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupdir)"},
		{domain.CapListGroupNames, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupdir)"},
		{domain.CapListJoinedGroups, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupdir)"},
		{domain.CapCreateGroup, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/grouplife)"},
		{domain.CapJoinGroupViaInvite, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/grouplife)"},
		{domain.CapLeaveGroup, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/grouplife)"},
		{domain.CapSetGroupName, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupset)"},
		{domain.CapSetGroupTopic, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupset)"},
		{domain.CapSetGroupAnnounceMode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupset)"},
		{domain.CapSetGroupLocked, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupset)"},
		{domain.CapUpdateGroupParticipants, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupmembers) — NOTE the port's own doc comment records that on headless the write can succeed while the acting session cannot read it back (H58/H65); Supported here means the OPERATION is served, not that every observable side effect is"},
		{domain.CapSetGroupPhoto, domain.StatusSupported, domain.EvidenceProbable, domain.StatusEngineUnsupported, domain.EvidenceConfirmed, "wa_noise: adapters/group/adapter.go. wa_headless: CONFIRMED absent — docs/CAPACIDADES.md H140, cited verbatim in port doc comment (pkg/application/contracts/group_ports.go GroupPhotoSetter): 'no build headless os módulos de foto da página estão ausentes'"},
		{domain.CapSetGroupEphemeralTimer, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/group/adapter.go. wa_headless: " + absentAdapter + " — port doc comment (GroupEphemeralSetter) says explicitly this one has NOT been measured on headless, unlike GroupPhotoSetter"},
		{domain.CapGetSubGroups, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/group/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapGetLinkedGroupsParticipants, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/group/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapLinkGroupToCommunity, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/group/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapUnlinkGroupFromCommunity, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/group/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapListGroupJoinRequests, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupreq)"},
		{domain.CapUpdateGroupJoinRequests, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupreq)"},
		{domain.CapSetGroupJoinApprovalMode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/groupreq)"},

		// --- User / contacts ---------------------------------------------
		{domain.CapCheckIsOnWhatsApp, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/identity)"},
		{domain.CapResolveLIDForPN, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/identity)"},
		{domain.CapResolvePNForLID, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/identity)"},
		{domain.CapResolveManyLIDs, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/identity)"},
		{domain.CapGetProfilePicture, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/avatar)"},
		{domain.CapListContacts, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/roster)"},
		{domain.CapListContactNames, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/roster)"},
		{domain.CapGetUserInfo, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/roster)"},
		{domain.CapGetBlocklist, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/blocklist)"},
		{domain.CapUpdateBlocklist, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/blocklist)"},
		{domain.CapGetPrivacySettings, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/user/adapter.go. wa_headless: " + absentAdapter},
		{domain.CapSetPrivacySetting, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/user/adapter.go. wa_headless: " + absentAdapter},

		// --- Pairing / session lifecycle ----------------------------------
		{domain.CapCheckPairingStatus, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/pairing/adapter.go. wa_headless: " + absentAdapter + " — plausible this is engine_unsupported (headless auth is QR/cookie-based, not a pairing code), but that has not been confirmed by reading the auth flow, so it stays unknown rather than asserted"},
		{domain.CapRequestPairingCode, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/pairing/adapter.go. wa_headless: " + absentAdapter + " (same caveat as check_pairing_status)"},
		{domain.CapDisconnectSession, domain.StatusSupported, domain.EvidenceProbable, domain.StatusSupported, domain.EvidenceProbable, "wa_noise+wa_headless: " + probableAdapter + " (pkg/infra/wa-headless/session)"},
		{domain.CapLogoutSession, domain.StatusSupported, domain.EvidenceProbable, domain.StatusUnknown, domain.EvidenceUnknown, "wa_noise: adapters/user/adapter.go (or session teardown path). wa_headless: " + absentAdapter},

		// wa_noise: DetectOwnAccountKind (internal/wa-noise/capabilities/user/
		// accounttype.go) reads a verified-name certificate via usync — a
		// structured protocol field, confirmed by unit tests with a real
		// (marshaled) certificate fixture. wa_headless: Conn.canSetMyPushname()
		// (WAWebConnModel), the same getter capabilities/profile already
		// measured live on the lab account (HOUSEKEEP.md: canSetMyPushname()
		// = false there, confirming that account is Business) — confirmed
		// that the getter resolves and answers, not re-confirmed against a
		// live personal account by this worktree.
		{domain.CapDetectAccountType, domain.StatusSupported, domain.EvidenceConfirmed, domain.StatusSupported, domain.EvidenceProbable, "wa_noise: pkg/infra/wa-noise/adapters/accounttype (internal/wa-noise/capabilities/user/accounttype.go). wa_headless: pkg/infra/wa-headless/accounttype (internal/wa-headless/capabilities/accounttype), reusing the getter measured live in capabilities/profile per HOUSEKEEP.md"},
	}

	for _, r := range rows {
		m.expand(r.capability, domain.EngineNoise, r.waNoiseStatus, r.waNoiseEvidence, r.note)
		m.expand(r.capability, domain.EngineWaHeadless, r.waHeadlessStatus, r.waHeadlessEvidence, r.note)
	}
	log.Info().Int("rows", len(rows)).
		Msg("capabilityregistry: default matrix built from source-reading evidence")
	return m
}
