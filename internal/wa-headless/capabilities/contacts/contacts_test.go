package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	answer string
	err    error
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.err != nil {
		return p.err
	}
	*out = p.answer
	return nil
}

func lister(p *pageDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func rowsJSON(rows ...string) string {
	return `{"ok":true,"rows":[` + strings.Join(rows, ",") + `]}`
}

func phoneRow(user, pushname string) string {
	return fmt.Sprintf(`{"user":%q,"server":"c.us","phone_user":"","pushname":%q,`+
		`"verified_name":"","is_business":false,"is_psa":false,"is_user":true}`, user, pushname)
}

// lidRow mirrors the REAL rule measured on 2026-08-20: a lid row may carry a
// phoneNumber (398 of 489 did), and a phone row never carries a lid (0 of 454).
// The double therefore has no way to express the reverse edge — which is the
// point, because a double that offered it would let a merge walking the wrong
// direction pass green.
func lidRow(user, phoneUser, pushname string) string {
	return fmt.Sprintf(`{"user":%q,"server":"lid","phone_user":%q,"pushname":%q,`+
		`"verified_name":"","is_business":false,"is_psa":false,"is_user":true}`, user, phoneUser, pushname)
}

// TestTheSamePersonInTwoRowsBecomesOneContact is the reason this package
// exists. 944 rows described about 550 people on the lab profile; returning the
// rows unchanged would inflate the roster by ~70%.
func TestTheSamePersonInTwoRowsBecomesOneContact(t *testing.T) {
	p := &pageDouble{answer: rowsJSON(
		phoneRow("111", "Ana"),
		lidRow("999", "111", ""),
	)}
	got, err := lister(p).List(context.Background(), "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Contacts) != 1 {
		t.Fatalf("got %d contacts from 2 rows for one person: %v", len(got.Contacts), got.Contacts)
	}
	c := got.Contacts[0]
	if c.PN != "111@c.us" || c.LID != "999@lid" {
		t.Fatalf("the merged contact lost an identity: %+v", c)
	}
	if !c.Merged || got.Merged != 1 {
		t.Fatalf("the merge was not reported: contact.Merged=%t roster.Merged=%d", c.Merged, got.Merged)
	}
	if got.Rows != 2 {
		t.Fatalf("Rows=%d, want the pre-merge count of 2 — without it a caller "+
			"cannot tell deduplication happened", got.Rows)
	}
}

// TestMergingDoesNotErasePushname: the two rows rarely both carry a name, and
// whichever one has it must win. Overwriting with the empty string is the
// obvious way to lose 456 names.
func TestMergingDoesNotErasePushname(t *testing.T) {
	// Name on the LID row, phone row silent.
	p := &pageDouble{answer: rowsJSON(
		phoneRow("111", ""),
		lidRow("999", "111", "Ana"),
	)}
	got, _ := lister(p).List(context.Background(), "t/list")
	if len(got.Contacts) != 1 || got.Contacts[0].Pushname != "Ana" {
		t.Fatalf("the lid row's name was lost: %+v", got.Contacts)
	}

	// Name on the phone row, lid row silent — the other direction.
	p = &pageDouble{answer: rowsJSON(
		phoneRow("111", "Ana"),
		lidRow("999", "111", ""),
	)}
	got, _ = lister(p).List(context.Background(), "t/list")
	if len(got.Contacts) != 1 || got.Contacts[0].Pushname != "Ana" {
		t.Fatalf("the phone row's name was erased by the empty lid row: %+v", got.Contacts)
	}
}

// TestLidWithoutAPhoneRowIsStillAPerson: 91 of 489 lid rows named no phone that
// had a row of its own. Dropping them would silently lose contacts.
func TestLidWithoutAPhoneRowIsStillAPerson(t *testing.T) {
	p := &pageDouble{answer: rowsJSON(
		phoneRow("111", "Ana"),
		lidRow("999", "", "Bea"), // no phone at all
		lidRow("888", "777", ""), // names a phone with no row of its own
	)}
	got, err := lister(p).List(context.Background(), "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Contacts) != 3 {
		t.Fatalf("got %d, want 3 — a lid without a matching phone row is still a "+
			"person: %v", len(got.Contacts), got.Contacts)
	}
	if got.Merged != 0 {
		t.Fatalf("Merged=%d, want 0 — nothing here was a duplicate", got.Merged)
	}
	// The lid row that named an orphan phone still knows the number.
	var orphan *Contact
	for i := range got.Contacts {
		if got.Contacts[i].LID == "888@lid" {
			orphan = &got.Contacts[i]
		}
	}
	if orphan == nil || orphan.PN != "777@c.us" {
		t.Fatalf("a lid row naming an unlisted phone lost the number: %+v", got.Contacts)
	}
}

// TestGroupsAreNotContacts: the roster held one "g.us" row. A group returned as
// a contact is the same silent wrongness the deduplication exists to prevent.
func TestGroupsAreNotContacts(t *testing.T) {
	p := &pageDouble{answer: rowsJSON(
		phoneRow("111", "Ana"),
		`{"user":"120363","server":"g.us","phone_user":"","pushname":"Grupo",`+
			`"verified_name":"","is_business":false,"is_psa":false,"is_user":false}`,
	)}
	got, _ := lister(p).List(context.Background(), "t/list")
	if len(got.Contacts) != 1 {
		t.Fatalf("a group was returned as a contact: %v", got.Contacts)
	}
	if got.Rows != 2 {
		t.Fatalf("Rows=%d, want 2 — the denominator counts what the page held", got.Rows)
	}
}

// TestIdentityPrefersTheLID, because the send path measured that this build
// addresses by lid and that the phone jid produces "No LID for user" (H34).
func TestIdentityPrefersTheLID(t *testing.T) {
	both := Contact{PN: "111@c.us", LID: "999@lid"}
	if both.Identity() != "999@lid" {
		t.Fatalf("Identity()=%q, want the lid", both.Identity())
	}
	onlyPN := Contact{PN: "111@c.us"}
	if onlyPN.Identity() != "111@c.us" {
		t.Fatalf("Identity()=%q, want the phone when there is no lid", onlyPN.Identity())
	}
}

// TestOrderDoesNotDependOnThePageOrder is the ordering contract, and the first
// version of this test did NOT test it.
//
// That version fed one input repeatedly and asserted the output matched itself.
// It passed — and it would have passed with the sort deleted, because the merge
// accumulates into a slice in input order, so repeat runs agree trivially. A
// test that cannot fail is worse than no test.
//
// The contract worth having is that the roster reads the same no matter what
// order the page hands the rows over, since nothing documents that order and it
// is free to change under us.
func TestOrderDoesNotDependOnThePageOrder(t *testing.T) {
	forward := rowsJSON(
		phoneRow("333", "C"), phoneRow("111", "A"), phoneRow("222", "B"),
		lidRow("999", "222", ""), lidRow("888", "", "D"), lidRow("777", "", "E"),
	)
	reversed := rowsJSON(
		lidRow("777", "", "E"), lidRow("888", "", "D"), lidRow("999", "222", ""),
		phoneRow("222", "B"), phoneRow("111", "A"), phoneRow("333", "C"),
	)
	keys := func(answer string) string {
		got, err := lister(&pageDouble{answer: answer}).List(context.Background(), "t/list")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		var out []string
		for _, c := range got.Contacts {
			out = append(out, c.Identity())
		}
		return strings.Join(out, "|")
	}
	a, b := keys(forward), keys(reversed)
	if a != b {
		t.Fatalf("the roster order followed the page's order:\n forward =%s\n reversed=%s", a, b)
	}
	// And it is repeatable, because the merge indexes through a map and map
	// iteration in Go is randomised by design.
	for i := 0; i < 25; i++ {
		if got := keys(forward); got != a {
			t.Fatalf("run %d differed: %s vs %s", i, got, a)
		}
	}
}

// TestRedactionNeverPrintsAPersonsData. A contact is a number and a name, and
// both are exactly what the briefing forbids in a log.
func TestRedactionNeverPrintsAPersonsData(t *testing.T) {
	c := Contact{PN: "5541999998888@c.us", LID: "123456@lid", Pushname: "Ana Silva"}
	s := c.String()
	for _, secret := range []string{"5541999998888", "123456", "Ana", "Silva"} {
		if strings.Contains(s, secret) {
			t.Fatalf("String() leaked %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "pushname=<redacted>") {
		t.Fatalf("a contact with a name must say so without saying it: %s", s)
	}
	if empty := (Contact{PN: "1@c.us"}).String(); !strings.Contains(empty, "pushname=none") {
		t.Fatalf("a contact with no name must be distinguishable from a redacted one: %s", empty)
	}
}

func TestUnavailableCollectionIsAnError(t *testing.T) {
	p := &pageDouble{answer: `{"ok":false}`}
	if _, err := lister(p).List(context.Background(), "t/list"); !errors.Is(err, ErrNoCollection) {
		t.Fatalf("got %v, want ErrNoCollection", err)
	}
}

func TestCancelledContextIsNotASuccessfulEmptyRoster(t *testing.T) {
	p := &pageDouble{answer: rowsJSON(phoneRow("111", "Ana"))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lister(p).List(ctx, "t/list"); err == nil {
		t.Fatal("a cancelled context produced a roster")
	}
}

// TestASecondLidForTheSamePersonIsResolvedStably.
//
// The live roster folded 398 lid rows into 390 people: eight people carry TWO
// lid rows. Keeping whichever arrived last would make those eight depend on the
// page's row order — the very dependency TestOrderDoesNotDependOnThePageOrder
// refuses. Which lid is canonical is unknown, so the tie breaks on the smaller
// string: arbitrary, but the same every time.
func TestASecondLidForTheSamePersonIsResolvedStably(t *testing.T) {
	forward := rowsJSON(
		phoneRow("111", "Ana"),
		lidRow("999", "111", ""),
		lidRow("222", "111", ""), // same person, a second lid
	)
	reversed := rowsJSON(
		lidRow("222", "111", ""),
		lidRow("999", "111", ""),
		phoneRow("111", "Ana"),
	)
	for _, answer := range []string{forward, reversed} {
		got, err := lister(&pageDouble{answer: answer}).List(context.Background(), "t/list")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got.Contacts) != 1 {
			t.Fatalf("two lids for one person produced %d contacts: %v",
				len(got.Contacts), got.Contacts)
		}
		if got.Contacts[0].LID != "222@lid" {
			t.Fatalf("LID=%q, want the stable choice 222@lid regardless of row order",
				got.Contacts[0].LID)
		}
		// Merged counts ROWS folded in, so both lid rows count.
		if got.Merged != 2 {
			t.Fatalf("Merged=%d, want 2 — it counts rows folded in, not people "+
				"duplicated, which is why the live numbers were 398 and 390", got.Merged)
		}
	}
}

// TestAPSARowIsNotAPerson.
//
// The roster holds exactly one row the page marks isPSA(): a system sentinel
// with a one-character user. It is ALSO isUser(), so no "is this an individual"
// check catches it, and the first version of this package returned it as a
// contact.
//
// It was not found by reading the roster. It was found by the avatar
// capability, which asked the server for that row's picture and got a request
// that hung for the full budget and never answered — one of twelve contacts
// failing while eleven worked.
func TestAPSARowIsNotAPerson(t *testing.T) {
	psa := `{"user":"0","server":"c.us","phone_user":"","pushname":"",` +
		`"verified_name":"","is_business":false,"is_psa":true,"is_user":true}`
	p := &pageDouble{answer: rowsJSON(phoneRow("111", "Ana"), psa)}
	got, err := lister(p).List(context.Background(), "t/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Contacts) != 1 {
		t.Fatalf("a PSA sentinel was returned as a contact: %v", got.Contacts)
	}
	if got.Rows != 2 {
		t.Fatalf("Rows=%d, want 2 — the denominator counts what the page held, "+
			"including what was dropped", got.Rows)
	}
}

// TestAPSAOnTheLIDSideIsAlsoDropped: the drop rule must not live on one branch.
// The phone pass and the lid pass are separate loops, and a filter applied to
// only one of them would let the sentinel back in through the other.
func TestAPSAOnTheLIDSideIsAlsoDropped(t *testing.T) {
	psa := `{"user":"0","server":"lid","phone_user":"","pushname":"",` +
		`"verified_name":"","is_business":false,"is_psa":true,"is_user":true}`
	p := &pageDouble{answer: rowsJSON(phoneRow("111", "Ana"), psa)}
	got, _ := lister(p).List(context.Background(), "t/list")
	if len(got.Contacts) != 1 {
		t.Fatalf("a PSA row on the lid side survived: %v", got.Contacts)
	}
}
