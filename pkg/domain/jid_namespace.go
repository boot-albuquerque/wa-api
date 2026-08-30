package domain

import "strings"

// Namespace names WHICH identity space a JID lives in, independently of which
// transport spelled it.
//
// It exists because the two transports do NOT agree on the canonical suffix for
// the same person, and that was measured, not assumed:
//
//	socket (noise)   a bare number becomes 5511…@s.whatsapp.net
//	page   (headless) a phone identity is filed under 5511…@c.us
//
// The irony is on the record: the vendored socket types call c.us
// LegacyUserServer (internal/noise/protocol/types/jid.go:19), yet c.us is
// the CURRENT namespace of the build we drive — and it is not dead on the
// socket either, where it is the USync query suffix
// (internal/noise/capabilities/user/info.go:46).
//
// So there is no shared canonical form to extract, and pretending otherwise is
// how a JID produced by one adapter becomes silently wrong in the other. What
// IS shared is this vocabulary: each adapter materialises its own suffix, and
// both agree on what the suffix MEANS.
type Namespace string

// The identity spaces, and the suffixes each transport spells them with.
const (
	// NamespaceUnknown is the honest answer for a JID with no server, or with
	// a server neither transport claims. It is NOT an error: naming the gap is
	// the point, and a caller that cannot serve it should refuse explicitly.
	NamespaceUnknown Namespace = ""
	// NamespacePhone is a person addressed by phone number.
	NamespacePhone Namespace = "phone"
	// NamespaceLID is a person addressed by the hidden linked-identity space,
	// which is where the page build files nearly everything.
	NamespaceLID Namespace = "lid"
	// NamespaceGroup is a group conversation, which is not a person and must
	// never be treated as one.
	NamespaceGroup Namespace = "group"
	// NamespaceBroadcast covers status and broadcast lists.
	NamespaceBroadcast Namespace = "broadcast"
	// NamespaceNewsletter is a channel.
	NamespaceNewsletter Namespace = "newsletter"
)

// serverNamespaces maps every server suffix either transport uses onto the
// space it names. Two suffixes mapping to NamespacePhone is the whole reason
// this file exists.
var serverNamespaces = map[string]Namespace{
	"s.whatsapp.net": NamespacePhone,
	"c.us":           NamespacePhone,
	"lid":            NamespaceLID,
	"g.us":           NamespaceGroup,
	"broadcast":      NamespaceBroadcast,
	"newsletter":     NamespaceNewsletter,
}

// Server returns the part after the last "@", or "" when there is none.
//
// The LAST "@" and not the first: the socket parser splits on "@" and keeps
// parts[1], silently discarding anything after a second one, so "a@b@c" is a
// shape that reaches here rather than being rejected upstream.
func (j JID) Server() string {
	s := string(j)
	if i := strings.LastIndex(s, "@"); i >= 0 {
		return s[i+1:]
	}
	return ""
}

// Namespace reports which identity space this JID lives in.
//
// A JID with no server is NamespaceUnknown rather than a guessed default: the
// default belongs to the adapter, which knows its own transport, and guessing
// it here would put one transport's canonical form in the shared layer — the
// exact silent-wrongness this vocabulary exists to prevent.
func (j JID) Namespace() Namespace {
	return serverNamespaces[j.Server()]
}

// NormalizeIdentityInput applies the input cleanup both transports need before
// they parse: trim surrounding space and drop a leading "+".
//
// This is deliberately the ONLY parsing that is shared. It touches what a human
// typed, not what a transport considers canonical.
func NormalizeIdentityInput(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "+")
}
