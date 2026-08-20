package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// subDouble routes by which script it was handed, the way the real page
// distinguishes an install from a drain.
type subDouble struct {
	installed bool
	reason    string
	// drains are returned in order, so a test can express a sequence — which is
	// the only way to exercise the reinstall path.
	drains   []string
	next     int
	installs int
}

func (p *subDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "s.buf = []") {
		if p.next < len(p.drains) {
			*out = p.drains[p.next]
			p.next++
			return nil
		}
		*out = `{"installed":true,"events":[],"dropped":0,"seen":0}`
		return nil
	}
	p.installs++
	if !p.installed {
		*out = fmt.Sprintf(`{"installed":false,"reason":%q}`, p.reason)
		return nil
	}
	*out = `{"installed":true,"already":false}`
	return nil
}

func sub(p *subDouble) *Subscription { return Subscribe(engine.NewRunner(), p.eval, 0) }

func evJSON(name, user, server, phoneUser, pushname string, psa bool) string {
	return fmt.Sprintf(`{"name":%q,"row":{"user":%q,"server":%q,"phone_user":%q,`+
		`"pushname":%q,"verified_name":"","is_business":false,"is_psa":%t,"is_user":true}}`,
		name, user, server, phoneUser, pushname, psa)
}

func drainJSON(dropped, seen int, events ...string) string {
	return fmt.Sprintf(`{"installed":true,"events":[%s],"dropped":%d,"seen":%d}`,
		strings.Join(events, ","), dropped, seen)
}

// TestTheSubscriptionBindsChangeAndNotOnlyAdd is the finding this file exists
// for. The message subscription binds 'add' and works; a roster changes by rows
// being UPDATED, and 90 seconds of live measurement saw change fire 7 times
// while add, remove, reset, update, sort, sync and destroy fired zero.
//
// Binding only 'add' here would install cleanly and deliver nothing — the
// failure mode with no symptom.
func TestTheSubscriptionBindsChangeAndNotOnlyAdd(t *testing.T) {
	p := &subDouble{installed: true}
	if err := sub(p).Install(context.Background(), "t/install"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// The assertion is on the script, because the double cannot emit an event
	// for a handler that was never bound — the absence would look like silence.
	script := Subscribe(engine.NewRunner(), p.eval, 0).installScript()
	if !strings.Contains(script, `coll.on('`+EventChange+`'`) {
		t.Fatalf("the subscription does not bind %q, which is the only event "+
			"measured to fire for a roster:\n%s", EventChange, script)
	}
	if !strings.Contains(script, `coll.on('`+EventAdd+`'`) {
		t.Fatalf("the subscription does not bind %q; ninety quiet seconds do not "+
			"prove a new contact never arrives that way", EventAdd)
	}
}

func TestDrainDecodesChangeEvents(t *testing.T) {
	p := &subDouble{installed: true, drains: []string{
		drainJSON(0, 2,
			evJSON(EventChange, "111", "c.us", "", "Ana", false),
			evJSON(EventAdd, "999", "lid", "111", "", false),
		),
	}}
	got, err := sub(p).Drain(context.Background(), "t/drain")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got.Events) != 2 {
		t.Fatalf("got %d events, want 2: %v", len(got.Events), got.Events)
	}
	if got.Events[0].Name != EventChange || got.Events[1].Name != EventAdd {
		t.Fatalf("the event names were not carried through: %v", got.Events)
	}
	if got.Events[0].Contact.PN != "111@c.us" {
		t.Fatalf("phone row decoded wrong: %+v", got.Events[0].Contact)
	}
	// A lid row carrying a phone still knows the number, even unmerged.
	if got.Events[1].Contact.LID != "999@lid" || got.Events[1].Contact.PN != "111@c.us" {
		t.Fatalf("lid row lost an identity: %+v", got.Events[1].Contact)
	}
	if got.Events[1].Contact.Merged {
		t.Fatal("a single-row event must not claim to be merged: there was no " +
			"second row to fold in")
	}
}

// TestThePSASentinelIsFilteredHereToo. H41 removed it from the listing. A
// subscription that let it through would put it back through the other door,
// which is exactly how a filter applied in one place fails.
func TestThePSASentinelIsFilteredHereToo(t *testing.T) {
	p := &subDouble{installed: true, drains: []string{
		drainJSON(0, 2,
			evJSON(EventChange, "0", "c.us", "", "", true),
			evJSON(EventChange, "111", "c.us", "", "Ana", false),
		),
	}}
	got, _ := sub(p).Drain(context.Background(), "t/drain")
	if len(got.Events) != 1 {
		t.Fatalf("the PSA sentinel survived the subscription: %v", got.Events)
	}
	if got.Seen != 2 {
		t.Fatalf("Seen=%d, want 2 — the counter reports what the handler received, "+
			"including what was filtered", got.Seen)
	}
}

// TestDroppedIsCarried: a full buffer means the sequence is truncated, and a
// caller reading a truncated sequence as complete is the silent-incompleteness
// failure this module keeps meeting.
func TestDroppedIsCarried(t *testing.T) {
	p := &subDouble{installed: true, drains: []string{
		drainJSON(17, 40, evJSON(EventChange, "111", "c.us", "", "Ana", false)),
	}}
	got, _ := sub(p).Drain(context.Background(), "t/drain")
	if got.Dropped != 17 || got.Seen != 40 {
		t.Fatalf("dropped=%d seen=%d, want 17 and 40", got.Dropped, got.Seen)
	}
	if !strings.Contains(got.String(), "dropped=17") {
		t.Fatalf("the rendering hides the truncation: %s", got)
	}
}

// TestALostSubscriptionIsReinstalledAndSaysSo. The events between the loss and
// the reinstall are gone and UNCOUNTABLE, so a caller must be able to tell a
// broken sequence from a quiet one.
func TestALostSubscriptionIsReinstalledAndSaysSo(t *testing.T) {
	p := &subDouble{installed: true, drains: []string{
		`{"installed":false}`,
		drainJSON(0, 1, evJSON(EventChange, "111", "c.us", "", "Ana", false)),
	}}
	got, err := sub(p).Drain(context.Background(), "t/drain")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !got.Reinstalled {
		t.Fatal("the subscription was lost and put back, and the drain did not say so")
	}
	if len(got.Events) != 1 {
		t.Fatalf("the drain after the reinstall was not returned: %v", got.Events)
	}
	if p.installs != 1 {
		t.Fatalf("installed %d times, want exactly 1", p.installs)
	}
}

func TestAPageWithoutTheCollectionCannotInstall(t *testing.T) {
	p := &subDouble{installed: false, reason: "NO_COLLECTION"}
	err := sub(p).Install(context.Background(), "t/install")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("got %v, want ErrNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "NO_COLLECTION") {
		t.Fatalf("the page's reason was dropped: %v", err)
	}
}

func TestEventRenderingRedacts(t *testing.T) {
	e := Event{Name: EventChange, Contact: Contact{PN: "5541999998888@c.us", Pushname: "Ana Silva"}}
	s := e.String()
	for _, secret := range []string{"5541999998888", "Ana", "Silva"} {
		if strings.Contains(s, secret) {
			t.Fatalf("String() leaked %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "name=change") {
		t.Fatalf("the event name must survive redaction: %s", s)
	}
}

func TestCancelledContextDoesNotDrain(t *testing.T) {
	p := &subDouble{installed: true, drains: []string{drainJSON(0, 0)}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sub(p).Drain(ctx, "t/drain"); err == nil {
		t.Fatal("a cancelled context produced a drain")
	}
}
