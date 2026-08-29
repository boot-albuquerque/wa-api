package headless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeAddressbook measures the address-book surface before anything is
// designed against it.
//
// READ ONLY. It loads modules and reports what they export and how many
// arguments they take. It does NOT call saveContactAction or
// deleteContactAction: both write to this account's address book, and a probe
// that changes the thing it is measuring is not a probe. The one read —
// getDeviceIds — is called, because a device count is not a mutation.
func TestProbeAddressbook(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_ADDRBOOK") == "" {
		t.Skip("set WA_PROBE_ADDRBOOK=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	kick := `(() => {
		window.__ab = null;
		const park = v => { window.__ab = JSON.stringify(v); };
		(async () => {
		const out = { modules: {}, arity: {}, contact: {}, devices: {} };
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m) : 'falsy';
				return m;
			} catch (e) {
				out.modules[name] = 'ABSENT: ' + (e && e.message);
				return null;
			}
		};
		try {
			const W = window.require('WAWebWidFactory');
			const Save = load('WAWebSaveContactAction');
			const Del = load('WAWebDeleteContactAction');
			const Dev = load('WAWebApiDeviceList');
			if (Save && Save.saveContactAction) out.arity.save = Save.saveContactAction.length;
			if (Del && Del.deleteContactAction) out.arity.del = Del.deleteContactAction.length;
			if (Dev && Dev.getDeviceIds) out.arity.devices = Dev.getDeviceIds.length;

			// What the peer's contact record looks like TODAY, in field names
			// and booleans only. It is the baseline the save has to move.
			const CC = window.require('WAWebContactCollection').ContactCollection;
			const wid = W.createWid(PEER_PLACEHOLDER);
			const c = CC.get(wid) || CC.get(PEER_PLACEHOLDER);
			out.contact.found = !!c;
			if (c) {
				out.contact.fields = Object.keys(c);
				// THE FLAGS THE PRIVACY FILTER WOULD CARE ABOUT, read through the
				// getters AND through the raw storage. The model keeps its
				// properties behind "__x_" and exposes them by getter; reading
				// only one of the two would report "absent" for a field that is
				// merely somewhere else.
				out.contact.isAddressBookContact = c.isAddressBookContact;
				out.contact.isMyContact = c.isMyContact;
				out.contact.raw_isAddressBookContact = c.__x_isAddressBookContact;
				out.contact.raw_isMyContact = c.__x_isMyContact;
				out.contact.raw_syncToAddressbook = c.__x_syncToAddressbook;
				out.contact.raw_isContactSyncCompleted = c.__x_isContactSyncCompleted;
				out.contact.hasName = !!c.name;
				out.contact.hasPushname = !!c.pushname;
				out.contact.hasShortName = !!c.shortName;
			}

			if (Dev && Dev.getDeviceIds) {
				try {
					const d = await Dev.getDeviceIds([wid]);
					out.devices.kind = Array.isArray(d) ? 'array[' + d.length + ']' : typeof d;
					if (Array.isArray(d) && d.length && d[0]) {
						out.devices.entryFields = Object.keys(d[0]);
						out.devices.count = d[0].devices && typeof d[0].devices === 'object'
							? Object.keys(d[0].devices).length : 'not an object';
					}
				} catch (e) {
					out.devices.threw = String(e && e.message);
				}
			}
		} catch (e) {
			out.fatal = String(e && e.message);
		}
		park(out);
		})();
		return 'kicked';
	})()`
	kick = strings.ReplaceAll(kick, "PEER_PLACEHOLDER", strconv.Quote(peer))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/ab-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 120; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/ab-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__ab || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never settled")
	}
	t.Logf("%s", raw)
}
