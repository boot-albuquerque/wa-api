package spa

import "testing"

// THE PHONE NAMESPACE IS THE ONE THIS BUILD DOES NOT INDEX PEOPLE BY.
func TestAPhoneJIDIsUnresolved(t *testing.T) {
	if !IsUnresolvedIdentity("5516999999999@c.us") {
		t.Fatal("a phone jid was not reported as unresolved; the readers that " +
			"refuse it would go back to answering wrongly")
	}
}

func TestALidIsResolved(t *testing.T) {
	if IsUnresolvedIdentity("123456789@lid") {
		t.Fatal("a lid was reported as unresolved; every read would refuse")
	}
}

// A GROUP IS NOT AN UNRESOLVED PERSON, and conflating them is a mistake this
// module already made once: resolution answers about PEOPLE and returns
// NOT_ON_WHATSAPP for a group plainly present.
func TestAGroupIsNotAnUnresolvedIdentity(t *testing.T) {
	if IsUnresolvedIdentity("120363000000000000@g.us") {
		t.Fatal("a group jid was reported as an unresolved identity; every group " +
			"read would refuse")
	}
}

// AN EMPTY JID IS SOMEBODY ELSE'S ERROR. Each reader already refuses it with its
// own "no jid given", and answering true here would replace that precise message
// with a misleading one about resolution.
func TestAnEmptyJIDIsNotThisRulesProblem(t *testing.T) {
	for _, in := range []string{"", "   "} {
		if IsUnresolvedIdentity(in) {
			t.Fatalf("%q was reported as an unresolved identity rather than being "+
				"left to the caller's own empty check", in)
		}
	}
}

// SUFFIX, NOT SUBSTRING. A jid that merely CONTAINS the phone suffix somewhere is
// not in the phone namespace.
func TestTheMatchIsOnTheSuffix(t *testing.T) {
	if IsUnresolvedIdentity("@c.usXYZ@lid") {
		t.Fatal("the rule matched a substring instead of the trailing namespace")
	}
}
