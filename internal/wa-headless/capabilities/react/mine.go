package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// stateKeyMine is where the page parks the answer for the reaction read.
const stateKeyMine = "__waHeadlessReactMine"

// mineOn reports whether THIS ACCOUNT currently has a reaction on a message.
//
// IT IS THE POSTCONDITION Remove could not have. The sticky hasReaction flag
// answers "has ever had one" within a session, which is the wrong question for a
// removal, and the aggregate branch that was supposed to answer the right one
// never ran (H83). What was missing was a source — and the source exists: an
// async fetch keyed by the id OBJECT, measured in H154.
//
// IT ASKS THE SHARED EXPRESSION, not a copy. capabilities/message reports the
// same fact to callers, and two copies would drift into the one failure shape
// that hides itself: a removal that verifies here and reads back as present
// there, with both halves looking correct alone.
func (r *Reactor) mineOn(ctx context.Context, msgID, label string) (bool, error) {
	key := nextStateKey()

	var kicked string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/mine-kick", func(ctx context.Context) error {
		return r.eval(ctx, mineScript(msgID, key), &kicked)
	}); err != nil {
		return false, fmt.Errorf("%w: %v", ErrReact, err)
	}
	deadline := time.Now().Add(reactBudget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/mine-read", func(ctx context.Context) error {
			return r.eval(ctx, `window.`+stateKeyMine+` || ""`, &raw)
		}); err != nil {
			return false, fmt.Errorf("%w: %v", ErrReact, err)
		}
		if raw != "" {
			var out struct {
				OK       bool   `json:"ok"`
				Why      string `json:"why"`
				NotFound bool   `json:"notFound"`
				Mine     bool   `json:"mine"`
			}
			if err := json.Unmarshal([]byte(raw), &out); err != nil {
				return false, fmt.Errorf("react: unexpected mine answer: %w", err)
			}
			if out.NotFound {
				return false, ErrNoMessage
			}
			if !out.OK {
				return false, fmt.Errorf("%w (%s)", ErrReact, out.Why)
			}
			return out.Mine, nil
		}
		if !time.Now().Before(deadline) {
			return false, fmt.Errorf("%w: the page never settled within %s", ErrReact, reactBudget)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(reactTick):
		}
	}
}

func mineScript(msgID string, key string) string {
	return `(() => {
	window.` + stateKeyMine + ` = null;
	const park = v => { window.` + stateKeyMine + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const readReactions = ` + spa.ReactionsForMessageExpr + `;
	(async () => {
	try {
		const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
		const want = ` + strconv.Quote(msgID) + `;
		let m = null;
		for (const c of coll.getModelsArray()) {
			try { if (c.id && c.id.id === want) { m = c; break; } } catch (e) {}
		}
		if (!m) { park({ ok: true, notFound: true, mine: false }); return; }
		const r = await readReactions(m);
		if (!r.ok) { park({ ok: false, why: r.why, mine: false }); return; }
		// byMe VEM DA PAGINA. Derivar "e minha?" comparando o jid desta conta
		// com os remetentes e' a comparacao que ja deu errado duas vezes aqui,
		// e num build LID-first ela erra em silencio.
		let mine = false;
		for (const g of (r.groups || [])) { if (g && g.byMe) { mine = true; break; } }
		park({ ok: true, notFound: false, mine: mine });
	} catch (e) {
		park({ ok: false, why: safe(e), mine: false });
	}
	})();
	return "kicked";
	})()`
}
