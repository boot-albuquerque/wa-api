package group

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/headless/engine"
)

type metaDouble struct {
	answer     string
	kicks      int
	lastScript string
}

func (d *metaDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+joinStateKey) {
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func metaMgr(d *metaDouble) *Manager { return New(engine.NewRunner(), d.eval) }

func stripComments(script string) string {
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

// A non-group jid never reaches the page.
func TestMetadataRefusesANonGroup(t *testing.T) {
	d := &metaDouble{answer: `{"ok":true}`}
	if _, err := metaMgr(d).Metadata(context.Background(), "1@c.us", "t"); !errors.Is(err, ErrNotGroup) {
		t.Errorf("err = %v, want ErrNotGroup", err)
	}
	if d.kicks != 0 {
		t.Error("a non-group reached the page")
	}
}

// AN EMPTY DESCRIPTION IS AMBIGUOUS, AND THE SOURCE IS HOW A CALLER TELLS.
//
// Measured on the lab group: both desc and displayedDesc read undefined while
// descTime is a number and __x_displayedDesc exists in raw storage. That is
// consistent with a group having no description AND with a reader looking at the
// wrong field, and one group cannot separate them. Picking one silently is the
// defect class this repository has met five times.
func TestTheDescriptionCarriesItsSource(t *testing.T) {
	cases := map[string]struct{ answer, wantText, wantSrc string }{
		"displayed": {`{"ok":true,"desc":"olá","descSource":"displayedDesc"}`, "olá", "displayedDesc"},
		"legacy":    {`{"ok":true,"desc":"olá","descSource":"desc"}`, "olá", "desc"},
		"absent":    {`{"ok":true,"desc":"","descSource":"none"}`, "", "none"},
	}
	for name, c := range cases {
		d := &metaDouble{answer: c.answer}
		got, err := metaMgr(d).Metadata(context.Background(), labGroupID, "t")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Description != c.wantText || got.DescriptionSource != c.wantSrc {
			t.Errorf("%s: got %q/%q", name, got.Description, got.DescriptionSource)
		}
	}
	// And the script must consult BOTH fields, naming the winner.
	d := &metaDouble{answer: `{"ok":true}`}
	_, _ = metaMgr(d).Metadata(context.Background(), labGroupID, "t")
	code := stripComments(d.lastScript)
	for _, want := range []string{"md.displayedDesc", "md.desc", `descSource = "displayedDesc"`, `descSource = "desc"`} {
		if !strings.Contains(code, want) {
			t.Errorf("the script does not consult/report %s", want)
		}
	}
}

// THE REFRESH RUNS BEFORE THE READ, and the ORDER is the assertion: both calls
// present in the wrong order answers with boot-time metadata, which is the
// failure that looks like a correct answer.
func TestMetadataRefreshesFirst(t *testing.T) {
	d := &metaDouble{answer: `{"ok":true}`}
	_, _ = metaMgr(d).Metadata(context.Background(), labGroupID, "t")
	code := stripComments(d.lastScript)
	refresh := strings.Index(code, "queryAndUpdateGroupMetadataById")
	read := strings.Index(code, "chat.groupMetadata")
	if refresh < 0 || read < 0 {
		t.Fatal("the script does not both refresh and read")
	}
	if refresh > read {
		t.Fatal("the refresh runs AFTER the read; it would answer with boot-time metadata")
	}
}

// The participants survive with their flags, and a zero join time stays zero.
func TestTheParticipantsSurvive(t *testing.T) {
	d := &metaDouble{answer: `{"ok":true,"owner":"1@lid","created":1700000000,` +
		`"participants":[{"jid":"1@lid","admin":true,"superAdmin":true,"joined":1700000001},` +
		`{"jid":"2@lid","admin":false,"superAdmin":false,"joined":0}]}`}
	got, err := metaMgr(d).Metadata(context.Background(), labGroupID, "t")
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(got.Participants) != 2 {
		t.Fatalf("%d participants", len(got.Participants))
	}
	if !got.Participants[0].SuperAdmin || got.Participants[1].Admin {
		t.Errorf("the admin flags were lost: %+v", got.Participants)
	}
	if got.Participants[0].JoinedAt.IsZero() {
		t.Error("a join time was dropped")
	}
	if !got.Participants[1].JoinedAt.IsZero() {
		t.Error("join time 0 became the epoch")
	}
	if got.Owner == "" || got.CreatedAt.IsZero() {
		t.Errorf("owner/createdAt lost: %+v", got)
	}
}

// A missing chat is ErrNotGroup, not a read failure: the repairs differ.
func TestAMissingChatIsNotAReadFailure(t *testing.T) {
	d := &metaDouble{answer: `{"ok":false,"why":"NO_CHAT"}`}
	if _, err := metaMgr(d).Metadata(context.Background(), labGroupID, "t"); !errors.Is(err, ErrNotGroup) {
		t.Errorf("err = %v, want ErrNotGroup", err)
	}
	d2 := &metaDouble{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	_, err := metaMgr(d2).Metadata(context.Background(), labGroupID, "t")
	if !errors.Is(err, ErrMetadata) || errors.Is(err, ErrNotGroup) {
		t.Errorf("err = %v, want ErrMetadata and not ErrNotGroup", err)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("the reason was dropped: %v", err)
	}
}

// The rendering carries no identity.
func TestTheMetadataRenderingIsQuiet(t *testing.T) {
	m := Metadata{JID: labGroupID, Subject: "wa-headless-lab", Owner: "5541999999999@lid",
		Description: "segredo", DescriptionSource: "none",
		Participants: []Participant{{JID: "1@lid", Admin: true}}}
	s := m.String()
	if strings.Contains(s, "5541") || strings.Contains(s, "segredo") ||
		strings.Contains(s, "wa-headless-lab") || strings.Contains(s, labGroupID) {
		t.Errorf("Metadata.String carries content: %s", s)
	}
	if !strings.Contains(s, "admins=1") {
		t.Errorf("the rendering does not count admins: %s", s)
	}
}
