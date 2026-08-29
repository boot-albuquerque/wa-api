package channel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"wa-api/internal/headless/engine"
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
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
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
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
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
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
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

// reactionDouble keeps the server's WIRE value as its own state, so a test can
// make the page accept the call and the server keep the old policy — the failure
// the reference reports as success, since it computes its boolean from the
// absence of an exception.
type reactionDouble struct {
	serverWire int
	takes      bool

	answer  string
	scripts []string
}

func (d *reactionDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		*out = d.answer
		return nil
	}
	d.scripts = append(d.scripts, expr)
	if strings.Contains(expr, "editReactionCodesSetting") {
		if d.takes {
			d.serverWire = digitAfter(expr, "reactionCodesSetting: ")
		}
		d.answer = `{"ok":true}`
	} else {
		d.answer = `{"ok":true,"notFound":false,"jid":"1@newsletter","code":"c",` +
			`"name":"n","reactionRaw":` + itoa(d.serverWire) + `}`
	}
	*out = "kicked"
	return nil
}

func digitAfter(s, marker string) int {
	i := strings.Index(s, marker)
	if i < 0 {
		return -1
	}
	rest := s[i+len(marker):]
	n := 0
	seen := false
	for _, r := range rest {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
		seen = true
	}
	if !seen {
		return -1
	}
	return n
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func reactor(d *reactionDouble) *Manager { return NewManager(engine.NewRunner(), d.eval) }

// THE TWO VOCABULARIES MUST NOT BE COMPARED TO EACH OTHER.
//
// The reference's code and the page's wire value differ (0→3, 1→1, 2→0). A
// verification that compared the CODE against what the server reports would pass
// by accident for ReactionsBasic — where the two happen to coincide — and fail
// for the other two. That is the worst kind of bug: right sometimes.
func TestTheReactionPolicyIsVerifiedAgainstTheWireValue(t *testing.T) {
	d := &reactionDouble{serverWire: 0, takes: true}
	got, err := reactor(d).SetReactionPolicy(context.Background(), "1@newsletter", "c",
		ReactionsAll, "t")
	if err != nil {
		t.Fatalf("SetReactionPolicy: %v", err)
	}
	if got != 3 {
		t.Fatalf("the server holds %d; ReactionsAll maps to wire 3, not to its own code", got)
	}
}

// A POLICY THE SERVER IGNORED IS NOT A SUCCESS.
func TestAReactionPolicyTheServerIgnoredIsAnError(t *testing.T) {
	d := &reactionDouble{serverWire: 0, takes: false}
	_, err := reactor(d).SetReactionPolicy(context.Background(), "1@newsletter", "c",
		ReactionsAll, "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "wire value") {
		t.Errorf("the error does not name the vocabulary it compared: %v", err)
	}
}

// A VALUE OUTSIDE THE THREE NEVER REACHES THE PAGE.
func TestAnUnknownReactionPolicyIsRefusedBeforeThePage(t *testing.T) {
	d := &reactionDouble{}
	if _, err := reactor(d).SetReactionPolicy(context.Background(), "1@newsletter", "c",
		ReactionPolicy(7), "t"); !errors.Is(err, ErrBadReactionPolicy) {
		t.Fatalf("err = %v, want ErrBadReactionPolicy", err)
	}
	if len(d.scripts) != 0 {
		t.Error("an unknown policy reached the page")
	}
}

// THE FLAG AND THE VALUE KEY ARE BOTH RIGHT. They differ, and getting either
// wrong reads as "the server ignored us".
func TestTheReactionScriptUsesBothCorrectKeys(t *testing.T) {
	d := &reactionDouble{takes: true}
	_, _ = reactor(d).SetReactionPolicy(context.Background(), "1@newsletter", "c",
		ReactionsNone, "t")
	var script string
	for _, s := range d.scripts {
		if strings.Contains(s, "editReactionCodesSetting") {
			script = s
		}
	}
	if script == "" {
		t.Fatal("no reaction script ran")
	}
	if !strings.Contains(script, "{ editReactionCodesSetting: true }") {
		t.Error("the property flag is not set as its own object")
	}
	if !strings.Contains(script, "reactionCodesSetting: 0") {
		t.Error("ReactionsNone must send wire 0")
	}
}

// "THE FIELD IS ABSENT" IS NOT "THE WRITE FAILED".
//
// A freshly created channel's metadata carries no reaction mixin at all
// (measured, H133). Reporting that as ErrNotTaken would claim the write failed
// when nobody knows — the opposite of what invariant 14 is for. This module has
// conflated absent with wrong twice before: acks (H108) and descriptions (H126).
func TestAnAbsentReactionFieldIsUnverifiableAndNotAFailure(t *testing.T) {
	d := &reactionDouble{serverWire: reactionAbsent, takes: false}
	_, err := reactor(d).SetReactionPolicy(context.Background(), "1@newsletter", "c",
		ReactionsAll, "t")
	if !errors.Is(err, ErrUnverifiable) {
		t.Fatalf("err = %v, want ErrUnverifiable", err)
	}
	if errors.Is(err, ErrNotTaken) {
		t.Error("an unverifiable write is being reported as a failed one")
	}
}

// THE FOLLOW LIST IS THE CACHE, AND THE DOC MUST SAY SO.
//
// H139 measured six stale models across the two lab accounts, all reporting
// serverAlive:false. A caller reading this list as "what exists on the server"
// counts channels that do not, and the only defence a library has against that
// is saying it where the caller looks.
func TestTheFollowedDocWarnsThatItIsTheCache(t *testing.T) {
	src, err := os.ReadFile("owner.go")
	if err != nil {
		t.Fatalf("reading owner.go: %v", err)
	}
	doc := string(src)
	i := strings.Index(doc, "func (m *Manager) Followed(")
	if i < 0 {
		t.Fatal("Followed is gone")
	}
	// The doc comment is what precedes the declaration.
	head := doc[:i]
	j := strings.LastIndex(head, "// Followed lists")
	if j < 0 {
		t.Fatal("Followed has no doc comment")
	}
	block := head[j:]
	for _, must := range []string{"CACHE", "serverAlive"} {
		if !strings.Contains(block, must) {
			t.Errorf("the Followed doc does not mention %q; a caller reading this "+
				"list as the server's truth would count channels that do not exist", must)
		}
	}
}
