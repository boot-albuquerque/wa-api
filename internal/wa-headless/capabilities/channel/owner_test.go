package channel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// ownerDouble answers BOTH the write and the verifying read, and it keeps them
// as SEPARATE state on purpose: that is the only way a test can make the page
// accept a write and the server still report the old value, which is the exact
// failure the reference cannot report at all.
type ownerDouble struct {
	// serverName and serverDesc are what a read reports. A write does NOT touch
	// them unless writeTakes is true.
	serverName string
	serverDesc string
	writeTakes bool
	// gone makes the verifying read fail, which is what a deleted channel does.
	gone bool
	// creationDisabled reproduces the gate being off.
	creationDisabled bool
	// createdCode is what the create call reports back.
	createdCode string

	answer      string
	lastScripts []string
}

func (d *ownerDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = d.answer
		return nil
	}
	d.lastScripts = append(d.lastScripts, expr)
	switch {
	case strings.Contains(expr, "createNewsletterQuery"):
		if d.creationDisabled {
			d.answer = `{"ok":false,"disabled":true}`
		} else {
			d.answer = `{"ok":true,"disabled":false,"jid":"1@newsletter","code":"` +
				d.createdCode + `","at":1700000000}`
		}
	case strings.Contains(expr, "editNewsletterMetadataAction"):
		if d.writeTakes {
			if v := between(expr, `value["name"] = "`, `"`); v != "" {
				d.serverName = v
			}
			if strings.Contains(expr, `"editDescription"`) {
				d.serverDesc = between(expr, `value["description"] = "`, `"`)
			}
		}
		d.answer = `{"ok":true}`
	case strings.Contains(expr, "deleteNewsletterAction"):
		d.gone = true
		d.answer = `{"ok":true}`
	default: // the verifying read, byInviteScript
		if d.gone {
			d.answer = `{"ok":true,"notFound":true}`
		} else {
			d.answer = `{"ok":true,"notFound":false,"jid":"1@newsletter","code":"c",` +
				`"name":"` + d.serverName + `","description":"` + d.serverDesc + `"}`
		}
	}
	*out = "kicked"
	return nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

func owner(d *ownerDouble) *Manager { return NewManager(engine.NewRunner(), d.eval) }

// A WRITE THE SERVER IGNORED IS NOT A SUCCESS.
//
// The reference returns true from setSubject having only seen the call not
// throw. This reads the channel back and fails, and the double keeps the server
// value separate precisely so that case can exist.
func TestARenameTheServerIgnoredIsAnError(t *testing.T) {
	d := &ownerDouble{serverName: "old", writeTakes: false}
	err := owner(d).SetName(context.Background(), "1@newsletter", "code", "new", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "old value") {
		t.Errorf("the error does not say what happened: %v", err)
	}
}

func TestARenameThatTakesSucceeds(t *testing.T) {
	d := &ownerDouble{serverName: "old", writeTakes: true}
	if err := owner(d).SetName(context.Background(), "1@newsletter", "code", "new", "t"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	if d.serverName != "new" {
		t.Fatalf("the server holds %q", d.serverName)
	}
}

// THE FLAG AND THE VALUE KEY DIFFER, and getting that wrong reads as "the server
// ignored us" rather than as a bug. editName is the flag; name is the key.
func TestTheEditFlagAndTheValueKeyAreBothCorrect(t *testing.T) {
	d := &ownerDouble{writeTakes: true}
	if err := owner(d).SetName(context.Background(), "1@newsletter", "code", "x", "t"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	var editScript string
	for _, s := range d.lastScripts {
		if strings.Contains(s, "editNewsletterMetadataAction") {
			editScript = s
		}
	}
	if !strings.Contains(editScript, `property["`+editName+`"] = true`) {
		t.Errorf("the script does not set the %s flag", editName)
	}
	if !strings.Contains(editScript, `value["name"]`) {
		t.Error("the script does not put the value under the key the action expects")
	}
}

// AN EMPTY DESCRIPTION IS ALLOWED. Clearing one is legitimate, and refusing it
// would make this the only setting a caller cannot undo.
func TestAnEmptyDescriptionIsAllowedAndAnEmptyNameIsNot(t *testing.T) {
	d := &ownerDouble{serverDesc: "something", writeTakes: true}
	if err := owner(d).SetDescription(context.Background(), "1@newsletter", "code", "", "t"); err != nil {
		t.Fatalf("clearing a description was refused: %v", err)
	}
	if err := owner(d).SetName(context.Background(), "1@newsletter", "code", "  ", "t"); !errors.Is(err, ErrNoName) {
		t.Fatalf("err = %v, want ErrNoName: a channel with no name is not a rename", err)
	}
}

// CREATION IS VERIFIED BY READING THE CHANNEL BACK, not by trusting the echo.
func TestACreateThatCannotBeReadBackIsAnError(t *testing.T) {
	d := &ownerDouble{createdCode: "abc", gone: true} // the read will not find it
	_, err := owner(d).Create(context.Background(), "test", "", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
}

func TestACreateWithoutACodeCannotBeVerified(t *testing.T) {
	d := &ownerDouble{createdCode: ""}
	_, err := owner(d).Create(context.Background(), "test", "", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "nothing to verify") {
		t.Errorf("the error does not say why: %v", err)
	}
}

// THE GATE IS ITS OWN ERROR. The reference returns the string
// 'CreateChannelError: A channel creation is not enabled' for a caller to
// pattern-match.
func TestCreationBeingDisabledIsItsOwnError(t *testing.T) {
	d := &ownerDouble{creationDisabled: true}
	_, err := owner(d).Create(context.Background(), "test", "", "t")
	if !errors.Is(err, ErrCreationDisabled) {
		t.Fatalf("err = %v, want ErrCreationDisabled", err)
	}
	if errors.Is(err, ErrWrite) {
		t.Error("a disabled gate is being reported as a page refusal")
	}
}

func TestAnEmptyNameNeverReachesThePage(t *testing.T) {
	d := &ownerDouble{}
	if _, err := owner(d).Create(context.Background(), "   ", "", "t"); !errors.Is(err, ErrNoName) {
		t.Fatalf("err = %v, want ErrNoName", err)
	}
	if len(d.lastScripts) != 0 {
		t.Error("an empty name reached the page")
	}
}

// A DELETE THAT LEFT THE CHANNEL READABLE IS A FAILURE.
func TestADeleteThatDidNotDeleteIsAnError(t *testing.T) {
	d := &ownerDouble{}
	// The delete script sets gone; force it back so the verify still finds it.
	d.answer = ""
	m := owner(d)
	if err := m.Delete(context.Background(), "1@newsletter", "code", "t"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stubborn := &stubbornDouble{}
	if err := NewManager(engine.NewRunner(), stubborn.eval).Delete(context.Background(),
		"1@newsletter", "code", "t"); !errors.Is(err, ErrStillThere) {
		t.Fatalf("err = %v, want ErrStillThere", err)
	}
}

type stubbornDouble struct{ answer string }

func (d *stubbornDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = d.answer
		return nil
	}
	if strings.Contains(expr, "deleteNewsletterAction") {
		d.answer = `{"ok":true}`
	} else {
		// The channel is STILL THERE after the delete.
		d.answer = `{"ok":true,"notFound":false,"jid":"1@newsletter","code":"c","name":"n"}`
	}
	*out = "kicked"
	return nil
}

// A DELETE WITHOUT A CODE CANNOT BE VERIFIED, and saying so beats reporting
// success.
func TestADeleteWithoutACodeSaysItCouldNotCheck(t *testing.T) {
	d := &ownerDouble{}
	err := owner(d).Delete(context.Background(), "1@newsletter", "", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "could not be checked") {
		t.Errorf("the error does not say why: %v", err)
	}
}

// The lookup does not use the reference's injected helper, which this stack does
// not inject.
func TestTheLookupDoesNotUseTheInjectedHelper(t *testing.T) {
	d := &ownerDouble{writeTakes: true}
	_ = owner(d).SetName(context.Background(), "1@newsletter", "code", "x", "t")
	var found bool
	for _, s := range d.lastScripts {
		if strings.Contains(s, "WWebJS") {
			t.Fatal("a script calls window.WWebJS, which this stack never injects")
		}
		if strings.Contains(s, "editNewsletterMetadataAction") {
			found = true
			// AND IT MUST LOOK IN THE NEWSLETTER COLLECTION. Asking
			// WAWebChatCollection is what the first version did, and it holds 384
			// chats and ZERO newsletters on this account: every owner operation
			// failed with "channel not loaded" against a channel that had just
			// been created successfully (H113).
			if !strings.Contains(s, collNewsletters) {
				t.Errorf("the script does not look in %s", collNewsletters)
			}
			if strings.Contains(s, "ChatCollection") {
				t.Error("the script looks in ChatCollection, which holds no newsletters")
			}
		}
	}
	if !found {
		t.Fatal("no edit script was recorded")
	}
}

// followDouble answers the follow/unfollow scripts and keeps the membership as
// its OWN state, so a test can make the page accept the call and leave the
// membership where it was — which is the failure the reference cannot report,
// because it returns true for merely not throwing.
type followDouble struct {
	membership string
	// takes says whether the page actually moves the membership.
	takes bool
	// unreachable reproduces a channel the collection cannot fetch.
	unreachable bool

	answer  string
	scripts []string
}

func (d *followDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = d.answer
		return nil
	}
	d.scripts = append(d.scripts, expr)
	switch {
	case strings.Contains(expr, "subscribeToNewsletterAction"),
		strings.Contains(expr, "unsubscribeFromNewsletterAction"):
		if d.unreachable {
			d.answer = `{"ok":false,"why":"channel not reachable"}`
			break
		}
		if d.membership == "owner" {
			d.answer = `{"ok":false,"owner":true}`
			break
		}
		if d.takes {
			if strings.Contains(expr, "subscribeToNewsletterAction") {
				d.membership = "subscriber"
			} else {
				d.membership = membershipGuest
			}
		}
		d.answer = `{"ok":true,"membership":"` + d.membership + `"}`
	default:
		d.answer = `{"ok":true,"results":[]}`
	}
	*out = "kicked"
	return nil
}

func follower(d *followDouble) *Manager { return NewManager(engine.NewRunner(), d.eval) }

// A SUBSCRIPTION THE PAGE IGNORED IS NOT A SUCCESS.
func TestAFollowThatLeftTheAccountAGuestIsAnError(t *testing.T) {
	d := &followDouble{membership: membershipGuest, takes: false}
	_, err := follower(d).Follow(context.Background(), "1@newsletter", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "still a guest") {
		t.Errorf("the error does not say what happened: %v", err)
	}
}

func TestAFollowThatTakesReportsTheNewMembership(t *testing.T) {
	d := &followDouble{membership: membershipGuest, takes: true}
	got, err := follower(d).Follow(context.Background(), "1@newsletter", "t")
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if got == membershipGuest || got == "" {
		t.Fatalf("membership = %q; a successful follow must leave a real one", got)
	}
}

// AND THE POSTCONDITION RUNS IN THE OTHER DIRECTION TOO. An unfollow that left
// a real membership is just as much a silent success.
func TestAnUnfollowThatDidNotUnfollowIsAnError(t *testing.T) {
	d := &followDouble{membership: "subscriber", takes: false}
	_, err := follower(d).Unfollow(context.Background(), "1@newsletter", "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	d2 := &followDouble{membership: "subscriber", takes: true}
	if _, err := follower(d2).Unfollow(context.Background(), "1@newsletter", "t"); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
}

// OWNING IS ITS OWN ERROR, not a generic refusal: the repair is different and
// the page's answer for it looks like failure.
func TestSubscribingToOwnChannelIsItsOwnError(t *testing.T) {
	d := &followDouble{membership: "owner"}
	_, err := follower(d).Follow(context.Background(), "1@newsletter", "t")
	if !errors.Is(err, ErrOwnChannel) {
		t.Fatalf("err = %v, want ErrOwnChannel", err)
	}
	if errors.Is(err, ErrWrite) || errors.Is(err, ErrNotTaken) {
		t.Error("owning a channel is being reported as a page failure")
	}
}

func TestAnUnreachableChannelIsItsOwnError(t *testing.T) {
	d := &followDouble{membership: membershipGuest, unreachable: true}
	if _, err := follower(d).Follow(context.Background(), "1@newsletter", "t"); !errors.Is(err, ErrNotReachable) {
		t.Fatalf("err = %v, want ErrNotReachable", err)
	}
}

// THE SCRIPT MUST FETCH, NOT ONLY LOOK UP. A channel this account does not
// follow is NOT in the collection — measured empty — so `get` alone finds
// nothing and the action would have nothing to act on.
func TestTheFollowScriptFetchesAChannelItDoesNotHave(t *testing.T) {
	d := &followDouble{membership: membershipGuest, takes: true}
	if _, err := follower(d).Follow(context.Background(), "1@newsletter", "t"); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	var script string
	for _, s := range d.scripts {
		if strings.Contains(s, "subscribeToNewsletterAction") {
			script = s
		}
	}
	if !strings.Contains(script, "NC.find(") {
		t.Fatal("the script never fetches the channel; a channel this account does " +
			"not follow is absent from the collection and get() alone finds nothing")
	}
	// AND IT MUST PASS A WID, NOT THE JID STRING. Measured (H123): the bare jid
	// gave "channel not reachable" against a channel the metadata query read
	// without trouble. This assertion is what stops that regressing silently —
	// the string form fails at RUNTIME, where no unit test would see it.
	if !strings.Contains(script, "createWid(JID)") {
		t.Error("the script passes the raw jid to find; this build needs a Wid")
	}
	if strings.Contains(script, "WWebJS") {
		t.Error("the script uses the reference's injected helper, which this stack " +
			"never injects")
	}
}

// An empty follow list is an answer, not an error.
func TestAnEmptyFollowListIsNotAnError(t *testing.T) {
	d := &followDouble{}
	got, err := follower(d).Followed(context.Background(), "t")
	if err != nil {
		t.Fatalf("Followed: %v", err)
	}
	if got == nil {
		t.Error("an empty list came back nil rather than empty")
	}
}

func TestAnEmptyJIDNeverReachesThePageForAFollow(t *testing.T) {
	d := &followDouble{}
	if _, err := follower(d).Follow(context.Background(), " ", "t"); !errors.Is(err, ErrNoJID) {
		t.Fatalf("err = %v, want ErrNoJID", err)
	}
	if len(d.scripts) != 0 {
		t.Error("an empty jid reached the page")
	}
}
