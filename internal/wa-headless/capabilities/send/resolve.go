package send

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var _ = spa.ModuleWidFactory

// Resolution is what the identity-and-chat step found, WITHOUT sending
// anything.
//
// It is exported so the group path can be proven against a real account
// without messaging real people. A group chat has members, and a live proof
// that had to send in order to check the resolution would be a test that
// cannot be run.
type Resolution struct {
	// JID is the identity a message would actually be addressed to. For an
	// individual it is the lid the SERVER returned; for a group it is the
	// group jid itself, which has no lid counterpart.
	JID string
	// IsGroup says which of the two paths answered.
	IsGroup bool
}

// String redacts the identity, like everything else here.
func (r Resolution) String() string {
	return fmt.Sprintf("send.Resolution(jid=<redacted> resolved=%t group=%t)", r.JID != "", r.IsGroup)
}

// Resolve runs the identity-and-chat step and stops there.
//
// It shares resolveChatExpr with the send paths rather than reimplementing it,
// so a proof about Resolve is a proof about what a send would do — the whole
// point of exporting it.
func Resolve(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	toJID, label string) (Resolution, error) {

	var kicked string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return eval(ctx, resolveScript(toJID), &kicked)
	}); err != nil {
		return Resolution{}, fmt.Errorf("%w: %v", ErrNoChat, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		JID     string `json:"jid"`
		IsGroup bool   `json:"is_group"`
	}
	deadline := time.Now().Add(verifyBudget)
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return eval(ctx, resolveResultScript, &raw)
		}); err != nil {
			return Resolution{}, fmt.Errorf("%w: %v", ErrNoChat, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Resolution{}, fmt.Errorf("send: unexpected resolve answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Resolution{}, fmt.Errorf("%w: the page never settled the resolution", ErrNoChat)
		}
		time.Sleep(verifyTick)
	}
	if !out.OK {
		return Resolution{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	}
	return Resolution{JID: out.JID, IsGroup: out.IsGroup}, nil
}

const resolveStateKey = "__waHeadlessResolveResult"

func resolveScript(toJID string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(resolveStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(resolveStateKey) + `] = v; };
		const resolveChat = ` + resolveChatExpr + `;
		(async () => {
			try {
				const r = await resolveChat(` + strconv.Quote(toJID) + `);
				if (!r.ok) { park({ stage: 'done', ok: false, why: r.why }); return; }
				park({ stage: 'done', ok: true, why: '', jid: r.jid,
					is_group: !!(r.wid && r.wid.server === 'g.us') });
			} catch (e) {
				park({ stage: 'done', ok: false, why: String((e && e.message) || e).slice(0, 160) });
			}
		})();
		return { started: true };
	})())`
}

const resolveResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + resolveStateKey + `"` + `];
	if (!s) { return { stage: 'done', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
