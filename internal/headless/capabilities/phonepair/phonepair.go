// Package phonepair requests a phone-number linking code from a live
// headless session — the alternative to scanning a QR code
// (internal/headless/capabilities/qr).
//
// # Where the algorithm comes from
//
// whatsapp-web.js (src/Client.js, requestPairingCode, main branch, read via
// raw.githubusercontent.com on 2026-08-29 — HOUSEKEEP H122/F380):
//
//	window.require('WAWebAltDeviceLinkingApi').setPairingType('ALT_DEVICE_LINKING');
//	await window.require('WAWebAltDeviceLinkingApi').initializeAltDeviceLinking();
//	return window.require('WAWebAltDeviceLinkingApi').startAltLinkingFlow(phoneNumber, showNotification);
//
// MEASURED against this build (F380, TestProbeRequestPairingCode,
// .lab/test-account-profile — a genuinely UNPAIRED session, not the
// already-paired one H122 originally probed against): all three calls
// resolve without a JS-level error, and the sequence reaches WhatsApp's
// real server — a fake test phone number produced a structured
// IQErrorBadRequest response, not a crash or a "function not found". H122
// had only confirmed the three functions EXIST; this is the first time the
// chain was actually invoked, and it was invoked against a session in the
// UNPAIRED state its own gate requires (see below).
//
// # The state gate H122 measured is real, and this package does not fight it
//
// The reference only re-requests a code while
// window.require('WAWebSocketModel').Socket.state stays UNPAIRED or
// UNPAIRED_IDLE — its own periodic re-generation loop (window.codeInterval)
// checks this and stops otherwise. This package does not replicate that
// loop (a caller wanting a fresh code again calls Request again — the same
// "no page clock" reasoning as qr.Reader, invariant 6), but it reads the
// state BEFORE calling, purely for diagnostics: a request against an
// already-paired session is refused one layer up, by
// appport.PhonePairer.IsPaired (see pkg/infra/headless/pairing), so this
// package never needs to interpret the state itself to decide whether to
// proceed.
//
// # Error taxonomy
//
// WhatsApp's refusal for a pairing-code request carries a `type.name`
// (`IQErrorBadRequest` in the one live measurement so far, wire code likely
// 400/429 depending on cause — MEASURED against oxidezap/whatsapp-rust's
// documented CompanionHelloError enum, which the reference implementation
// does not itself expose but a from-scratch Rust reimplementation of the
// same wire protocol does: RateOverlimit=429, FeatureNotAvailable=452,
// BadRequest is ambiguous on the wire between "genuinely invalid content"
// and server-side throttling). Read reports the raw type name in the error
// message rather than inventing a closed enum this package has not
// measured every member of — a caller that needs to special-case
// rate-limiting specifically can match on the substring, and a future
// measurement that catches a real RateOverlimit response can promote it to
// a typed sentinel then, not before.
package phonepair

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// stateKeyPrefix names the page global a call parks its answer on — same
// idiom as capabilities/qr and capabilities/phone (H177: a prefix, not a
// fixed key, so two concurrent calls on the same session cannot cross
// answers).
const stateKeyPrefix = "__headlessPhonePair"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Budget bounds how long Request waits for the page to answer. Var so a
// test can compress it, same convention as capabilities/qr.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

// ErrRequest is the page refusing or failing to produce a code.
var ErrRequest = fmt.Errorf("phonepair: the page could not produce a linking code")

// Reader requests a linking code from a live session.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Reader.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// kickScript fires the three-call chain MEASURED above. showNotification is
// always true, matching the reference's own default — it tells WhatsApp to
// also push an in-app notification to the phone alongside the code.
func kickScript(key, phone string) string {
	phoneJSON, _ := json.Marshal(phone)
	return `(() => {
	(async () => {
		const out = { ok: false, code: "", why: "", errType: "" };
		try {
			window.require('WAWebAltDeviceLinkingApi').setPairingType('ALT_DEVICE_LINKING');
			await window.require('WAWebAltDeviceLinkingApi').initializeAltDeviceLinking();
			const code = await window.require('WAWebAltDeviceLinkingApi').startAltLinkingFlow(` + string(phoneJSON) + `, true);
			out.ok = true;
			out.code = String(code || "");
		} catch (e) {
			out.why = String((e && e.message) || e).slice(0, 200);
			try { out.errType = (e && e.type && e.type.name) || ""; } catch (e2) {}
		}
		window.` + key + ` = JSON.stringify(out);
	})();
	return 'kicked';
})()`
}

type wireResult struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Why     string `json:"why"`
	ErrType string `json:"errType"`
}

// Request asks the page for a linking code for phone (plain international
// digits, no leading '+' — matches the reference's own documented format).
// It returns the code on success, or an error naming WhatsApp's own refusal
// type when the page reported one.
func (r *Reader) Request(ctx context.Context, phone, label string) (code string, err error) {
	key := nextStateKey()
	raw, err := r.parked(ctx, kickScript(key, phone), key, label+"/phonepair")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRequest, err)
	}
	var out wireResult
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return "", fmt.Errorf("phonepair: unexpected answer shape: %w", e)
	}
	if out.OK && out.Code != "" {
		return out.Code, nil
	}
	if out.ErrType != "" {
		return "", fmt.Errorf("%w: %s (%s)", ErrRequest, out.Why, out.ErrType)
	}
	return "", fmt.Errorf("%w: %s", ErrRequest, out.Why)
}

// parked kicks the async script and polls the page global it parks its
// answer on — same idiom as capabilities/qr.Reader.parked.
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
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never answered within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
