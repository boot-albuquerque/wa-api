package spa

import "strings"

// Server suffixes this build files identities under.
const (
	// ServerLID is the identity namespace this build actually uses. 397 of 399
	// messages on the reference account live here.
	ServerLID = "@lid"
	// ServerPhone is the phone-number namespace — what a human types and what
	// most APIs hand in, and NOT what this build indexes by.
	ServerPhone = "@c.us"
	// ServerGroup is a group conversation, which is neither of the above and
	// must never be treated as an unresolved person.
	ServerGroup = "@g.us"
)

// IsUnresolvedIdentity reports whether a jid names a PERSON in the phone
// namespace, which this build does not index people by.
//
// IT IS ONE RULE, IN ONE PLACE, and that is the point. Three readers were
// measured answering a phone jid with a well-formed WRONG answer, each in its own
// way (H148, H151):
//
//	addressbook.DeviceCount  "no device record"      vs  5 devices under the LID
//	chats.ByJID              "no conversation"       vs  found
//	chats.MarkUnread         "no such conversation"  vs  runs
//
// That was not three bugs. It was a decision nobody had taken about WHO resolves
// identity, and the orchestration took it (decision 66): an unresolved identity
// must fail EXPLICITLY — no hidden network call inside a reader, and no rule
// copied into each one.
//
// WHAT IT IS NOT. This does not say "the caller passed something invalid": a
// phone jid is a perfectly good thing to hold. It says this reader cannot answer
// about it, and the caller should resolve first with capabilities/lookup.
//
// GROUPS ARE NOT PEOPLE. A group jid is returned false, because resolution
// answers about people and asking it about a group produced NOT_ON_WHATSAPP for
// groups plainly present — the mistake spa/identity.go already carries a warning
// about.
//
// A reader that CAN answer for both namespaces — contacts.ByJID, whose roster
// merges the two lines (H129) — does not consult this. The rule is "answer if you
// can, refuse explicitly if you cannot, never answer wrongly"; that is one rule,
// and the readers differ only in which half of it applies.
func IsUnresolvedIdentity(jid string) bool {
	j := strings.TrimSpace(jid)
	if j == "" {
		return false
	}
	if strings.HasSuffix(j, ServerGroup) {
		return false
	}
	return strings.HasSuffix(j, ServerPhone)
}
