package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Groups this account and a contact both belong to.
//
// THE BODY OF findCommonGroups SAYS THREE THINGS a caller would otherwise learn
// by accident, and all three are reflected here rather than smoothed over:
//
//   - it returns null for THIS ACCOUNT's own contact. That is not an empty
//     answer, it is a refusal, and ErrIsSelf says so.
//   - it excludes parent (community) groups and locked ones. So the answer is
//     "groups you could talk in together", which is what a caller almost always
//     means, and the doc says it rather than leaving the difference invisible.
//   - it caches on the contact and reuses a pending promise. Asking twice is
//     cheap; asking after a membership change may not be fresh, which matters
//     on a build already measured not to refresh group metadata in-session
//     (H58).

var (
	// ErrCommonGroups is the page refusing or throwing.
	ErrCommonGroups = fmt.Errorf("contacts: the page refused to find common groups")
	// ErrNoContact is an identity that is not in the roster.
	ErrNoContact = fmt.Errorf("contacts: no such contact in the loaded roster")
	// ErrIsSelf is asking about this account itself. The page returns null and
	// this is what null means — a distinct answer from "no groups in common".
	ErrIsSelf = fmt.Errorf("contacts: this account has no groups in common with itself")
)

// CommonGroups is the answer.
type CommonGroups struct {
	// JIDs are the group identities. They are group jids, not people, so they
	// are safe to carry — but the RENDERING still shows only a count, because a
	// list of a person's groups is a profile of that person.
	JIDs []string
	// Waited is how long the page took.
	Waited time.Duration
}

func (c CommonGroups) String() string {
	return fmt.Sprintf("contacts.CommonGroups(count=%d waited=%s)",
		len(c.JIDs), c.Waited.Round(time.Millisecond))
}

const commonStateKey = "__waHeadlessCommonGroups"

// CommonGroupsWith lists the groups this account shares with a contact.
func (l *Lister) CommonGroupsWith(ctx context.Context, jid, label string) (CommonGroups, error) {
	if strings.TrimSpace(jid) == "" {
		return CommonGroups{}, ErrNoContact
	}
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, commonGroupsScript(jid), &kicked)
	}); err != nil {
		return CommonGroups{}, fmt.Errorf("%w: %v", ErrCommonGroups, err)
	}

	var out struct {
		Stage string   `json:"stage"`
		OK    bool     `json:"ok"`
		Why   string   `json:"why"`
		JIDs  []string `json:"jids"`
	}
	deadline := time.Now().Add(commonBudget)
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return l.eval(ctx, commonResultScript, &raw)
		}); err != nil {
			return CommonGroups{}, fmt.Errorf("%w: %v", ErrCommonGroups, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return CommonGroups{}, fmt.Errorf("contacts: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return CommonGroups{}, fmt.Errorf("%w: the page never settled within %s", ErrCommonGroups, commonBudget)
		}
		time.Sleep(commonTick)
	}

	switch {
	case out.Why == "IS_SELF":
		return CommonGroups{}, ErrIsSelf
	case out.Why == "NO_CONTACT" || out.Why == "NOT_ON_WHATSAPP" || out.Why == "WID_NULL":
		return CommonGroups{}, ErrNoContact
	case !out.OK:
		return CommonGroups{}, fmt.Errorf("%w at %s (%s)", ErrCommonGroups, out.Stage, out.Why)
	}
	// A CONTACT WITH NO GROUPS IN COMMON IS A SUCCESSFUL EMPTY ANSWER, not an
	// error. The refusal case is ErrIsSelf, and keeping them apart is the whole
	// reason the page's null was not flattened into "none".
	return CommonGroups{JIDs: out.JIDs, Waited: time.Since(start)}, nil
}

// Bounds. Var, not const, so tests can compress the clock.
var (
	commonBudget = 30 * time.Second
	commonTick   = 500 * time.Millisecond
)

func commonGroupsScript(jid string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(commonStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(commonStateKey) + `] = v; };
		(async () => {
		let stage = 'resolve';
		try {
			const r = await (` + spa.ResolveIdentityExpr + `)(` + strconv.Quote(jid) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			stage = 'contact';
			// THE CONTACT MODEL, because findCommonGroups unproxies its single
			// argument — this build's way of saying it wants a model.
			const CC = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			const contact = CC.get(r.wid) || CC.get(r.jid);
			if (!contact) { park({ stage, ok: false, why: 'NO_CONTACT' }); return; }

			stage = 'find';
			const A = window.require('` + string(spa.ModuleFindCommonGroupsContactAction) + `');
			const found = await A.findCommonGroups(contact);
			// NULL MEANS "THIS IS ME", not "none". The body returns
			// Promise.resolve(null) for getIsMe(contact) and a collection
			// otherwise, so collapsing the two would answer a refusal with an
			// empty list.
			if (found === null || found === undefined) {
				park({ stage, ok: false, why: 'IS_SELF' }); return;
			}
			const arr = found.getModelsArray ? found.getModelsArray() : (found || []);
			const jids = [];
			for (const g of arr) {
				try {
					const id = g && g.id;
					if (id && id._serialized) { jids.push(id._serialized); }
				} catch (e) {}
			}
			park({ stage: 'done', ok: true, why: '', jids: jids });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const commonResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + commonStateKey + `"` + `];
	if (!s) { return { stage: 'find', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
