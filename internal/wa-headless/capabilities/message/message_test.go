package message

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

type double struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		d.reads++
		if d.reads <= d.pendingReads {
			*out = ""
			return nil
		}
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func rd(d *double) *Reader { return New(engine.NewRunner(), d.eval) }

func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// An empty id never reaches the page.
func TestAnEmptyMessageIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).OriginOf(context.Background(), "  ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Errorf("err = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty id reached the page")
	}
}

// THE SENDER OF A GROUP MESSAGE IS NOT THE CHAT.
//
// In a one-to-one the participant is null and the sender IS the chat; in a group
// the participant is who spoke. Conflating them attributes the speech to the
// group, which is the whole reason Origin has two fields.
func TestTheGroupSenderIsNotTheChat(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"120-1@g.us","group":true,` +
		`"sender":"5541999999999@lid","fromMe":false,"t":1700000000}`}
	got, err := rd(d).OriginOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("OriginOf: %v", err)
	}
	if got.SenderJID == got.ChatJID {
		t.Fatal("the sender equals the chat on a group message")
	}
	if got.SenderIsChat {
		t.Error("SenderIsChat is true for a group message")
	}
	if !got.IsGroup {
		t.Error("the group flag was lost")
	}
	if got.At.IsZero() {
		t.Error("the timestamp was dropped")
	}

	// AND THE SCRIPT MUST DERIVE IT, which the check above cannot see.
	//
	// The double supplies `sender` ready-made, so replacing the whole derivation
	// with `sender = chat` left this test green — a negative control that did
	// not bite, and the third time today that a check asserted on the PARSING
	// while the property lived in the PRODUCTION.
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "id.participant") {
		t.Fatal("the script does not read the participant; in a group the sender " +
			"would collapse into the chat")
	}
}

// And in a one-to-one they legitimately coincide, which is recorded rather than
// hidden — a caller comparing them should not have to.
func TestInAOneToOneTheSenderIsTheChat(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"1@lid","group":false,"sender":"1@lid"}`}
	got, err := rd(d).OriginOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("OriginOf: %v", err)
	}
	if !got.SenderIsChat {
		t.Fatal("SenderIsChat is false when they are the same jid")
	}
}

// GROUP IS ASKED, NOT INFERRED FROM THE SUFFIX. Inferring from "@g.us" is one
// build change away from being wrong, and this build has already changed its
// identity namespace once (LID).
func TestTheGroupFlagIsAskedOfThePage(t *testing.T) {
	d := &double{answer: `{"ok":true,"chat":"1@g.us"}`}
	_, _ = rd(d).OriginOf(context.Background(), "3EB0", "t")
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "getIsGroup") {
		t.Error("the script does not ask the page whether the chat is a group")
	}
}

// A message this session has not loaded is its own error, not a read failure:
// loading is the caller's job and the repairs differ.
func TestAnUnloadedMessageIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).OriginOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	d2 := &double{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	_, err := rd(d2).OriginOf(context.Background(), "3EB0", "t")
	if !errors.Is(err, ErrRead) || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrRead and not ErrNotFound", err)
	}
}

// The lookup falls back to a scan by RAW id, because that is the id every other
// capability in this module reports and therefore the only one a caller has.
func TestTheLookupScansByTheRawID(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	_, _ = rd(d).OriginOf(context.Background(), "3EB0", "t")
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "c.id.id ===") {
		t.Error("the script does not scan by the raw message id")
	}
}

// The rendering carries no identity.
func TestTheOriginRenderingIsQuiet(t *testing.T) {
	o := Origin{ChatJID: "120-1@g.us", SenderJID: "5541999999999@lid", IsGroup: true}
	s := o.String()
	if strings.Contains(s, "5541") || strings.Contains(s, "120-1") {
		t.Errorf("Origin.String carries identity: %s", s)
	}
}

// The parked loop is bounded.
func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{pendingReads: 1 << 30}
	if _, err := rd(d).OriginOf(context.Background(), "3EB0", "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

// THE SHAPE READER MUST NOT BE ABLE TO CARRY A BODY.
//
// This is the guard that makes the divergence honest rather than a rename. A
// version that read m[k] — even only to decide whether a field is empty — would
// already have the body in hand, and one JSON.stringify later it would be in
// Go. So the check is on the SCRIPT: the value side is never touched.
func TestTheShapeReaderNeverReadsAValue(t *testing.T) {
	d := &double{answer: `{"ok":true,"keys":["id","t","type"]}`}
	if _, err := rd(d).ShapeOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("ShapeOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	for _, forbidden := range []string{"m[k]", "o[k]", "Object.values", "Object.entries", "JSON.stringify(m"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the shape script contains %q, which reads the value side", forbidden)
		}
	}
	if !strings.Contains(code, "Object.keys") {
		t.Error("the shape script does not read keys at all")
	}
}

// And the returned type has nowhere to put one: []string of names.
func TestTheShapeIsNamesAndIsSorted(t *testing.T) {
	d := &double{answer: `{"ok":true,"keys":["a","b","c"]}`}
	got, err := rd(d).ShapeOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("ShapeOf: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("keys = %d, want 3", len(got))
	}
	if !strings.Contains(withoutComments(d.lastScript), ".sort()") {
		t.Error("the script does not sort, so two reads of one message will not compare")
	}
}

func TestTheShapeOfAnUnloadedMessageIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).ShapeOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAnEmptyIDNeverReachesThePageForAShape(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).ShapeOf(context.Background(), " ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("err = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty id reached the page")
	}
}

// ABSENT AND ZERO ARE DIFFERENT ANSWERS.
//
// Measured: of 395 loaded messages, 25 read ack 0 and 5 carried no ack field at
// all (H108). A reader that merged them would say "this message never left"
// about a message the page never spoke about — which is the same mistake the
// event freshness work made once with AgeSeconds, and H90 made with device
// counts.
func TestAnAbsentAckIsNotAZeroAck(t *testing.T) {
	absent := &double{answer: `{"ok":true,"hasAck":false,"ack":0}`}
	gotAbsent, err := rd(absent).CurrentOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("CurrentOf: %v", err)
	}
	zero := &double{answer: `{"ok":true,"hasAck":true,"ack":0}`}
	gotZero, err := rd(zero).CurrentOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("CurrentOf: %v", err)
	}
	if gotAbsent == gotZero {
		t.Fatal("a message with no ack reads identically to one the page says is at " +
			"ack 0; the two cannot be told apart by a caller")
	}
	if gotAbsent.HasAck {
		t.Error("HasAck is true when the page reported no ack")
	}
	if !gotZero.HasAck || gotZero.Ack != 0 {
		t.Error("a real ack 0 lost either its value or its presence")
	}

	// And the SCRIPT must be what makes the distinction, which the check above
	// cannot see: the double supplies hasAck ready-made.
	if !strings.Contains(withoutComments(absent.lastScript), `typeof m.ack === "number"`) {
		t.Error("the script does not test for the ack's presence, so absent and zero " +
			"collapse before Go ever sees them")
	}
}

// THIS PACKAGE DOES NOT OWN THE REVOKED VOCABULARY.
//
// capabilities/revoke already decides what revoked means (isRevokedMsg,
// type === 'revoked', revokeSender). A second copy here would be two lists free
// to drift, which ADR-0004 and this repo's string-literal rule both exist to
// prevent. The type is passed through raw instead.
func TestTheRevokedVocabularyIsNotDuplicatedHere(t *testing.T) {
	d := &double{answer: `{"ok":true,"type":"revoked"}`}
	got, err := rd(d).CurrentOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("CurrentOf: %v", err)
	}
	if got.Type != "revoked" {
		t.Fatalf("type = %q; the page's own word must survive the trip", got.Type)
	}
	code := withoutComments(d.lastScript)
	for _, owned := range []string{"isRevokedMsg", "revokeSender"} {
		if strings.Contains(code, owned) {
			t.Errorf("this script names %q, which capabilities/revoke owns", owned)
		}
	}
}

func TestAReloadOfAGoneMessageIsErrNotFound(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).CurrentOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAnEmptyIDNeverReachesThePageForAReload(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).CurrentOf(context.Background(), " ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("err = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty id reached the page")
	}
}

// A MESSAGE THAT QUOTES NOTHING IS NOT AN ERROR. Most messages quote nothing —
// measured: 336 loaded messages, ZERO with a quoted id — so treating the absence
// as a failure would make the common case look broken.
func TestAMessageThatQuotesNothingIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":false,"quotes":false}`}
	got, err := rd(d).QuotedOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("QuotedOf: %v", err)
	}
	if got.Quotes {
		t.Error("a message with no quoted id reported a quote")
	}
	if got.MessageID != "" {
		t.Error("an id came back for a message that quotes nothing")
	}
}

// "QUOTES X" AND "X IS LOADED" ARE DIFFERENT FACTS.
//
// Merging them would say "no quote" about a reply whose target simply has not
// hydrated — a lie about the MESSAGE rather than about the session. The
// distinction is the same one H108 had to make between an absent ack and an ack
// of zero.
func TestQuotingAndBeingLoadedAreSeparateFacts(t *testing.T) {
	d := &double{answer: `{"ok":true,"quotes":true,"quotedId":"ORIG1","quotedLoaded":false}`}
	got, err := rd(d).QuotedOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("QuotedOf: %v", err)
	}
	if !got.Quotes {
		t.Fatal("a message with a quoted id reported no quote")
	}
	if got.Loaded {
		t.Error("an unloaded target was reported as loaded")
	}
	if got.MessageID != "ORIG1" {
		t.Errorf("the quoted id was lost: %q", got.MessageID)
	}
}

// THE SCRIPT MUST READ quotedStanzaID, and MUST NOT read the sentinel fields.
//
// Every message model carries __x_fromQuotedMsg, __x_isQuotedMsgAvailable and
// __x_questionReplyQuotedMessage, and all three hold a LAZY SENTINEL rather than
// data — measured on all 110 of a hydration. A reader that trusted them would
// report every message as quoting something, which is exactly what a first,
// wrong instrument reported (H131).
func TestTheQuotedScriptReadsTheFieldThatCarriesTheReference(t *testing.T) {
	d := &double{answer: `{"ok":true,"quotes":false}`}
	if _, err := rd(d).QuotedOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("QuotedOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "quotedStanzaID") {
		t.Fatal("the script does not read quotedStanzaID, which is the field that " +
			"actually carries the reference and the one send uses to prove a reply")
	}
	for _, sentinel := range []string{
		"fromQuotedMsg", "isQuotedMsgAvailable", "questionReplyQuotedMessage",
	} {
		if strings.Contains(code, sentinel) {
			t.Errorf("the script reads %q, which holds a lazy sentinel on every "+
				"message and would count them all as quoting", sentinel)
		}
	}
}

func TestAQuotedLookupOfAnUnloadedMessageIsErrNotFound(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).QuotedOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAnEmptyIDNeverReachesThePageForAQuote(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).QuotedOf(context.Background(), " ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("err = %v, want ErrNoMessage", err)
	}
	if d.kicks != 0 {
		t.Error("an empty id reached the page")
	}
}

// THE SENTINEL IS NOT READ. This is the assertion that would have caught H131
// the first time, and it is asserted on the SCRIPT rather than on the result
// because the double supplies whatever the parser is asked to parse — a result
// assertion here would measure the double's honesty, not the rule.
func TestTheMentionsScriptDoesNotReadTheSentinelField(t *testing.T) {
	d := &double{answer: `{"ok":true,"people":[],"groups":[]}`}
	if _, err := rd(d).MentionsOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("MentionsOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	if strings.Contains(code, "nonJidMentions") {
		t.Fatal("the script reads nonJidMentions, which is not an array and was " +
			"measured as present on 46 of 62 messages including ones that mention " +
			"nobody; reading it counts every message as mentioning somebody")
	}
	for _, field := range []string{"mentionedJidList", "groupMentions"} {
		if !strings.Contains(code, field) {
			t.Errorf("the script does not read %q, which is one of the two fields "+
				"measured to actually carry a mention", field)
		}
	}
}

// A NON-ARRAY IS NOT A MENTION. The shape guard has to live in the page, since
// that is the only side that ever sees the sentinel — by the time a value has
// crossed into Go it is already JSON and already an array or not.
func TestTheMentionsScriptTakesOnlyArrays(t *testing.T) {
	d := &double{answer: `{"ok":true,"people":[],"groups":[]}`}
	if _, err := rd(d).MentionsOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("MentionsOf: %v", err)
	}
	if !strings.Contains(withoutComments(d.lastScript), "Array.isArray") {
		t.Fatal("the script does not check Array.isArray before iterating, so a " +
			"lazy sentinel would be walked as if it were a list of mentions")
	}
}

// People and groups are separate facts with separate shapes.
func TestPeopleAndGroupsDoNotShareAList(t *testing.T) {
	d := &double{answer: `{"ok":true,"people":["1@lid"],` +
		`"groups":[{"jid":"9@g.us","subject":"lab"}]}`}
	m, err := rd(d).MentionsOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("MentionsOf: %v", err)
	}
	if len(m.People) != 1 || m.People[0] != "1@lid" {
		t.Fatalf("people: %#v", m.People)
	}
	if len(m.Groups) != 1 || m.Groups[0].JID != "9@g.us" || m.Groups[0].Subject != "lab" {
		t.Fatalf("groups: %#v", m.Groups)
	}
	if !m.Any() {
		t.Fatal("Any() said nothing was mentioned about a message with both")
	}
}

// A message mentioning nobody is not an error, and Any() says so.
func TestMentioningNobodyIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"people":[],"groups":[]}`}
	m, err := rd(d).MentionsOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("a message with no mentions must not be an error: %v", err)
	}
	if m.Any() {
		t.Fatal("Any() claimed a mention on a message that carries none")
	}
}

// The rendering carries counts, never an identity and never a group name.
func TestTheMentionsRenderingIsQuiet(t *testing.T) {
	m := Mentions{
		People: []string{"5516999999999@lid"},
		Groups: []GroupMention{{JID: "120363@g.us", Subject: "familia"}},
	}
	s := m.String()
	for _, leak := range []string{"5516", "999999999", "120363", "familia"} {
		if strings.Contains(s, leak) {
			t.Fatalf("the rendering leaks %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, "people=1") || !strings.Contains(s, "groups=1") {
		t.Fatalf("the rendering lost the counts: %s", s)
	}
}

func TestAnUnloadedMessageHasNoMentionsAnswer(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).MentionsOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAnEmptyIDNeverReachesThePageForMentions(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).MentionsOf(context.Background(), "  ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("want ErrNoMessage, got %v", err)
	}
	if d.kicks != 0 {
		t.Fatalf("an empty id reached the page %d times", d.kicks)
	}
}

// SOMEBODY ELSE'S MESSAGE IS ITS OWN ANSWER. Folding it into ErrRead would tell
// a caller the page refused, when in fact the question has no meaning.
func TestInfoAboutSomebodyElsesMessageIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":false,"notMine":true}`}
	if _, err := rd(d).InfoOf(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNotMine) {
		t.Fatalf("want ErrNotMine, got %v", err)
	}
}

// THE SCRIPT REFUSES BEFORE ASKING, like the reference does. Asking the store
// about a message this account did not send is a question the server has no
// answer for, and the guard has to be in the page because that is where the
// message model is.
func TestTheInfoScriptChecksOwnershipBeforeQuerying(t *testing.T) {
	d := &double{answer: `{"ok":true,"answered":false}`}
	if _, err := rd(d).InfoOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("InfoOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	iGuard := strings.Index(code, "fromMe")
	iQuery := strings.Index(code, "queryMsgInfo")
	if iGuard < 0 || iQuery < 0 {
		t.Fatalf("script is missing the guard or the query (guard=%d query=%d)", iGuard, iQuery)
	}
	if iGuard > iQuery {
		t.Fatal("the ownership guard runs AFTER the query, so a message this " +
			"account did not send would still be asked about")
	}
}

// THE COLLECTION IS NOT THE SOURCE. H71 measured WAWebMsgInfoCollection empty
// and concluded the build has no read receipts; it is still empty and it is not
// where the answer lives.
func TestTheInfoScriptDoesNotReadTheEmptyCollection(t *testing.T) {
	d := &double{answer: `{"ok":true,"answered":false}`}
	if _, err := rd(d).InfoOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("InfoOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "queryMsgInfo") {
		t.Fatal("the script does not call queryMsgInfo, which is the only thing " +
			"that produces message info on this build")
	}
	if strings.Contains(code, "MsgInfoCollection") {
		t.Fatal("the script reads MsgInfoCollection, which is populated BY the " +
			"query and reads empty until somebody makes it")
	}
}

// AN UNANSWERED QUERY IS NOT "NOBODY GOT IT". A message too young for the server
// to report on would otherwise come back as three empty lists, which a caller
// reads as delivery failure.
func TestAnUnansweredInfoIsNotThreeEmptyLists(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":false,"notMine":false,"answered":false}`}
	got, err := rd(d).InfoOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("InfoOf: %v", err)
	}
	if got.Answered {
		t.Fatal("an unanswered query reported itself as answered")
	}
	if !strings.Contains(got.String(), "answered=false") {
		t.Fatalf("the rendering hides that nothing was answered: %s", got)
	}
}

// THE REMAINING COUNTS ARE THE PAGE'S, NOT DERIVED. Deriving them would need the
// participant count at send time, which is not the group's size today.
func TestTheRemainingCountsComeFromThePage(t *testing.T) {
	d := &double{answer: `{"ok":true,"answered":true,"delivered":["1@lid"],` +
		`"read":[],"played":[],"deliveredRemaining":4,"readRemaining":5,"playedRemaining":-1}`}
	got, err := rd(d).InfoOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("InfoOf: %v", err)
	}
	if got.DeliveredRemaining != 4 || got.ReadRemaining != 5 {
		t.Fatalf("the remaining counts were not carried through: %#v", got)
	}
	if got.PlayedRemaining != -1 {
		t.Fatal("an absent count was turned into zero; -1 is how this type says " +
			"the page reported nothing, and zero would mean everybody played it")
	}
	if len(got.Delivered) != 1 || got.Delivered[0] != "1@lid" {
		t.Fatalf("the delivered list did not come back whole: %#v", got.Delivered)
	}
}

// The rendering carries counts, never an identity.
func TestTheInfoRenderingIsQuiet(t *testing.T) {
	i := Info{Answered: true, Delivered: []string{"5516999999999@lid"}, ReadRemaining: 3}
	s := i.String()
	for _, leak := range []string{"5516", "999999999"} {
		if strings.Contains(s, leak) {
			t.Fatalf("the rendering leaks %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, "delivered=1") {
		t.Fatalf("the rendering lost the counts: %s", s)
	}
}

func TestAnEmptyIDNeverReachesThePageForInfo(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).InfoOf(context.Background(), " ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("want ErrNoMessage, got %v", err)
	}
	if d.kicks != 0 {
		t.Fatalf("an empty id reached the page %d times", d.kicks)
	}
}

// THE KEY IS THE ID OBJECT. Measured side by side on a message carrying a
// reaction: find(_serialized) threw "called find without an id" because
// _serialized is null on this build, find(id.id) returned null, and find(id)
// returned the record. Passing the reference's key here would fail on every
// message forever, silently reporting that nothing has reactions.
func TestTheReactionsScriptKeysByTheIdObject(t *testing.T) {
	d := &double{answer: `{"ok":true,"groups":[]}`}
	if _, err := rd(d).ReactionsOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("ReactionsOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "Reactions.find(m.id)") {
		t.Fatal("the script does not call Reactions.find with the id OBJECT, which " +
			"is the only key that answers on this build")
	}
	if strings.Contains(code, "find(m.id._serialized)") || strings.Contains(code, "find(m.id.id)") {
		t.Fatal("the script keys the lookup by a serialized id; _serialized is null " +
			"here and the raw id returns null, so it would find nothing forever")
	}
}

// THE MODELS ARRAY IS NOT THE SOURCE. Two findings measured it at zero after a
// verified reaction and concluded this build cannot report them; the record
// comes from the fetch and never lands there.
func TestTheReactionsScriptDoesNotReadTheModelsArray(t *testing.T) {
	d := &double{answer: `{"ok":true,"groups":[]}`}
	if _, err := rd(d).ReactionsOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("ReactionsOf: %v", err)
	}
	code := withoutComments(d.lastScript)
	if strings.Contains(code, "Reactions.getModelsArray") {
		t.Fatal("the script reads Reactions.getModelsArray, which stays at 0 even " +
			"after a reaction this module verified")
	}
}

func TestReactionsComeBackGroupedByEmoji(t *testing.T) {
	d := &double{answer: `{"ok":true,"groups":[` +
		`{"emoji":"A","byMe":true,"senders":["1@lid","2@lid"]},` +
		`{"emoji":"B","byMe":false,"senders":["3@lid"]}]}`}
	got, err := rd(d).ReactionsOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("ReactionsOf: %v", err)
	}
	if len(got.Groups) != 2 {
		t.Fatalf("the grouping was lost: %#v", got.Groups)
	}
	if got.Total() != 3 {
		t.Fatalf("Total counted %d senders across the groups, want 3", got.Total())
	}
	if !got.Groups[0].ByMe || got.Groups[1].ByMe {
		t.Fatal("ByMe was not carried per group; it is the page's own flag and " +
			"comparing jids to derive it is the comparison that has gone wrong here before")
	}
}

// A MESSAGE NOBODY REACTED TO IS NOT AN ERROR.
func TestAMessageWithNoReactionsIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":false,"groups":[]}`}
	got, err := rd(d).ReactionsOf(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("a message with no reactions must not be an error: %v", err)
	}
	if got.Any() || got.Total() != 0 {
		t.Fatalf("an empty read reported reactions: %s", got)
	}
}

// The rendering carries counts, never the emoji and never a sender.
func TestTheReactionsRenderingIsQuiet(t *testing.T) {
	r := Reactions{Groups: []Reaction{{Emoji: "ZZZ", Senders: []string{"5516999999999@lid"}}}}
	s := r.String()
	for _, leak := range []string{"ZZZ", "5516", "999999999"} {
		if strings.Contains(s, leak) {
			t.Fatalf("the rendering leaks %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, "groups=1") || !strings.Contains(s, "senders=1") {
		t.Fatalf("the rendering lost the counts: %s", s)
	}
}

func TestAnEmptyIDNeverReachesThePageForReactions(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).ReactionsOf(context.Background(), "", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("want ErrNoMessage, got %v", err)
	}
	if d.kicks != 0 {
		t.Fatalf("an empty id reached the page %d times", d.kicks)
	}
}

// THE SCRIPT EMBEDS THE SHARED EXPRESSION, it does not carry a copy.
//
// capabilities/react verifies its removals against the same fact. Two copies
// would drift, and the drift would show as a removal that verified and then read
// back as still present — which is the worst shape a bug can take here, because
// both halves would look correct in isolation.
func TestTheReactionsScriptEmbedsTheSharedExpression(t *testing.T) {
	d := &double{answer: `{"ok":true,"groups":[]}`}
	if _, err := rd(d).ReactionsOf(context.Background(), "3EB0", "t"); err != nil {
		t.Fatalf("ReactionsOf: %v", err)
	}
	if !strings.Contains(d.lastScript, spa.ReactionsForMessageExpr) {
		t.Fatal("the script no longer embeds spa.ReactionsForMessageExpr, so this " +
			"package and capabilities/react now hold two copies of the same query")
	}
}
