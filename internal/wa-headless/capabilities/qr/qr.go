// Package qr assembles the pairing QR string from a live wa_headless
// session, the same contract pkg/infra/wa-noise/adapters/pairing/qr.go
// already serves for wa_noise: a raw string, never rendered here — the
// caller (or the eventual HTTP consumer) draws the code, this package only
// answers "what does the code say right now".
//
// # Where the algorithm comes from
//
// whatsapp-web.js (src/Client.js, main branch, read via
// raw.githubusercontent.com on 2026-08-29 — HOUSEKEEP H145):
//
//	registrationInfo = await window.require('WAWebSignalStoreApi').waSignalStore.getRegistrationInfo()
//	noiseKeyPair     = await window.require('WAWebUserPrefsInfoStore').waNoiseInfo.get()
//	staticKeyB64     = window.require('WABase64').encodeB64(noiseKeyPair.staticKeyPair.pubKey)
//	identityKeyB64   = window.require('WABase64').encodeB64(registrationInfo.identityKeyPair.pubKey)
//	advSecretKey     = await window.require('WAWebUserPrefsMultiDevice').getADVSecretKey()
//	platform         = window.require('WAWebCompanionRegClientUtils').DEVICE_PLATFORM
//	qr = ref + ',' + staticKeyB64 + ',' + identityKeyB64 + ',' + advSecretKey + ',' + platform
//
// MEASURED against this build, not copied on faith (H145,
// TestProbeQRConstructionSurface, .lab/test-account-profile): every module
// and every step of the chain above resolved and produced a value —
// getRegistrationInfo, waNoiseInfo.get, getADVSecretKey, encodeB64, and
// Conn.ref itself once the page had settled past PAIRING_LOADING (ref
// arrives around t+15s; a probe run before that sees it empty, which is why
// this package treats "no ref yet" as a legitimate, retryable answer — see
// Read's own doc comment).
//
// One divergence from the reference: WAWebCmd.Cmd.refreshQR does NOT exist
// in this build (measured false); WAWebLaunchSocketUtils.refreshQR does
// (measured true, and independently confirmed by H122) — this package does
// not refresh yet, but a future refresh path should use the latter, not the
// former.
package qr

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// stateKeyPrefix names the page global a call parks its answer on. A PREFIX,
// not a fixed key — see capabilities/phone's own doc comment on why a shared
// global lets two concurrent calls on the same session cross answers (H177).
const stateKeyPrefix = "__waHeadlessQR"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Budgets. Var so a test can compress them, same convention as
// capabilities/phone.
var (
	Budget = 20 * time.Second
	Tick   = 250 * time.Millisecond
)

// ErrRead is the page refusing or failing partway through the chain.
var ErrRead = fmt.Errorf("qr: the page could not assemble the code")

// Reader assembles the QR string from a live session.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Reader.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

func kickScript(key string) string {
	return `(() => {
	(async () => {
		const out = { ok: false, why: "", qr: "" };
		try {
			const conn = window.require('WAWebConnModel');
			const ref = conn && conn.Conn && conn.Conn.ref;
			if (!ref) { out.why = "no_ref"; window.` + key + ` = JSON.stringify(out); return; }
			const registrationInfo = await window.require('WAWebSignalStoreApi').waSignalStore.getRegistrationInfo();
			const noiseKeyPair = await window.require('WAWebUserPrefsInfoStore').waNoiseInfo.get();
			const b64 = window.require('WABase64').encodeB64;
			const staticKeyB64 = b64(noiseKeyPair.staticKeyPair.pubKey);
			const identityKeyB64 = b64(registrationInfo.identityKeyPair.pubKey);
			const advSecretKey = await window.require('WAWebUserPrefsMultiDevice').getADVSecretKey();
			const platform = window.require('WAWebCompanionRegClientUtils').DEVICE_PLATFORM;
			out.ok = true;
			out.qr = ref + ',' + staticKeyB64 + ',' + identityKeyB64 + ',' + advSecretKey + ',' + platform;
		} catch (e) {
			out.why = String((e && e.message) || e).slice(0, 140);
		}
		window.` + key + ` = JSON.stringify(out);
	})();
	return 'kicked';
})()`
}

type wireResult struct {
	OK  bool   `json:"ok"`
	Why string `json:"why"`
	QR  string `json:"qr"`
}

// Read assembles the current QR string, or reports why it could not.
//
// An empty string with a nil error means "no code on offer right now" —
// either the ref has not arrived yet or the caller polled a session that
// finished pairing between calls. Both are normal states, not failures.
func (r *Reader) Read(ctx context.Context, label string) (string, error) {
	key := nextStateKey()
	raw, err := r.parked(ctx, kickScript(key), key, label+"/qr")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out wireResult
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return "", fmt.Errorf("qr: unexpected answer shape: %w", e)
	}
	if !out.OK {
		if out.Why == "no_ref" {
			return "", nil
		}
		return "", fmt.Errorf("%w: %s", ErrRead, out.Why)
	}
	return out.QR, nil
}

// parked kicks the async script and polls the page global it parks its
// answer on, same idiom as capabilities/phone.Reader.parked.
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
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
