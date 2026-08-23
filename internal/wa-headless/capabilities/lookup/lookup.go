// Package lookup exposes, as a capability, the resolutions this module already
// performed INTERNALLY.
//
// WHY IT EXISTS AT ALL. Five ledger rows sat at PARTIAL with the same note —
// "existe como passo interno, não como capacidade exposta". The work was done
// and unreachable: send resolves an identity before dispatching, and no caller
// could ask the same question without sending something. The Phase 1 criterion
// the orchestration set (zero MISSING and zero ACTIONABLE PARTIAL) is what makes
// that a defect rather than a footnote: a PARTIAL caused by work not exposed is
// actionable, and this package is the action.
//
// IT ADDS NO NEW PAGE KNOWLEDGE. Every script here is one the module already
// runs; what is new is that a caller can run it on purpose. That is deliberate —
// a lookup package that invented its own resolution would be a second opinion
// competing with the one send actually uses, and the two would drift.
package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoJID is an empty input.
	ErrNoJID = fmt.Errorf("lookup: no jid given")
	// ErrNotOnWhatsApp is a number the server does not know.
	//
	// IT IS NOT AN ERROR OF OURS, and it is not the same as a failed read: the
	// question was asked and answered, and the answer is no. A caller deciding
	// whether to offer a "send" button needs that difference.
	ErrNotOnWhatsApp = fmt.Errorf("lookup: that number is not on WhatsApp")
	// ErrRead is the page refusing or failing.
	ErrRead = fmt.Errorf("lookup: the page refused the lookup")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 200 * time.Millisecond
)

// stateKeyPrefix names the page global a resolution parks its answer on.
//
// IT IS A PREFIX, NOT A KEY (H177/H178). One shared global meant two concurrent
// resolutions on the same session overwrote each other, and each polled until
// non-empty — so one could take the other's answer. Measured in
// capabilities/message at 12 crossings in 12 rounds; the shape here is
// identical, and this package is the one most likely to hit it, because it runs
// INSIDE other capabilities rather than only from a caller.
//
// The nonce comes from Go: a page-side Math.random or Date.now would put a
// decision and a clock where invariant 6 forbids them.
const stateKeyPrefix = "__waHeadlessLookup"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Identity is who the server says a jid is.
type Identity struct {
	// JID is the identity the SERVER returned, which on this build is usually a
	// LID and is NOT necessarily what was asked for. Comparing the two is how a
	// caller notices the resolution did something.
	JID string
	// AskedFor is the input, kept so the comparison above is possible without
	// the caller having to remember.
	AskedFor string
	// IsGroup says the input was a group, which short-circuits: the resolution
	// answers people, and asking it about a group would answer NOT_ON_WHATSAPP
	// for something that plainly exists (spa/identity.go).
	IsGroup bool
	// Resolved says the server returned an identity DIFFERENT from the input.
	Resolved bool
}

// String reports shape, never identity.
func (i Identity) String() string {
	return fmt.Sprintf("lookup.Identity(jid=%t group=%t resolved=%t)",
		i.JID != "", i.IsGroup, i.Resolved)
}

// Resolver answers identity questions.
type Resolver struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Resolver {
	return &Resolver{runner: runner, eval: eval}
}

// NumberID is the reference's getNumberId: does this number exist on WhatsApp,
// and under which identity.
//
// IT USES THE SAME EXPRESSION SEND USES, on purpose. A second resolution written
// here would be a competing answer, and the interesting failure would be the two
// disagreeing — which nobody would notice until a send failed for a number this
// package had just said was fine.
func (r *Resolver) NumberID(ctx context.Context, jid, label string) (Identity, error) {
	if strings.TrimSpace(jid) == "" {
		return Identity{}, ErrNoJID
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, resolveScript(jid, key), key, label+"/number-id")
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		JID     string `json:"jid"`
		IsGroup bool   `json:"isGroup"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Identity{}, fmt.Errorf("lookup: unexpected answer: %w", e)
	}
	if !out.OK {
		if out.Why == whyNotOnWhatsApp {
			return Identity{}, ErrNotOnWhatsApp
		}
		return Identity{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return Identity{
		JID: out.JID, AskedFor: jid, IsGroup: out.IsGroup,
		Resolved: out.JID != "" && out.JID != jid,
	}, nil
}

func (r *Resolver) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := r.runner.Do(ctx, engine.OpQuery, label+"/kick", func(c context.Context) error {
		return r.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpQuery, label+"/poll", func(c context.Context) error {
			return r.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida: sem isso, a correcao troca uma
			// resposta cruzada por um global de pagina POR RESOLUCAO, e este
			// pacote resolve dentro de outros — acumularia depressa.
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
