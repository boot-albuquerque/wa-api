package events

import (
	"context"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// decodeForTest runs one drain against a page double that answers exactly the
// given rows. It exercises the REAL decode path — including the group
// reclassification — rather than reimplementing it.
func decodeForTest(t *testing.T, rows string) []Event {
	t.Helper()
	eval := func(ctx context.Context, expr string, out *string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		*out = rows
		return nil
	}
	p := NewPump(engine.NewRunner(), eval, NewHub())
	evs, err := p.drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	return evs
}

func installScriptForTest() string { return installScript() }

// installCodeForTest is the install script WITHOUT its comments.
//
// The distinction is not pedantry: the first version of
// TestTheVoteListenerDoesNotPatchThePage failed because the script's own comment
// EXPLAINS that the reference patches bulkUpsert, and the guard matched that
// prose. A guard that matches its own documentation is a guard that cannot be
// written honestly — this repository has paid for that eight times.
func installCodeForTest() string {
	var b strings.Builder
	for _, line := range strings.Split(installScript(), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") {
			continue
		}
		if i := strings.Index(line, "// "); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// THE RECLASSIFICATION ONLY APPLIES TO GROUP SYSTEM MESSAGES.
//
// An ordinary chat message that happened to carry a subtype must NOT become a
// group event: the carrier is "gp2", and dropping that condition would turn any
// message with a colliding subtype string into a group notification.
func TestOnlyGp2MessagesAreReclassified(t *testing.T) {
	rows := `{"seen":1,"dropped":0,"rows":[
	 {"type":"message.added","seq":1,"at":1700000000000,"chat":"1@g.us","msg":"M1",
	  "kind":"chat","subtype":"add","msgT":1700000000}]}`
	evs := decodeForTest(t, rows)
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	if evs[0].Type != MessageAdded {
		t.Fatalf("a chat message with subtype %q became %q", "add", evs[0].Type)
	}
	if evs[0].Subtype != "add" {
		t.Error("the subtype was dropped; a subscriber cannot see what arrived")
	}
}

// AND A GP2 MESSAGE WITH A MAPPED SUBTYPE DOES become the group event.
func TestAGroupSystemMessageBecomesItsEvent(t *testing.T) {
	for sub, want := range map[string]Type{
		"add": GroupJoined, "remove": GroupLeft,
		"promote": GroupAdminChanged, "membership_approval_mode": GroupUpdated,
	} {
		rows := `{"seen":1,"dropped":0,"rows":[
		 {"type":"message.added","seq":1,"at":1700000000000,"chat":"1@g.us","msg":"M1",
		  "kind":"gp2","subtype":"` + sub + `","msgT":1700000000}]}`
		evs := decodeForTest(t, rows)
		if len(evs) != 1 {
			t.Fatalf("subtype %q gave %d events", sub, len(evs))
		}
		if evs[0].Type != want {
			t.Errorf("subtype %q became %q, want %q", sub, evs[0].Type, want)
		}
	}
}

// AN UNMAPPED SUBTYPE STAYS VISIBLE. It must not vanish and must not be
// relabelled — it arrives as what the page called it, with its subtype intact.
func TestAnUnmappedGroupSubtypeStaysAMessage(t *testing.T) {
	rows := `{"seen":1,"dropped":0,"rows":[
	 {"type":"message.added","seq":1,"at":1700000000000,"chat":"1@g.us","msg":"M1",
	  "kind":"gp2","subtype":"meta_invents_this_next_year","msgT":1700000000}]}`
	evs := decodeForTest(t, rows)
	if len(evs) != 1 {
		t.Fatalf("an unmapped subtype produced %d events; it must not vanish", len(evs))
	}
	if evs[0].Type != MessageAdded {
		t.Errorf("an unmapped subtype was relabelled %q", evs[0].Type)
	}
	if evs[0].Subtype == "" {
		t.Error("the subtype was dropped, so nobody can tell what arrived")
	}
}

// THE PAGE SCRIPT MUST SEND THE SUBTYPE. The checks above feed rows straight in,
// so they cannot see whether the page actually projects the field — and if it
// does not, every group message arrives with an empty subtype and none of them
// classify.
func TestTheInstallScriptProjectsTheSubtype(t *testing.T) {
	code := installScriptForTest()
	if !strings.Contains(code, "m.subtype") {
		t.Fatal("the page projection never reads m.subtype; every group message " +
			"would arrive unclassifiable")
	}
	if !strings.Contains(code, "subtype:") {
		t.Error("the projected row has no subtype field")
	}
}

// THE VOTE LISTENER USES A CLEAN DOOR, NOT A PATCH.
//
// The reference monkey-patches pollVoteTableMode.bulkUpsert because it found no
// listener. This build exposes WAWebCollections.PollVote with .on (measured), so
// the install must use it — and must NOT contain the patching shape, which H112
// established as a thing this module does not do to somebody else's object.
func TestTheVoteListenerDoesNotPatchThePage(t *testing.T) {
	code := installScriptForTest()
	if !strings.Contains(code, "PollVote") {
		t.Fatal("the install never reaches the PollVote collection")
	}
	if !strings.Contains(code, "PV.on('add', onVote)") {
		t.Error("the vote listener is not installed through the collection's own on()")
	}
	bare := installCodeForTest()
	for _, patch := range []string{"bulkUpsert", "injectToFunction", "origFunction"} {
		if strings.Contains(bare, patch) {
			t.Errorf("the install contains %q, which is the reference's patching "+
				"technique and alters the running page", patch)
		}
	}
}

// A VOTE CARRIES CONTENT, AND THE CONTENT MUST NOT CROSS. The option a person
// selected is text the poll author wrote; routing needs the poll and the voter,
// nothing else.
func TestTheVoteRowCarriesNoOptionText(t *testing.T) {
	code := installCodeForTest()
	i := strings.Index(code, "const onVote")
	if i < 0 {
		t.Fatal("no vote handler in the install script")
	}
	seg := code[i:]
	if j := strings.Index(seg, "PV.on("); j > 0 {
		seg = seg[:j]
	}
	for _, forbidden := range []string{"selectedOptions", "options", "optionText", "name"} {
		if strings.Contains(seg, forbidden) {
			t.Errorf("the vote row projects %q, which is poll content", forbidden)
		}
	}
}

// AND THE LISTENER IS REMOVED ON UNINSTALL, like every other one — a handler
// left behind outlives the session that installed it.
func TestTheVoteListenerIsRegisteredForRemoval(t *testing.T) {
	code := installScriptForTest()
	if !strings.Contains(code, "s.handlers.push([PV, 'add', onVote]") {
		t.Fatal("the vote handlers are not registered in s.handlers, so uninstall " +
			"cannot take them off")
	}
}
