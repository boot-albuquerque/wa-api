package waheadless

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/capabilities/avatar"
	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAFetchesAvatarsForRealContacts proves the contract that the unit
// suite can only assert against a double: asking the live server for a dozen
// contacts produces BOTH outcomes — pictures and legitimate absences — and no
// errors.
//
// It feeds itself from listContacts rather than from hand-picked jids, which
// makes it an integration proof of the pair: the identity that capability
// chooses is the identity this one must accept.
//
// It logs counts and never a url or a contact.
func TestRealSPAFetchesAvatarsForRealContacts(t *testing.T) {
	requireRealSPA(t)
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Skip("WA_SEND_FROM_PROFILE is required for the live roster")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	roster, err := contacts.New(runner, eval).List(ctx, "real/avatar/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(roster.Contacts) < 12 {
		t.Skipf("the roster has %d contacts; this proof needs at least 12", len(roster.Contacts))
	}

	f := avatar.New(runner, eval)
	const sample = 12
	present, absent, tagged, withPreview, failed := 0, 0, 0, 0, 0
	for i := 0; i < sample; i++ {
		c := roster.Contacts[i]
		a, err := f.Fetch(ctx, c.Identity(), "real/avatar/fetch")
		if err != nil {
			// COUNT, do not abort. Failing on the first one hides whether the
			// problem is this contact or the whole path, and those need
			// different repairs.
			failed++
			kind := "lid"
			if c.LID == "" {
				kind = "pn"
			}
			t.Logf("contact %d (%s, merged=%t, %s): %v", i+1, kind, c.Merged, describeJID(c.Identity()), err)
			continue
		}
		if a.Present {
			present++
			if a.URL == "" {
				t.Fatal("Present=true with an empty url: the two must not disagree")
			}
			if a.PreviewURL != "" {
				withPreview++
			}
		} else {
			absent++
			if a.URL != "" || a.PreviewURL != "" {
				t.Fatalf("Present=false but a url came back: %s", a)
			}
		}
		if a.Tag != "" {
			tagged++
		}
	}
	t.Logf("of %d live contacts: present=%d absent=%d failed=%d tagged=%d withPreview=%d",
		sample, present, absent, failed, tagged, withPreview)

	if present+absent+failed != sample {
		t.Fatalf("present(%d)+absent(%d)+failed(%d) != asked(%d)", present, absent, failed, sample)
	}
	if failed > 0 {
		t.Fatalf("%d of %d contacts could not be answered at all", failed, sample)
	}
	if present == 0 {
		t.Fatal("not one of twelve contacts had a picture; the measurement found ten, " +
			"so a zero here means the request path stopped working, not that the " +
			"address book went blank")
	}
}

// describeJID says what CLASS an identity belongs to without saying which
// identity it is. When one contact of twelve behaves differently, "which one"
// is the question — and printing the number to answer it is exactly what the
// briefing forbids, so the shape is printed instead.
func describeJID(jid string) string {
	at := strings.Index(jid, "@")
	if at < 0 {
		return "malformed"
	}
	user, server := jid[:at], jid[at+1:]
	digits := true
	for _, r := range user {
		if r < '0' || r > '9' {
			digits = false
			break
		}
	}
	return fmt.Sprintf("server=%s userLen=%d allDigits=%t", server, len(user), digits)
}
