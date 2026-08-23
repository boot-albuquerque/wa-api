// Package waheadless is the facade of the browser-automation stack: the only
// import path consumers outside this tree may use.
//
// The tree is layered, and the layering is the point:
//
//	core/           session lifecycle and composition root — implementation
//	engine/         the browser boundary; chromedp lives BEHIND this interface
//	spa/            what we know about web.whatsapp.com: hook discovery, injection
//	capabilities/   what the stack can do, one package per capability
//	runtime/        instance pool, recycling, capacity
//	observability/  logging bridge
//
// Dependency direction, declared: nothing outside this tree imports anything
// but this file. `core/` is implementation detail. The same rule in
// internal/wa-noise/ is enforced by scripts/waclient-facade-check.sh, and the
// lesson that produced that script applies here in advance — a rule that does
// not fail the build is a comment, not a rule.
//
// Writing rule: this file holds aliases and delegation only. Anything that
// needs a body lives in core/ or in capabilities/.
//
// The facade is still empty, but the tree is no longer: CAP-02 landed the
// foundation — deadline policy, runner, operation tracing, clean CDP shutdown
// and target priming, in engine/ and observability/. Nothing outside this tree
// consumes it yet, so there is nothing to re-export; the first alias belongs
// with the first capability that has a consumer.
//
// gate_test.go holds the module-wide rules, and it is the enforcement this
// comment used to only ask for: the driver stays inside engine/, no wait keeps
// its clock in the page, and priming never happens inside a bounded operation.
package waheadless

import (
	"context"

	"wa-api/internal/wa-headless/capabilities/avatar"
	"wa-api/internal/wa-headless/capabilities/block"
	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/chatstate"
	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/capabilities/groupreq"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/owner"
	"wa-api/internal/wa-headless/capabilities/presence"
	"wa-api/internal/wa-headless/capabilities/react"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// The server suffixes this build files identities under.
//
// These are the facade's first symbols, and they are here for the reason the
// doc above states: the adapter in pkg/infra/wa-headless has to materialise
// THIS transport's canonical form, and the canonical form is not shared.
// Decision 74 measured why — the socket spells a phone identity
// s.whatsapp.net while this build spells it c.us, and the vendored socket
// types call c.us "legacy" even though it is what the page uses now. Each
// adapter owns its own suffix; only the vocabulary is shared.
//
// Re-exported rather than reachable: an adapter that imported spa/ directly
// would be reaching past the facade, which is the one thing this file exists
// to prevent.
const (
	ServerLID   = spa.ServerLID
	ServerPhone = spa.ServerPhone
	ServerGroup = spa.ServerGroup
)

// IsUnresolvedIdentity reports whether a jid names a PERSON in the phone
// namespace, which this build does not index people by. It is decision 66's
// single rule, and the adapter needs it to refuse explicitly instead of
// answering wrongly.
func IsUnresolvedIdentity(jid string) bool { return spa.IsUnresolvedIdentity(jid) }

// Session lifecycle, for the adapter that has to keep N of them.
//
// These are aliases and delegation, which is all this file may hold. The
// registry in pkg/infra/wa-headless needs to START a session, ASK whether it is
// still up, and STOP it — and it must do so through here rather than reaching
// into runtime/ or core/.
type (
	// StartConfig is everything a session needs to boot. DebuggingPort zero is
	// the normal case: Chromium picks an ephemeral port and publishes it in the
	// profile, which is what ties the endpoint to the browser we launched
	// (decision 75).
	StartConfig = core.StartConfig
	// Session is a live headless session.
	Session = core.Session
	// Holder owns exactly ONE session and keeps it alive across commands. One
	// per WhatsApp profile: invariant 13 is one profile, one active owner.
	Holder = runtime.Holder
	// StopVia records HOW a browser went down, so a dirty stop is never
	// indistinguishable from a clean one.
	StopVia = engine.StopVia
)

// Errors a caller has to tell apart, because each one calls for a different
// reaction and "no session" alone cannot say which.
var (
	// ErrHolderStopped means this holder was stopped and will not boot again.
	ErrHolderStopped = runtime.ErrHolderStopped
	// ErrSessionDied means the browser process is gone underneath us.
	ErrSessionDied = runtime.ErrSessionDied
	// ErrNoSession means nothing has been started yet.
	ErrNoSession = runtime.ErrNoSession
)

// NewHolder prepares a Holder for cfg. It does not boot; the first Session
// call does.
func NewHolder(cfg StartConfig) *Holder { return runtime.NewHolder(cfg) }

// Operating a session: the pieces every capability is built from.
//
// A capability takes a Runner (which carries the deadline policy and the
// operation log) and an Evaluator (which is how anything reaches the page).
// Both come from here rather than from engine/ and spa/ directly, because an
// adapter that imported those would be reaching past this file.
type (
	// Runner bounds every operation by its class budget and records it.
	Runner = engine.Runner
	// Tab is the page a session drives.
	Tab = engine.Tab
	// Evaluator runs an expression in the page. It does NOT await promises —
	// invariant 6 — so a capability that needs an answer parks it on a global
	// and polls from Go.
	Evaluator = spa.Evaluator
)

// NewRunner builds a Runner with the default deadline policy.
func NewRunner() *Runner { return engine.NewRunner() }

// Chat state: archive, pin and mute, which the page exposes as one family.
type (
	// ChatStateSetter changes archive, pin and mute state.
	ChatStateSetter = chatstate.Setter
	// ChatStateChange is what a set actually did, read back. Invariant 14: no
	// silent success — every write is followed by a postcondition read.
	ChatStateChange = chatstate.Change
)

// NewChatState builds the chat-state capability over a session's page.
func NewChatState(runner *Runner, eval Evaluator) *ChatStateSetter {
	return chatstate.New(runner, eval)
}

// Presence: what this account announces it is doing.
type (
	// PresenceAnnouncer announces availability and per-chat state.
	PresenceAnnouncer = presence.Announcer
	// PresenceState is a per-chat announcement: composing, paused, recording.
	PresenceState = presence.State
)

// The per-chat states, re-exported so an adapter maps into a CLOSED set instead
// of forwarding a caller's string to a page function that may not exist.
const (
	PresenceComposing = presence.StateComposing
	PresencePaused    = presence.StatePaused
	PresenceRecording = presence.StateRecording
)

// ErrUnknownPresenceState refuses a state this stack does not know, rather than
// defaulting: "we sent nothing" and "we sent paused" look identical afterwards.
var ErrUnknownPresenceState = presence.ErrUnknownState

// NewPresence builds the presence capability over a session's page.
func NewPresence(runner *Runner, eval Evaluator) *PresenceAnnouncer {
	return presence.New(runner, eval)
}

// Blocking: who this account refuses to hear from.
type (
	// Blocker blocks, unblocks and lists.
	Blocker = block.Blocker
	// BlockResult is what a block or unblock actually did, read back —
	// invariant 14. It carries blocklist SIZES and never the entries.
	BlockResult = block.Result
)

// The refusals this capability distinguishes. They are separate values because
// each one calls for a different answer at the HTTP boundary, and a single
// "block failed" would make them indistinguishable.
var (
	// ErrBlockNoContact means the contact is not in the loaded roster.
	ErrBlockNoContact = block.ErrNoContact
	// ErrBlockGroup means a group was passed; a group cannot be blocked.
	ErrBlockGroup = block.ErrGroup
	// ErrBlockNotOnWhatsApp means the identity does not resolve on this build.
	ErrBlockNotOnWhatsApp = block.ErrNotOnWhatsApp
	// ErrBlockNoChat means this build refuses to block a phone contact with no
	// existing conversation.
	ErrBlockNoChat = block.ErrNoChatToBlockFrom
)

// NewBlocker builds the blocking capability over a session's page.
func NewBlocker(runner *Runner, eval Evaluator) *Blocker { return block.New(runner, eval) }

// Identity: who somebody is, in a build that files nearly everything under LID.
type (
	// Resolver answers identity questions by ASKING the page, which is the only
	// place that knows. Decision 66: a reader that cannot answer must refuse
	// explicitly rather than answer wrongly.
	Resolver = lookup.Resolver
	// Identity is what the server returned for one query.
	Identity = lookup.Identity
	// IdentityPair is the LID and the phone name of the same person. Either can
	// be empty, and empty means "the page did not produce it" — never "there is
	// none".
	IdentityPair = lookup.Pair
)

// ErrNotOnWhatsApp is a DEFINITIVE answer: the identity does not resolve on this
// build. It is not a lookup failure, and conflating the two would make a real
// "no such account" indistinguishable from a timeout.
var ErrNotOnWhatsApp = lookup.ErrNotOnWhatsApp

// NewResolver builds the identity capability over a session's page.
func NewResolver(runner *Runner, eval Evaluator) *Resolver { return lookup.New(runner, eval) }

// Avatar: the profile picture.
type (
	// AvatarFetcher reads profile pictures.
	AvatarFetcher = avatar.Fetcher
	// AvatarPicture is what the page knows about one picture. Present is the
	// field to branch on — an empty URL with Present true would be a different
	// bug from "this person has no picture".
	AvatarPicture = avatar.Avatar
)

// NewAvatarFetcher builds the avatar capability over a session's page.
func NewAvatarFetcher(runner *Runner, eval Evaluator) *AvatarFetcher {
	return avatar.New(runner, eval)
}

// The roster: who this account knows.
type (
	// ContactLister reads the contact collection.
	ContactLister = contacts.Lister
	// ContactRoster is the merged listing. Rows > len(Contacts) is the normal
	// state: the same person arrives as two rows, one per identity.
	ContactRoster = contacts.Roster
	// RosterContact is one person, already merged across their two identities.
	RosterContact = contacts.Contact
)

// NewContactLister builds the roster capability over a session's page.
func NewContactLister(runner *Runner, eval Evaluator) *ContactLister {
	return contacts.New(runner, eval)
}

// Channels, which the protocol calls newsletters.
type (
	// ChannelManager reads and administers channels.
	ChannelManager = channel.Manager
	// ChannelEntry is one channel in a listing.
	ChannelEntry = channel.DirectoryEntry
)

// NewChannelManager builds the channel capability over a session's page.
func NewChannelManager(runner *Runner, eval Evaluator) *ChannelManager {
	return channel.NewManager(runner, eval)
}

// The account's own identity.
type (
	// OwnIdentity is who this session is, in both namespaces.
	OwnIdentity = owner.Identity
	// OwnWID is one of the two identifiers.
	OwnWID = owner.WID
)

// RefreshOwnIdentity asks the page who this account is.
//
// DisplayName comes back EMPTY on this build, and that was measured rather than
// assumed: the getter exists — it is a function, not absent — and returns null
// against the real paired profile. Two candidates on the neighbouring module do
// not exist at all, and the measurement discarded them.
func RefreshOwnIdentity(ctx context.Context, runner *Runner, eval Evaluator, label string) (OwnIdentity, error) {
	return owner.Refresh(ctx, runner, eval, label)
}

// Priming the roster: asking the page to refresh what it knows about contacts.
type (
	// RosterPrimeResult reports what moved. "Nothing changed" is a SUCCESS with
	// Changed() false — on a roster that is already current that is the correct
	// answer, and the measured ordinary case.
	RosterPrimeResult = contacts.PrimeResult
)

// Reading a conversation, and reacting to a message.
type (
	// ChatLister reads and marks conversations.
	ChatLister = chats.Lister
	// MarkReadResult is the postcondition of marking read, read back —
	// invariant 14.
	MarkReadResult = chats.MarkResult
	// ChatList is a page of conversations WITH the denominators that make it
	// readable: Total > len(Chats) means the listing was truncated, and a
	// caller reading a short list as a complete one is the silent-incompleteness
	// failure this module keeps meeting.
	ChatList = chats.List
	// Chat is one conversation. Title is PII and never rendered by String.
	Chat = chats.Chat
	// GroupInvite is what a group invite link resolves to.
	GroupInvite = group.InviteInfo
	// GroupCreated is a group this session made or found already there.
	GroupCreated = group.Group
	// GroupJoined is the outcome of following an invite. Pending says the group
	// asks for approval and this became a REQUEST rather than a membership.
	GroupJoined = group.Joined
	// GroupInviteCode is a group's invite credential. Code is never rendered,
	// and Link() is a method so the dangerous form has to be asked for.
	GroupInviteCode = group.Invite
	// Reactor adds and removes reactions.
	Reactor = react.Reactor
	// ReactionResult is what a reaction actually did.
	ReactionResult = react.Result
)

// NewChatLister builds the chat capability over a session's page.
func NewChatLister(runner *Runner, eval Evaluator) *ChatLister { return chats.New(runner, eval) }

// NewReactor builds the reaction capability over a session's page.
func NewReactor(runner *Runner, eval Evaluator) *Reactor { return react.New(runner, eval) }

// Groups: membership and admin roles.
type (
	// GroupManager changes group membership and roles.
	GroupManager = group.Manager
	// GroupMembership is what a membership change did — COUNTS, never
	// identities, because the log must not name who is in a group.
	GroupMembership = group.Membership
	// GroupAdminChange is what a promote or demote did.
	GroupAdminChange = group.AdminChange
)

// NewGroupManager builds the group capability over a session's page.
func NewGroupManager(runner *Runner, eval Evaluator) *GroupManager {
	return group.New(runner, eval)
}

// Group membership requests, and the policy that produces them.
type (
	// GroupRequestManager reads and decides membership requests.
	GroupRequestManager = groupreq.Manager
	// GroupRequestList is the pending requests of one group.
	GroupRequestList = groupreq.List
	// GroupRequestAction is what a decision did for ONE requester. Code is the
	// page's own error number when OK is false.
	GroupRequestAction = groupreq.ActionResult
	// GroupPolicy is a group setting the page accepts.
	GroupPolicy = group.Policy
	// GroupRename is what renaming a group did. It carries no verification
	// flag: it names the model FIELD that carried the change, which is the
	// same evidence in a more useful shape.
	GroupRename = group.Rename
	// GroupDescribed is the description READ BACK after the change — the
	// strongest of the three group confirmations, because it is comparable
	// with what was asked for.
	GroupDescribed = group.Described
	// GroupPolicyChange is what a policy change did, READ BACK. Unlike a
	// membership change, this one IS verifiable by the acting session — the
	// difference is measured, not assumed (H85).
	GroupPolicyChange = group.PolicyChange
)

// PolicyJoinNeedsApproval is the policy that turns joins into requests.
const (
	PolicyJoinNeedsApproval = group.PolicyJoinNeedsApproval
	// PolicyMessagesAdminsOnly is `announcement` — only admins may send.
	PolicyMessagesAdminsOnly = group.PolicyMessagesAdminsOnly
	// PolicyInfoAdminsOnly is `restrict` — only admins may edit the group's
	// own information.
	PolicyInfoAdminsOnly = group.PolicyInfoAdminsOnly
)

// NewGroupRequestManager builds the request capability over a session's page.
func NewGroupRequestManager(runner *Runner, eval Evaluator) *GroupRequestManager {
	return groupreq.New(runner, eval)
}
