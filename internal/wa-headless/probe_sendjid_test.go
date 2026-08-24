package waheadless

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeOutgoingRemoteJID is a MEASUREMENT, not a guard. It answers one
// question the send verifier depends on and that no reasoning can settle:
// under WHICH remote_jid does this LID-first build store a message we sent?
//
// It prints no phone number. Identity is shown as server + local length + a
// short hash, which is enough to tell two jids apart without naming either.
func TestProbeOutgoingRemoteJID(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SENDJID") == "" {
		t.Skip("set WA_PROBE_SENDJID=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
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

	var raw string
	script := `JSON.stringify((() => {
		const coll = window.require('WAWebMsgCollection').MsgCollection;
		const all = coll.getModelsArray();
		const rows = [];
		let servers = {};
		for (let i = 0; i < all.length; i++) {
			const m = all[i]; const id = m && m.id; if (!id) continue;
			const sv = (id.remote && id.remote.server) || "?";
			servers[sv] = (servers[sv]||0)+1;
			rows.push({fromMe: !!id.fromMe, remoteServer: sv,
				remoteUser: (id.remote && id.remote.user) || null,
				t: m.t || 0, type: m.type || null, idx: i});
		}
		rows.sort((x,y) => y.t - x.t);
		return {total: all.length, servers: servers, top: rows.slice(0, 12)};
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	// Redact: replace every "remoteUser" value with server+len+hash8.
	t.Logf("raw length %d", len(raw))
	t.Logf("shape: %s", between(raw, `"total"`, `"top"`))
	for _, chunk := range strings.Split(raw, "},{") {
		u := between(chunk, `"remoteUser":"`, `"`)
		tag := "nil"
		if u != "" {
			s := sha256.Sum256([]byte(u))
			tag = "len=" + itoa(len(u)) + " h=" + hex.EncodeToString(s[:])[:8]
		}
		t.Logf("fromMe=%s server=%s user=%s t=%s type=%s",
			between(chunk, `"fromMe":`, `,`),
			between(chunk, `"remoteServer":`, `,`),
			tag,
			between(chunk, `"t":`, `,`),
			between(chunk, `"type":`, `}`))
	}
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	j := strings.Index(s, b)
	if j < 0 {
		return s
	}
	return s[:j]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
