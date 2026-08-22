package events

// Group system messages, and the four public events they become.
//
// ONE CARRIER, FOUR EVENTS. The page has no "group join" notification: it adds
// an ordinary message whose kind is "gp2" and whose subtype says what happened.
// The reference (Client.js:585–645 of the pinned upstream) dispatches that one
// carrier into four events, and this file is the same table, written where the
// decision belongs — in Go, because deciding in the page is what invariant 6
// forbids.
//
// WHAT WAS ACTUALLY OBSERVED, said plainly (H119). Measured 2026-08-22 against
// the lab account: 395 loaded messages, exactly ONE of kind "gp2", subtype
// "membership_approval_mode" — which maps to GroupUpdated. The other three
// events have their subtypes mapped from the upstream table and have NEVER been
// seen firing here, because producing them needs a group membership to actually
// change.
//
// That is the same position CallIncoming occupies in this package, and it is
// stated for the same reason: the mapping costs nothing and is the door a
// solution comes through, but nobody should read its presence as a delivered
// capability (H93).

// kindGroupSystem is the message kind that carries every group notification.
const kindGroupSystem = "gp2"

const (
	// GroupJoined is somebody entering the group — added by an admin, or in
	// through an invite link. NEVER OBSERVED on this account.
	GroupJoined Type = "group.joined"
	// GroupLeft is somebody leaving or being removed. NEVER OBSERVED.
	GroupLeft Type = "group.left"
	// GroupAdminChanged is a promotion or a demotion. NEVER OBSERVED.
	GroupAdminChanged Type = "group.admin_changed"
	// GroupUpdated is everything else the group announces about itself —
	// subject, description, approval mode, picture.
	//
	// IT IS THE ONLY ONE OF THE FOUR MEASURED FIRING: subtype
	// "membership_approval_mode", 1 of 395 messages.
	GroupUpdated Type = "group.updated"
)

// groupSubtypes maps the page's subtype to the event it becomes.
//
// The keys come from the pinned upstream's own branches, NOT from guessing:
// add/invite/linked_group_join -> join; remove/leave -> left;
// promote/demote -> admin changed; everything else -> update.
//
// A subtype that is not here does NOT become GroupUpdated by default, and that
// is deliberate: the reference's final `else` swallows anything, which means a
// subtype Meta adds tomorrow would silently arrive labelled as an update. Here
// an unmapped subtype stays MessageAdded, where a subscriber can still see it
// and where its kind and subtype are both readable.
var groupSubtypes = map[string]Type{
	"add":               GroupJoined,
	"invite":            GroupJoined,
	"linked_group_join": GroupJoined,

	"remove": GroupLeft,
	"leave":  GroupLeft,

	"promote": GroupAdminChanged,
	"demote":  GroupAdminChanged,

	// The update family, listed one by one rather than caught by a default.
	"subject":                     GroupUpdated,
	"description":                 GroupUpdated,
	"picture":                     GroupUpdated,
	"announce":                    GroupUpdated,
	"restrict":                    GroupUpdated,
	"membership_approval_mode":    GroupUpdated,
	"membership_approval_request": GroupUpdated,
	"created_membership_requests": GroupUpdated,
}

// GroupTypeFor reports which group event a subtype becomes, and whether it is
// one this package recognises at all.
func GroupTypeFor(subtype string) (Type, bool) {
	t, ok := groupSubtypes[subtype]
	return t, ok
}

// GroupTypes is the four, in a fixed order.
var GroupTypes = []Type{GroupJoined, GroupLeft, GroupAdminChanged, GroupUpdated}
