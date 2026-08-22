package waheadless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestChannelMetadataByInviteReal reads a channel by its invite code, and
// follows nothing.
//
// WHY IT NEEDED A HUMAN AT ALL. The newsletter collection is empty because this
// account follows nothing, so the model has no instance to inspect. The app's
// own discovery — getRecommendedNewsletters — does not answer, and does not
// answer its own 8s timeout either (H102). Inventing an invite link would have
// been guessing at somebody else's channel, so the code came from the human.
//
// THIS STAGE ONLY READS. If the metadata already carries the shape, following is
// unnecessary and the family unlocks without touching the account at all.
func TestChannelMetadataByInviteReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_CHANNEL") == "" {
		t.Skip("set WA_REAL_CHANNEL=1 and WA_CHANNEL_CODE=<invite code>")
	}
	code := os.Getenv("WA_CHANNEL_CODE")
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if code == "" || profile == "" {
		t.Fatal("WA_CHANNEL_CODE and WA_SEND_FROM_PROFILE are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	script := `(() => {
		window.__chan = null;
		const park = v => { window.__chan = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 150);
		(async () => {
		const out = {};
		try {
			const Q = window.require('WAWebNewsletterMetadataQueryJob');
			out.metaArity = Q.queryNewsletterMetadataByInviteCode.length;
			// ARIDADE 2 e o segundo argumento nao foi medido: duas formas, cada
			// uma parkeada antes de tentar, para que um travamento diga qual.
			for (const [nm, args] of [['codeOnly', [CODE]], ['codeAndTrue', [CODE, true]]]) {
				out['try_' + nm] = 'PENDING';
				park(out);
				try {
					const r = await Promise.race([
						Q.queryNewsletterMetadataByInviteCode.apply(null, args),
						new Promise((_, rej) => setTimeout(() => rej(new Error('TIMEOUT_12s')), 12000)),
					]);
					out['try_' + nm] = 'resolved: ' + typeof r;
					if (r && typeof r === 'object') {
						// NOMES E FORMAS. O nome do canal e publico, mas continua
						// sendo conteudo: o que cruza e a lista de CAMPOS, o
						// dominio do jid e booleanos.
						out.fields = Object.keys(r).slice(0, 30);
						const id = r.id || r.idJid || r.jid;
						out.idDomain = (id && id.server) ? String(id.server) :
							(typeof id === 'string' && id.indexOf('@') >= 0 ? id.split('@')[1] : 'none');
						out.hasName = !!(r.name || r.nameText);
						out.hasDescription = !!(r.description || r.descriptionText);
						out.subscribersType = typeof r.subscribersCount;
						out.stateType = typeof r.state;
						out.membershipType = typeof r.membership;
						// OS VALORES ESTAO DENTRO DOS MIXINS, e e por isso que
						// hasName veio falso: nao ha campo "name" no topo. A
						// estrutura e um conjunto de *MetadataMixin, cada um
						// carregando o seu pedaco — forma que a referencia nao
						// descreve. Uma camada abaixo, so NOMES de campo.
						out.mixins = {};
						for (const k of Object.keys(r)) {
							if (k.indexOf('Mixin') < 0) { continue; }
							try {
								const v = r[k];
								out.mixins[k] = (v && typeof v === 'object')
									? Object.keys(v).slice(0, 12)
									: (v === null ? 'null' : typeof v);
							} catch (e) { out.mixins[k] = 'threw'; }
						}
					}
					break;
				} catch (e) {
					out['try_' + nm] = 'threw: ' + safe(e);
					park(out);
				}
			}
		} catch (e) { out.setup = 'threw: ' + safe(e); }
		out.finished = true;
		park(out);
		})();
		return 'kicked';
	})()`
	script = strings.ReplaceAll(script, "CODE", strconv.Quote(code))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "chan/meta-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 80; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "chan/meta-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__chan || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(raw, `"finished":true`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(raw, `"finished":true`) {
		t.Logf("the read did NOT finish; whatever is PENDING below is the call that hung")
	}
	t.Logf("%s", raw)
}

// TestChannelByInviteCodeReal proves the delivered capability against a real
// channel, and follows nothing.
//
// The probe above measured the shape; this proves the reader built from it. The
// channel code comes from the human, because the app's own discovery does not
// answer (H102) and inventing a link would be guessing at somebody else's
// channel.
func TestChannelByInviteCodeReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_CHANNEL") == "" {
		t.Skip("set WA_REAL_CHANNEL=1 and WA_CHANNEL_CODE=<invite code or full link>")
	}
	code := os.Getenv("WA_CHANNEL_CODE")
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if code == "" || profile == "" {
		t.Fatal("WA_CHANNEL_CODE and WA_SEND_FROM_PROFILE are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	got, err := channel.New(runner, sess.Tab().Evaluate).ByInviteCode(ctx, code, "channel/read")
	if err != nil {
		t.Fatalf("ByInviteCode: %v", err)
	}
	t.Logf("%s", got)

	// THE POSTCONDITION IS WHAT A CHANNEL CANNOT LACK. A jid in the newsletter
	// namespace, and a name — a channel without either is not a channel, it is
	// a reader looking at the wrong field, which is exactly the defect the mixin
	// shape invites.
	if !strings.HasSuffix(got.JID, "@newsletter") {
		t.Errorf("jid is not in the newsletter namespace")
	}
	if got.Name == "" {
		t.Error("the channel has no name; the mixin read is looking at the wrong field")
	}
	// AND NOTHING WAS FOLLOWED. The whole point of this route is that it works
	// from outside; a true here would mean the read changed the account.
	if got.Following {
		t.Error("the account reads as following this channel; nothing here subscribes")
	}
	if got.Subscribers <= 0 {
		t.Logf("subscribers = %d — a public channel usually has some, so this is worth "+
			"noticing even though it is not necessarily wrong", got.Subscribers)
	}
}
