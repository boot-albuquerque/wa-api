package testkit

import (
	"context"
	"errors"
	"testing"

	waclient "wa-api/pkg/infra/wa-noise/client"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// A DOUBLE THAT DRIFTS FROM ITS CONTRACT IS WORSE THAN NO DOUBLE, because every
// suite built on it keeps passing while the production interface moves. That is
// ARMADILHAS §1 stated as a compile-time assertion: if waclient.Client or
// store.ContactStore gains a method, this file stops building and names the
// gap, instead of the fake silently standing in for something it no longer is.
var (
	_ waclient.Client    = (*Fake)(nil)
	_ store.ContactStore = (*ContactStore)(nil)
)

// TestContactStoreReturnsWhatItWasGiven is the one behaviour this double has:
// it is a preloaded map. A double that lost the entries it was handed would
// make every test using it exercise the empty case while claiming otherwise.
func TestContactStoreReturnsWhatItWasGiven(t *testing.T) {
	jid := types.JID{User: "5541999990000", Server: types.DefaultUserServer}
	want := types.ContactInfo{PushName: "Ana", Found: true}

	f := &ContactStore{Contacts: map[types.JID]types.ContactInfo{jid: want}}

	got, err := f.GetContact(context.Background(), jid)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if got.PushName != want.PushName || !got.Found {
		t.Fatalf("GetContact returned %+v, want %+v", got, want)
	}

	all, err := f.GetAllContacts(context.Background())
	if err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetAllContacts returned %d entries, want 1", len(all))
	}
}

// TestContactStoreMissForAnUnknownJIDIsNotAnError. The production store
// distinguishes "no such contact" from "the lookup failed", and a double that
// collapsed them would let a caller ship code that treats an empty address book
// as a database outage — or the reverse.
func TestContactStoreMissForAnUnknownJIDIsNotAnError(t *testing.T) {
	f := &ContactStore{Contacts: map[types.JID]types.ContactInfo{}}
	got, err := f.GetContact(context.Background(), types.JID{User: "nobody", Server: types.DefaultUserServer})
	if err != nil {
		t.Fatalf("a missing contact produced an error: %v", err)
	}
	if got.Found {
		t.Fatalf("a missing contact came back as Found: %+v", got)
	}
}

// TestErrOnGetIsHonoured: the failure injection is the reason this double
// exists at all, so a version that ignored it would silently turn every
// error-path test into a happy-path test.
func TestErrOnGetIsHonoured(t *testing.T) {
	boom := errors.New("store is down")
	f := &ContactStore{ErrOnGet: boom}
	if _, err := f.GetAllContacts(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("got %v, want the injected error", err)
	}
}
