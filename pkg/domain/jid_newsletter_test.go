package domain

import "testing"

// F271 — IsNewsletter is the rule that decides whether a request naming a
// channel can be admitted at all, so it has to be strict where IsLID and IsPN
// are merely descriptive: a jid with the right suffix but no user part names
// no channel, and letting it through is how "@newsletter" would reach the
// adapter and come back as a 500.
func TestJID_IsNewsletter(t *testing.T) {
	cases := []struct {
		jid  JID
		want bool
	}{
		{"120363000000000000@newsletter", true},
		{"1@newsletter", true},

		// The three values measured in the field against POST /newsletter/info.
		{"   ", false},
		{"nao-e-jid", false},
		{"554192421234@s.whatsapp.net", false},

		{"", false},
		{"@newsletter", false},            // right server, no channel
		{"   @newsletter", false},         // blank user part
		{"1 @newsletter", false},          // trailing blank in the user part
		{" 1@newsletter", false},          // leading blank in the user part
		{"1@newsletter ", false},          // trailing blank after the server
		{"1@NEWSLETTER", false},           // the server is compared verbatim
		{"1@newsletter@newsletter", true}, // last "@" wins, user part is "1@newsletter"
		{"120363000000000000@lid", false},
		{"120363000000000000@g.us", false},
	}

	for _, c := range cases {
		if got := c.jid.IsNewsletter(); got != c.want {
			t.Errorf("JID(%q).IsNewsletter() = %v, want %v", c.jid, got, c.want)
		}
	}
}

// The channel rule must not have moved the other two: LID and PN are the pair
// ARMADILHAS nº6 is about, and a change to the shared helpers here would show
// up as one of these flipping.
func TestJID_NewsletterDoesNotDisturbLIDAndPN(t *testing.T) {
	if !JID("123@lid").IsLID() || JID("123@lid").IsPN() || JID("123@lid").IsNewsletter() {
		t.Error("lid jid misclassified")
	}
	if !JID("5511999999999@s.whatsapp.net").IsPN() || JID("5511999999999@s.whatsapp.net").IsNewsletter() {
		t.Error("pn jid misclassified")
	}
}
