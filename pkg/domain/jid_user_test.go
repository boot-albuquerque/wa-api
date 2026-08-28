package domain

import "testing"

// F271 (follow-up) — IsUserJID is the rule that decides whether a userJID
// field can be admitted for demote/change_owner/admin_invite/admin_invite
// revoke, the same way IsNewsletter decides for the channel jid. It has to be
// strict where IsLID and IsPN are merely descriptive, or a malformed value
// reaches the adapter and comes back as a 500 instead of a 400 — the exact gap
// left open by the original F271 fix, recorded in HOUSEKEEP.md under
// "O que a correcção NÃO cobriu".
func TestJID_IsUserJID(t *testing.T) {
	cases := []struct {
		jid  JID
		want bool
	}{
		{"5511999999999@s.whatsapp.net", true},
		{"123456789@lid", true},

		// The two values measured in the field against POST /newsletter/admin-invite.
		{"   ", false},
		{"nao-e-jid", false},

		{"", false},
		{"@s.whatsapp.net", false},               // right server, no user part
		{"@lid", false},                          // right server, no user part
		{"   @lid", false},                       // blank user part
		{"1 @lid", false},                        // trailing blank in the user part
		{" 1@lid", false},                        // leading blank in the user part
		{"1@lid ", false},                        // trailing blank after the server
		{"120363000000000000@newsletter", false}, // wrong server
		{"120363000000000000@g.us", false},       // wrong server (group)
	}

	for _, c := range cases {
		if got := c.jid.IsUserJID(); got != c.want {
			t.Errorf("JID(%q).IsUserJID() = %v, want %v", c.jid, got, c.want)
		}
	}
}

// TestJID_UserJIDDoesNotDisturbOtherRules keeps the shared suffix helpers
// (ARMADILHAS nº6, LID vs PN) and IsNewsletter (F271) unaffected by this
// addition.
func TestJID_UserJIDDoesNotDisturbOtherRules(t *testing.T) {
	if !JID("123@lid").IsLID() || JID("123@lid").IsNewsletter() {
		t.Error("lid jid misclassified")
	}
	if !JID("5511999999999@s.whatsapp.net").IsPN() || JID("5511999999999@s.whatsapp.net").IsNewsletter() {
		t.Error("pn jid misclassified")
	}
	if JID("120363000000000000@newsletter").IsUserJID() {
		t.Error("channel jid must not pass as a user jid")
	}
}
