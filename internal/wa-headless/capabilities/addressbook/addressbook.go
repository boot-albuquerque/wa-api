// Package addressbook writes this account's OWN contact list.
//
// It is the half of "contacts" this module never had. capabilities/contacts
// READS what the account already knows — names, about, labels. Nothing here
// could put a name on a number, which meant the account's view of a contact was
// whatever the phone had synced and nothing this process could change.
//
// WHY IT CLOSES AN EVENT TOO. events.ContactChanged was installed and NEVER
// PROVEN: the bus had a listener for it and no way to make it fire, because
// nothing in this module could make a contact record move — the only paths were
// another account editing its own profile, which this side cannot cause. A save
// moves the record in THIS session, which is the trigger the rule asks for
// (H90).
//
// SYNCING TO THE PHONE IS A DIFFERENT ACT, and it is a parameter rather than a
// default. saveContactAction takes syncToAddressbook, and true writes the
// contact into the address book of the physical phone this account is paired
// with — a change outside this process, on somebody's device, that no test here
// can undo. It defaults to false and the caller has to ask.
package addressbook

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoNumber is an empty phone number.
	// ErrUnresolvedIdentity is a phone jid handed to DeviceCount, which cannot
	// answer about one.
	//
	// IT IS NOT ErrDevices (decisão 66). Measured side by side on the same peer:
	// under the lid, 5 devices; under the phone jid, "the page has no device
	// record for that user" — an answer that reads as a fact about the PERSON and
	// is a fact about the ARGUMENT. Both are well formed, which is what made the
	// error invisible for months (H148).
	ErrUnresolvedIdentity = fmt.Errorf("addressbook: that jid is a phone number and this build indexes people by lid; resolve it first")
	ErrNoNumber           = fmt.Errorf("addressbook: no phone number given")
	// ErrNoName is a save with neither a first nor a last name. The page would
	// accept it and store a nameless contact, which is indistinguishable from
	// not having saved at all — and is therefore refused here rather than
	// discovered later.
	ErrNoName = fmt.Errorf("addressbook: a save needs at least a first or a last name")
	// ErrSave is the page refusing or failing the save.
	ErrSave = fmt.Errorf("addressbook: the page refused the contact save")
	// ErrDelete is the page refusing or failing the delete.
	ErrDelete = fmt.Errorf("addressbook: the page refused the contact delete")
	// ErrDevices is the page refusing or failing the device read.
	ErrDevices = fmt.Errorf("addressbook: the page refused the device-list read")
	// ErrNotSaved is the page reporting success while the contact record did
	// not move. It is its own error because the repairs differ: a refusal is a
	// permission or a format problem, and a silent no-op is the page accepting
	// a write it did not apply.
	ErrNotSaved = fmt.Errorf("addressbook: the save was accepted and the contact record did not move")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKeyPrefix = "__waHeadlessAddressBook"

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Saved is what one save did.
//
// IT CARRIES BOOLEANS, NOT THE NAME. A contact name is exactly the kind of
// thing this module refuses to move around, and a result is what gets logged.
type Saved struct {
	// HadName says the contact already had a name before this call.
	HadName bool
	// HasName says it has one now. A save that leaves this false is reported
	// as ErrNotSaved rather than as success.
	HasName bool
	// SyncedToPhone repeats what the caller asked for, so a log says whether a
	// change left this process.
	SyncedToPhone bool
	// Waited is how long the record took to move.
	Waited time.Duration
}

func (s Saved) String() string {
	return fmt.Sprintf("addressbook.Saved(hadName=%t hasName=%t syncedToPhone=%t waited=%s)",
		s.HadName, s.HasName, s.SyncedToPhone, s.Waited.Round(time.Millisecond))
}

// Manager writes this account's contact list.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// Save puts a name on a number in this account's contact list.
//
// syncToPhone writes it to the paired PHONE's address book as well. That is a
// change on somebody's device that nothing here can undo, so it is a parameter
// with no default and the doc on this package says why.
//
// IT VERIFIES. The page accepts the call and the record moves a moment later;
// reporting success on the acceptance alone is the defect this module has paid
// for repeatedly (H82, H85), so the answer is read back until it moves or the
// budget runs out.
func (m *Manager) Save(ctx context.Context, phone, first, last string, syncToPhone bool, label string) (Saved, error) {
	phone = normalizeNumber(phone)
	if phone == "" {
		return Saved{}, ErrNoNumber
	}
	if strings.TrimSpace(first) == "" && strings.TrimSpace(last) == "" {
		return Saved{}, ErrNoName
	}
	start := time.Now()
	key := nextStateKey()
	raw, err := m.parked(ctx, saveScript(phone, first, last, syncToPhone, key), key, label+"/save")
	if err != nil {
		return Saved{}, fmt.Errorf("%w: %v", ErrSave, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
		Had bool   `json:"had"`
		Has bool   `json:"has"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Saved{}, fmt.Errorf("addressbook: unexpected save answer: %w", e)
	}
	if !out.OK {
		return Saved{}, fmt.Errorf("%w (%s)", ErrSave, out.Why)
	}
	has := out.Has
	// THE WAIT IS HERE, under the caller's context and this package's budget.
	// The record moves a moment after the page accepts the call, and the page
	// is deliberately not the one counting — invariant 6, and the script says
	// why at the point where the temptation lives.
	deadline := time.Now().Add(Budget)
	for !has {
		if !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return Saved{}, ctx.Err()
		case <-time.After(Tick):
		}
		named, err := m.named(ctx, phone, label+"/settle")
		if err != nil {
			return Saved{}, fmt.Errorf("%w: %v", ErrSave, err)
		}
		has = named
	}
	res := Saved{HadName: out.Had, HasName: has, SyncedToPhone: syncToPhone, Waited: time.Since(start)}
	if !has {
		return res, ErrNotSaved
	}
	return res, nil
}

// named asks the page whether the number carries a name right now.
func (m *Manager) named(ctx context.Context, phone, label string) (bool, error) {
	// ESTE SCRIPT NAO ESTACIONA — devolve o JSON direto —, mas usa o prelude
	// pelos helpers, e o prelude CRIA o global. A chave e' gerada e liberada aqui
	// para que a correcao da H177 nao deixe um global orfao por chamada.
	key := nextStateKey()
	var raw string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label, func(c context.Context) error {
		return m.eval(c, nameStateScript(phone, key), &raw)
	}); err != nil {
		return false, err
	}
	var ignored string
	_ = m.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
		return m.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
	})
	var out struct {
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		Named bool   `json:"named"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return false, fmt.Errorf("addressbook: unexpected name-state answer: %w", e)
	}
	if !out.OK {
		return false, fmt.Errorf("%s", out.Why)
	}
	return out.Named, nil
}

// Delete removes a number from this account's contact list.
func (m *Manager) Delete(ctx context.Context, phone, label string) error {
	phone = normalizeNumber(phone)
	if phone == "" {
		return ErrNoNumber
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, deleteScript(phone, key), key, label+"/delete")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDelete, err)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return fmt.Errorf("addressbook: unexpected delete answer: %w", e)
	}
	if !out.OK {
		return fmt.Errorf("%w (%s)", ErrDelete, out.Why)
	}
	return nil
}

// DeviceCount reports how many devices a user has.
//
// IT IS A READ ABOUT SOMEBODY ELSE'S SETUP, and it answers zero for a user this
// account has never exchanged keys with — which is not the same as "that user
// has no devices". The distinction is kept in the error rather than folded into
// the number: an unknown user returns ErrDevices, and a known one returns a
// count that may legitimately be small.
//
// THE IDENTITY MUST BE THE RESOLVED ONE, and getting that wrong is invisible:
// this build files under LID, and the SAME peer answers "no device record" under
// its phone jid and a count of 5 under its LID — measured side by side in the
// same session (H148). Both answers are well-formed, which is exactly the
// problem: the phone-jid answer looks like a fact about the user and is a fact
// about the identity that was passed.
//
// Callers resolve first, with capabilities/lookup, and this method now REFUSES
// instead of trusting them to: a phone jid returns ErrUnresolvedIdentity.
//
// It still does not resolve on its own, and that half is the decision (66): a
// hidden network call inside a reader surprises whoever calls it in a loop. The
// registered question of H148 was answered by the orchestration rather than
// settled here.
func (m *Manager) DeviceCount(ctx context.Context, userJID, label string) (int, error) {
	if strings.TrimSpace(userJID) == "" {
		return 0, ErrNoNumber
	}
	// A RECUSA E' EXPLICITA (decisão 66), e substitui o aviso que este doc dava:
	// dizer "resolva antes" num comentário não impede ninguém de não resolver.
	if spa.IsUnresolvedIdentity(userJID) {
		return 0, ErrUnresolvedIdentity
	}
	key := nextStateKey()
	raw, err := m.parked(ctx, devicesScript(userJID, key), key, label+"/devices")
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDevices, err)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		Count int    `json:"count"`
		Known bool   `json:"known"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return 0, fmt.Errorf("addressbook: unexpected device answer: %w", e)
	}
	if !out.OK {
		return 0, fmt.Errorf("%w (%s)", ErrDevices, out.Why)
	}
	if !out.Known {
		return 0, fmt.Errorf("%w: the page has no device record for that user", ErrDevices)
	}
	return out.Count, nil
}

// normalizeNumber strips a jid suffix and everything that is not a digit.
//
// THE PAGE WANTS DIGITS. The reference documents "17182222222", and every jid
// this module handles arrives as "17182222222@c.us" — so a caller passing the
// jid it already has, which is every caller, would otherwise send a string the
// page does not recognise and get a contact saved under a number that does not
// exist.
func normalizeNumber(s string) string {
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Manager) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return m.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return m.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida (H177).
			var ignored string
			_ = m.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return m.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
