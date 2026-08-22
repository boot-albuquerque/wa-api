// Package phone formats a number and names its country, using the page's own
// tables rather than a table of ours.
//
// THE PAGE DOES NOT REFUSE GARBAGE, AND THAT IS THE WHOLE PROBLEM. Measured
// 2026-08-22 against this build (probe_settings_test.go, TestProbePhoneShapes):
//
//	findCC("notaphone")  ->  "not"
//	findCC("1")          ->  "1"
//	formattedPhoneNumber("notaphone@s.whatsapp.net") -> a "+…" string, 11 chars
//
// It returns the first characters of whatever it was handed. A thin wrapper —
// which is what the reference is — would report a country code of "not" for junk
// and the caller would believe it, because the answer has exactly the shape a
// real answer has.
//
// So this package guards on BOTH sides: the input must be digits before the page
// is asked, and the answer must be digits before it is returned. The second
// check is the one that matters, because it catches shapes nobody measured.
package phone

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
	// ErrNotANumber is an input that is not a phone number.
	//
	// It never quotes the input. An error is a log line waiting to happen, and
	// this one would carry the very thing that must not be logged.
	ErrNotANumber = fmt.Errorf("phone: the input is not a phone number")
	// ErrRead is the page refusing or failing.
	ErrRead = fmt.Errorf("phone: the page refused the read")
	// ErrNonsenseAnswer is the page answering with something that is not a
	// country code.
	//
	// ITS OWN ERROR ON PURPOSE: it means the page's table accepted an input this
	// package's guard let through, which is a gap in the guard and not a caller
	// mistake. Merging it into ErrNotANumber would blame the caller for our bug.
	ErrNonsenseAnswer = fmt.Errorf("phone: the page answered something that is not a country code")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 20 * time.Second
	Tick   = 150 * time.Millisecond
)

// stateKeyPrefix names the page global a call parks its answer on.
//
// IT IS A PREFIX, NOT A KEY (H177). One shared global meant two concurrent
// calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer — measured in
// capabilities/message at 12 crossings in 12 rounds. The nonce comes from Go:
// a page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
const stateKeyPrefix = "__waHeadlessPhone"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// maxDigits is E.164's ceiling for a full international number. It is a
// BORROWED rule, not one measured here, and it is written down as such: the
// measurement above establishes that the page has no ceiling of its own.
const maxDigits = 15

// Reader answers about numbers.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// Number is what the page knows about one number.
type Number struct {
	// Formatted is the page's own rendering, e.g. with a leading + and spaces.
	Formatted string
	// CountryCode is the dialling code, digits only.
	CountryCode string
}

// String reports the SHAPE and never the value, because a Number is exactly the
// kind of thing that ends up in a log line by accident.
func (n Number) String() string {
	return fmt.Sprintf("phone.Number(formattedLen=%d cc=%q)", len(n.Formatted), n.CountryCode)
}

// Lookup formats a number and names its country in one page round trip.
func (r *Reader) Lookup(ctx context.Context, number, label string) (Number, error) {
	digits, ok := Digits(number)
	if !ok {
		return Number{}, ErrNotANumber
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, lookupScript(digits, key), key, label+"/phone")
	if err != nil {
		return Number{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK        bool   `json:"ok"`
		Why       string `json:"why"`
		Formatted string `json:"formatted"`
		CC        string `json:"cc"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Number{}, fmt.Errorf("phone: unexpected answer: %w", e)
	}
	if !out.OK {
		return Number{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	// THE ANSWER IS CHECKED, not just the input. findCC hands back the first
	// characters of whatever it was given, so an answer that is not digits means
	// the guard above let something through.
	if out.CC == "" || !allDigits(out.CC) {
		return Number{}, ErrNonsenseAnswer
	}
	return Number{Formatted: out.Formatted, CountryCode: out.CC}, nil
}

// Digits strips the decorations a caller may have typed and reports whether what
// is left is a plausible number.
//
// It accepts the jid suffixes this build uses, because callers hold jids rather
// than numbers — asking them to strip first would just move this function to
// every call site.
func Digits(number string) (string, bool) {
	s := strings.TrimSpace(number)
	for _, suffix := range []string{"@s.whatsapp.net", "@c.us", "@lid", "@g.us"} {
		if i := strings.Index(s, suffix); i >= 0 {
			s = s[:i]
			break
		}
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
			// decoration a person types; dropped rather than refused
		default:
			// ANYTHING ELSE IS A REFUSAL. "notaphone" must not reach the page,
			// because the page answers it.
			return "", false
		}
	}
	d := b.String()
	if d == "" || len(d) > maxDigits {
		return "", false
	}
	return d, true
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func (r *Reader) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return r.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
			return r.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida: sem isso a correcao troca uma
			// resposta cruzada por um global de pagina POR CHAMADA (H177).
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
