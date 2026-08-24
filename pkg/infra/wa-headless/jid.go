package waheadless

import (
	"fmt"
	"strings"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/pkg/domain"
)

// ToPageJID renders a domain.JID in the form THIS transport files identities
// under, which is not the form the socket transport uses.
//
// Decision 74 measured why there is no shared canonical form: the socket spells
// a phone identity `…@s.whatsapp.net` while this build spells it `…@c.us`, and
// the vendored socket types even call `c.us` "legacy" although it is what the
// page uses now. So each adapter owns its own suffix, and only the NAMESPACE
// vocabulary is shared.
//
// The consequence this function exists to prevent: a domain.JID produced by the
// socket adapter, handed to this one unchanged, names the right person in the
// wrong namespace — and the page would answer "no such conversation" for a chat
// plainly present. Translating by namespace instead of by string makes that a
// conversion rather than a silent mismatch.
func ToPageJID(j domain.JID) (string, error) {
	raw := domain.NormalizeIdentityInput(string(j))
	if raw == "" {
		return "", fmt.Errorf("waheadless: empty JID")
	}

	switch j.Namespace() {
	case domain.NamespacePhone:
		// Both spellings mean "phone identity"; this transport writes @c.us.
		return user(raw) + waheadless.ServerPhone, nil
	case domain.NamespaceLID:
		return user(raw) + waheadless.ServerLID, nil
	case domain.NamespaceGroup:
		return user(raw) + waheadless.ServerGroup, nil
	case domain.NamespaceUnknown:
		// No server at all is a bare number, and the DEFAULT belongs to the
		// adapter — which is this one. Refusing here would break the routes
		// that promise to accept a bare phone number.
		if strings.Contains(raw, "@") {
			return "", fmt.Errorf("waheadless: %q names a namespace this transport does not serve", raw)
		}
		return raw + waheadless.ServerPhone, nil
	default:
		// Broadcast and newsletter are real namespaces that this conversion has
		// no business guessing at. Saying so is better than inventing a suffix.
		return "", fmt.Errorf("waheadless: namespace %q has no chat form here", j.Namespace())
	}
}

// user is everything before the server, which is what gets re-suffixed.
func user(raw string) string {
	if i := strings.LastIndex(raw, "@"); i >= 0 {
		return raw[:i]
	}
	return raw
}
