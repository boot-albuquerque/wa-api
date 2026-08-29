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

// TestProbeGroupMetadata measures the group metadata fields four ledger rows
// depend on: owner, createdAt, description and participants.
//
// They are PROPERTIES in the reference, not calls — populated from metadata this
// module already fetches for other reasons. That makes them cheap and makes the
// only real question the one this probe asks: WHERE the values live on this
// build, which has hidden them behind mixins and getters four times already
// (H83, H94, H103, H104).
//
// READ ONLY, and identity-free: field names, counts, domains and booleans.
func TestProbeGroupMetadata(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GROUPMETA") == "" {
		t.Skip("set WA_PROBE_GROUPMETA=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	script := `(() => {
		window.__gm = null;
		const park = v => { window.__gm = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 130);
		(async () => {
		const out = {};
		try {
			// REFRESCA ANTES DE LER, como a familia de pedidos de entrada
			// aprendeu: a metadata de boot nao tem o que mudou desde entao.
			await window.require('WAWebGroupQueryJob')
				.queryAndUpdateGroupMetadataById({ id: GROUP });
			const CC = window.require('WAWebChatCollection').ChatCollection;
			const chat = CC.get(GROUP);
			const md = chat && chat.groupMetadata;
			out.hasChat = !!chat; out.hasMetadata = !!md;
			if (md) {
				// NOMES CRUS E NOMES DE GETTER, lado a lado. O padrao deste
				// build e esconder o valor atras de um getter e deixar o campo
				// cru sob "__x_"; ler o errado devolve undefined para sempre.
				out.rawFields = Object.keys(md).filter(k => k.indexOf('__x_') === 0).slice(0, 30);
				const probe = (name, fn) => {
					try { const v = fn(); out[name] = v === undefined ? 'undefined'
						: (v === null ? 'null' : typeof v); } catch (e) { out[name] = 'threw'; }
				};
				probe('owner', () => md.owner);
				probe('ownerSerialized', () => md.owner && md.owner._serialized);
				probe('creation', () => md.creation);
				probe('desc', () => md.desc);
				probe('descId', () => md.descId);
				probe('descTime', () => md.descTime);
				probe('descOwner', () => md.descOwner);
				// QUINTA VEZ DO MESMO PADRAO: md.desc e undefined, e o campo cru
				// __x_displayedDesc existe. Nao ponho fallback — meco qual e o
				// nome, porque fallback que engole foi o defeito do react.go
				// (H83): o ramo errado roda sempre e ninguem ve.
				probe('displayedDesc', () => md.displayedDesc);
				out.descLength = (typeof md.displayedDesc === 'string')
					? md.displayedDesc.length : 'not a string';
				probe('participants', () => md.participants);
				try {
					const p = md.participants;
					const arr = p && (typeof p.getModelsArray === 'function' ? p.getModelsArray()
						: (typeof p.toArray === 'function' ? p.toArray() : null));
					out.participantCount = arr ? arr.length : 'no array door';
					if (arr && arr.length) {
						out.participantFields = Object.keys(arr[0]).filter(k => k.indexOf('__x_') === 0).slice(0, 20);
						const p0 = arr[0];
						out.participantIdDomain = (p0.id && p0.id.server) ? String(p0.id.server) : 'unknown';
						out.participantIsAdminType = typeof p0.isAdmin;
						out.participantIsSuperAdminType = typeof p0.isSuperAdmin;
					}
				} catch (e) { out.participantCount = 'threw: ' + safe(e); }
				// E os getters proprios, se existirem.
				try {
					const G = window.require('WAWebGroupMetadataGetters');
					out.gettersModule = Object.keys(G).slice(0, 25);
				} catch (e) { out.gettersModule = 'absent'; }
			}
		} catch (e) { out.fatal = safe(e); }
		out.finished = true;
		park(out);
		})();
		return 'kicked';
	})()`
	script = strings.ReplaceAll(script, "GROUP", strconv.Quote(gjid))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/gm-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/gm-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__gm || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(raw, `"finished":true`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("%s", raw)
}
