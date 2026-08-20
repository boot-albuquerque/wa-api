// Package contacts answers the product's listContacts by reading the SPA's
// roster.
//
// THE ROSTER DOUBLE-COUNTS PEOPLE, and that is the whole difficulty. Measured
// 2026-08-20 against the lab profile:
//
//	944  models in WAWebContactCollection
//	454  distinct "c.us" identities
//	489  distinct "lid" identities
//	398  lid rows carrying a phoneNumber, covering 390 distinct phones
//	390  of those phones ALSO present as their own c.us row  <- all of them
//	  0  c.us rows carrying a lid
//
// So roughly 390 people appear twice, and a listing that returned
// getModelsArray() unchanged would report 944 contacts for about 550 people —
// wrong in a way no caller could detect, because both rows look valid.
//
// THE JOIN IS ONE-DIRECTIONAL. The lid row knows its phone; the phone row does
// not know its lid (0 of 454). Merging therefore walks lid -> phone and never
// the reverse, because the reverse edge does not exist in this build.
//
// The merge lives in Go, not in the page, so it can be tested against doubles
// instead of only against a live account.
package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// ErrNoCollection is a page where the roster did not resolve.
var ErrNoCollection = fmt.Errorf("contacts: the contact collection is not available")

// Contact is one PERSON, after the two rows describing them have been merged.
type Contact struct {
	// PN is the phone identity ("…@c.us"), empty when only a lid is known.
	PN string
	// LID is the server identity ("…@lid"), empty when only a phone is known.
	LID string
	// Pushname is the name the person set on their own device. Measured at
	// 456 of 944 rows, which makes it the only name this capability can
	// promise — see spa.ModuleContactGetters for why the others are empty.
	Pushname string
	// VerifiedName is a business's verified name (22 of 944 measured).
	VerifiedName string
	IsBusiness   bool
	// Merged reports that this person arrived as TWO rows. It is kept because
	// the count is the only evidence a caller has that deduplication happened.
	Merged bool
}

// Identity returns the jid a caller should address, preferring the server's.
//
// LID first is not a preference, it is what the send path measured: this build
// stores messages under the lid, and addressing by phone is what produced
// "No LID for user" (HOUSEKEEP H34).
func (c Contact) Identity() string {
	if c.LID != "" {
		return c.LID
	}
	return c.PN
}

// String redacts. A contact is a person's number and a person's name, which is
// the most personal data this module handles, so neither ever renders.
func (c Contact) String() string {
	name := "none"
	if c.Pushname != "" {
		name = "<redacted>"
	}
	return fmt.Sprintf("contacts.Contact(identity=<redacted> pn=%t lid=%t pushname=%s business=%t merged=%t)",
		c.PN != "", c.LID != "", name, c.IsBusiness, c.Merged)
}

// Roster is the result of a listing, WITH the denominators that make it
// readable. Rows without People would hide the deduplication entirely.
type Roster struct {
	// Contacts are the merged people, ordered deterministically.
	Contacts []Contact
	// Rows is how many models the collection held before merging (944 when
	// measured). Rows > len(Contacts) is the normal, correct state.
	Rows int
	// Merged is how many ROWS were folded into a person that already existed —
	// not how many people were duplicated. The two differ: the live roster
	// folded 398 rows into 390 people, because eight people carry two lid rows
	// each. Naming it after rows keeps Rows - Merged meaningful.
	Merged int
}

func (r Roster) String() string {
	return fmt.Sprintf("contacts.Roster(people=%d rows=%d merged=%d)",
		len(r.Contacts), r.Rows, r.Merged)
}

// Lister reads the roster for one session.
type Lister struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Lister.
func New(runner *engine.Runner, eval spa.Evaluator) *Lister {
	return &Lister{runner: runner, eval: eval}
}

// row is one contact model as the page reports it, before merging.
type row struct {
	User   string `json:"user"`
	Server string `json:"server"`
	// PhoneUser is the phoneNumber.user a LID row carries. Measured present on
	// 398 of 489 lid rows and on 0 of 454 phone rows.
	PhoneUser    string `json:"phone_user"`
	Pushname     string `json:"pushname"`
	VerifiedName string `json:"verified_name"`
	IsBusiness   bool   `json:"is_business"`
	// IsPSA marks a public-service-announcement row: a system sentinel, not a
	// person. Exactly one exists in the 944, and it is also isUser(), which is
	// why filtering on "is it a user" let it through.
	IsPSA bool `json:"is_psa"`
	// IsUser is the page's own answer to "is this an individual". Carried so
	// the drop rule is the page's rule and not a guess about jid shapes.
	IsUser bool `json:"is_user"`
}

// wire is the page's answer.
type wire struct {
	OK   bool  `json:"ok"`
	Rows []row `json:"rows"`
}

// RowExpr maps one contact model to the row shape, and it is EXPORTED so the
// change subscription uses the same one.
//
// Two extractors would drift: someone widens the listing to carry a new field
// and the subscription keeps sending the old shape, or — worse — one of them
// stops applying the page's isPSA predicate and the sentinel comes back through
// whichever door was not fixed. Same reasoning as messagemeta.MetaExpr, and for
// the same reason: one list, one place to be wrong.
const RowExpr = `(function (c) {
	const G = window.require('` + string(spa.ModuleContactGetters) + `');
	const str = function (v) { return (typeof v === 'string') ? v : ''; };
	let id;
	try { id = c.id; } catch (e) { return null; }
	if (!id || !id.user || !id.server) { return null; }
	let phoneUser = '';
	try { if (c.phoneNumber && c.phoneNumber.user) { phoneUser = c.phoneNumber.user; } } catch (e) {}
	let pushname = '', verified = '', biz = false;
	try { pushname = str(G.getPushname && G.getPushname(c)); } catch (e) {}
	try { verified = str(G.getVerifiedName && G.getVerifiedName(c)); } catch (e) {}
	try { biz = !!(G.getIsBusiness && G.getIsBusiness(c)); } catch (e) {}
	// The page's own predicates decide what is a person. A rule written here
	// from jid shapes would be a guess; isPSA is not.
	const pred = function (name) {
		try { return typeof id[name] === 'function' ? !!id[name]() : false; } catch (e) { return false; }
	};
	return { user: id.user, server: id.server, phone_user: phoneUser,
		pushname: pushname, verified_name: verified, is_business: biz,
		is_psa: pred('isPSA'), is_user: pred('isUser') };
})`

// script reads the collection and returns RAW rows. It deliberately does not
// merge: merging in the page would put the one piece of real logic here
// somewhere only a live account can exercise.
func script() string {
	return `JSON.stringify((() => {
		const mod = window.require('` + string(spa.ModuleContactCollection) + `');
		const coll = mod && mod.ContactCollection;
		if (!coll || typeof coll.getModelsArray !== 'function') { return { ok: false }; }
		const row = ` + RowExpr + `;
		const all = coll.getModelsArray();
		const rows = [];
		for (let i = 0; i < all.length; i++) {
			const r = row(all[i]);
			if (r) { rows.push(r); }
		}
		return { ok: true, rows: rows };
	})())`
}

// List reads the roster and returns one entry per person.
func (l *Lister) List(ctx context.Context, label string) (Roster, error) {
	var raw string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return l.eval(ctx, script(), &raw)
	}); err != nil {
		return Roster{}, fmt.Errorf("contacts: reading the collection: %w", err)
	}
	var w wire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return Roster{}, fmt.Errorf("contacts: unexpected answer: %w", err)
	}
	if !w.OK {
		return Roster{}, ErrNoCollection
	}
	return merge(w.Rows), nil
}

// serverPhone and serverLID are the two identity spaces this build uses. They
// are constants because a literal repeated across a comparison and a formatter
// is the same bug waiting to diverge (ADR-0004).
const (
	serverPhone = "c.us"
	serverLID   = "lid"
)

// isPerson decides what the roster may return, using the PAGE'S predicates.
//
// The first version dropped only "g.us" rows and kept everything else. That let
// through a single row with a one-character user which the page marks
// isPSA() — a system sentinel that is ALSO isUser(), so no is-it-a-user check
// would have caught it. It was found by the avatar capability, whose request
// for that row's picture hung for the full budget and never answered.
//
// The rule is therefore the page's own, not a shape heuristic: a person is a
// row the page calls a user and does not call a PSA. Measured over 944 rows:
// isUser 943, isGroup 1, isPSA 1, isServer 0, isNewsletter 0.
func isPerson(r row) bool {
	if r.IsPSA || !r.IsUser {
		return false
	}
	return r.Server == serverPhone || r.Server == serverLID
}

// merge folds the two rows describing one person into one Contact.
//
// It walks lid -> phone because that is the only edge the build provides: the
// lid row carries phoneNumber, the phone row carries nothing pointing back.
func merge(rows []row) Roster {
	out := Roster{Rows: len(rows)}

	// Phone rows first, indexed by their user, so the lid pass can find them.
	byPhone := make(map[string]*Contact, len(rows))
	order := make([]*Contact, 0, len(rows))
	for _, r := range rows {
		if r.Server != serverPhone || !isPerson(r) {
			continue
		}
		c := &Contact{
			PN:           r.User + "@" + serverPhone,
			Pushname:     r.Pushname,
			VerifiedName: r.VerifiedName,
			IsBusiness:   r.IsBusiness,
		}
		byPhone[r.User] = c
		order = append(order, c)
	}

	for _, r := range rows {
		if r.Server != serverLID || !isPerson(r) {
			continue
		}
		lid := r.User + "@" + serverLID
		if r.PhoneUser != "" {
			if c, ok := byPhone[r.PhoneUser]; ok {
				// The same person, arriving a second time. The lid row is the
				// one worth keeping for addressing; the phone row is kept for
				// the number. Fields fill in rather than overwrite: an empty
				// pushname on one row must not erase a real one on the other.
				// A PERSON CAN HAVE MORE THAN ONE LID ROW. Measured against
				// the live roster: 398 lid rows folded into 390 people, so
				// eight people arrived with two lids each.
				//
				// Taking whichever came last would make those eight depend on
				// the page's row order, which nothing documents and which the
				// ordering contract explicitly refuses to inherit. Which lid is
				// canonical is NOT known, so the tie is broken the only way
				// that is stable: the smaller string wins. Arbitrary but
				// repeatable beats correct-looking but not.
				if c.LID == "" || lid < c.LID {
					c.LID = lid
				}
				c.Merged = true
				out.Merged++
				fillIn(c, r)
				continue
			}
		}
		c := &Contact{
			LID:          lid,
			Pushname:     r.Pushname,
			VerifiedName: r.VerifiedName,
			IsBusiness:   r.IsBusiness,
		}
		if r.PhoneUser != "" {
			// A lid row naming a phone that has no row of its own. The person
			// is still one person, and the number is still known.
			c.PN = r.PhoneUser + "@" + serverPhone
		}
		order = append(order, c)
	}

	out.Contacts = make([]Contact, 0, len(order))
	for _, c := range order {
		out.Contacts = append(out.Contacts, *c)
	}
	// Deterministic order, because map iteration in Go is randomised by design
	// and a listing whose order changes between calls is a listing no caller
	// can diff.
	sort.SliceStable(out.Contacts, func(i, j int) bool {
		return sortKey(out.Contacts[i]) < sortKey(out.Contacts[j])
	})
	return out
}

// contactOf builds the single-row view of a person, WITHOUT merging. A change
// event carries one row, so there is no second row to fold in — and pretending
// otherwise would report a lid-only view of someone whose phone is known.
func contactOf(r row) Contact {
	c := Contact{
		Pushname:     strings.TrimSpace(r.Pushname),
		VerifiedName: strings.TrimSpace(r.VerifiedName),
		IsBusiness:   r.IsBusiness,
	}
	switch r.Server {
	case serverLID:
		c.LID = r.User + "@" + serverLID
		if r.PhoneUser != "" {
			c.PN = r.PhoneUser + "@" + serverPhone
		}
	case serverPhone:
		c.PN = r.User + "@" + serverPhone
	}
	return c
}

func sortKey(c Contact) string {
	if c.PN != "" {
		return "0" + c.PN
	}
	return "1" + c.LID
}

// fillIn copies fields from a second row for the same person WITHOUT erasing
// what the first row already provided.
func fillIn(c *Contact, r row) {
	if c.Pushname == "" {
		c.Pushname = strings.TrimSpace(r.Pushname)
	}
	if c.VerifiedName == "" {
		c.VerifiedName = strings.TrimSpace(r.VerifiedName)
	}
	if r.IsBusiness {
		c.IsBusiness = true
	}
}
