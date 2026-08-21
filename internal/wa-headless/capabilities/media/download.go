// Package media downloads and decrypts a message's attachment.
//
// THE POSTCONDITION IS CRYPTOGRAPHIC, and that makes this the strongest
// verification in the whole module. Everywhere else a postcondition asks the
// page whether something happened and has to trust the answer; here the message
// model carries filehash — the SHA-256 of the PLAINTEXT — and bytes that
// decrypt to anything else fail a check no page behaviour can fake.
//
// THE CALL SHAPE came from the app's own call sites, because downloadManager's
// method is async and its toString() shows nothing:
//
//	downloadAndMaybeDecrypt({signal, directPath, encFilehash, filehash,
//	                         mediaKey, mediaKeyTimestamp, type})
//
// WHAT COMES BACK WAS NEVER MEASURED ON THIS BUILD, so this package does not
// assume: it accepts an ArrayBuffer, a typed array or a Blob, and REPORTS which
// one it got. A capability that cannot say what it received cannot be checked.
//
// THE CEILING IS REAL. Bytes cross the CDP boundary as base64, which costs a
// third more than the payload, so an unbounded download would be an unbounded
// string in a message channel sized for control traffic.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Bounds. Var, not const, so tests can compress the clock.
var (
	downloadBudget = 120 * time.Second
	downloadTick   = 500 * time.Millisecond
)

// MaxBytes is the ceiling on a decoded attachment. It matches what the send
// side will accept, so a round trip through this module cannot produce
// something this module refuses to handle.
const MaxBytes = 4 << 20

var (
	// ErrDownload is the page refusing or throwing.
	ErrDownload = fmt.Errorf("media: the page refused the download")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("media: no such message in the loaded collection")
	// ErrNotMedia is a message with nothing to download.
	ErrNotMedia = fmt.Errorf("media: that message carries no attachment")
	// ErrNotOnPhone is the app's own MediaNotOnPhone condition: the sender's
	// device no longer holds it, and no retry will change that.
	ErrNotOnPhone = fmt.Errorf("media: the media is no longer available from the sender's device")
	// ErrTooLarge is the ceiling above.
	ErrTooLarge = fmt.Errorf("media: the attachment is larger than this package will carry across the page boundary")
	// ErrHashMismatch is the cryptographic postcondition failing. It is its own
	// error because it means something different from every other failure here:
	// bytes arrived and they are not the bytes that were sent.
	ErrHashMismatch = fmt.Errorf("media: the decrypted bytes do not match the message's filehash")
)

// Attachment is a downloaded, decrypted attachment.
type Attachment struct {
	// Bytes is the plaintext.
	Bytes []byte
	// MIME is what the message says it is. It is the SENDER's claim, not a
	// sniffed type, and a caller writing this to disk should treat it as such.
	MIME string
	// SHA256 is the verified hash, hex. It equals the message's own filehash —
	// that equality is the postcondition, and this field exists so a caller can
	// record what was verified rather than take "it passed" on faith.
	SHA256 string
	// PageShape says what the page handed back — "arraybuffer", "typedarray" or
	// "blob". Reported because it was never measured on this build, and a
	// capability that cannot say what it received cannot be checked.
	PageShape string
	Waited    time.Duration
}

func (a Attachment) String() string {
	return fmt.Sprintf("media.Attachment(bytes=%d mime=%s sha256=%s… shape=%s waited=%s)",
		len(a.Bytes), a.MIME, firstN(a.SHA256, 8), a.PageShape, a.Waited.Round(time.Millisecond))
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Downloader downloads attachments on one session.
type Downloader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Downloader.
func New(runner *engine.Runner, eval spa.Evaluator) *Downloader {
	return &Downloader{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessMediaDownload"

// Get downloads and decrypts the attachment of one message.
func (d *Downloader) Get(ctx context.Context, msgID, label string) (Attachment, error) {
	if strings.TrimSpace(msgID) == "" {
		return Attachment{}, ErrNoMessage
	}
	start := time.Now()

	var kicked string
	if err := d.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return d.eval(ctx, downloadScript(msgID), &kicked)
	}); err != nil {
		return Attachment{}, fmt.Errorf("%w: %v", ErrDownload, err)
	}

	var out struct {
		Stage    string `json:"stage"`
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		B64      string `json:"b64"`
		MIME     string `json:"mime"`
		FileHash string `json:"filehash"`
		Shape    string `json:"shape"`
		Size     int    `json:"size"`
	}
	deadline := time.Now().Add(downloadBudget)
	for {
		var raw string
		if err := d.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return d.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Attachment{}, fmt.Errorf("%w: %v", ErrDownload, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Attachment{}, fmt.Errorf("media: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Attachment{}, fmt.Errorf("%w: the page never settled within %s", ErrDownload, downloadBudget)
		}
		time.Sleep(downloadTick)
	}

	switch {
	case out.Why == "NOT_LOADED":
		return Attachment{}, ErrNoMessage
	case out.Why == "NOT_MEDIA":
		return Attachment{}, ErrNotMedia
	case out.Why == "TOO_LARGE":
		return Attachment{}, fmt.Errorf("%w (%d bytes, ceiling %d)", ErrTooLarge, out.Size, MaxBytes)
	case strings.Contains(out.Why, "MediaNotOnPhone"):
		return Attachment{}, ErrNotOnPhone
	case !out.OK:
		return Attachment{}, fmt.Errorf("%w at %s (%s)", ErrDownload, out.Stage, out.Why)
	}

	body, err := base64.StdEncoding.DecodeString(out.B64)
	if err != nil {
		return Attachment{}, fmt.Errorf("media: the page's base64 did not decode: %w", err)
	}
	if len(body) > MaxBytes {
		return Attachment{}, fmt.Errorf("%w (%d bytes, ceiling %d)", ErrTooLarge, len(body), MaxBytes)
	}

	// THE CRYPTOGRAPHIC POSTCONDITION. filehash is the SHA-256 of the
	// plaintext, base64 on the model; comparing it here means the bytes are the
	// bytes that were sent, and nothing the page could do would forge that.
	sum := sha256.Sum256(body)
	want, err := base64.StdEncoding.DecodeString(out.FileHash)
	if err != nil {
		return Attachment{}, fmt.Errorf("media: the message's filehash did not decode: %w", err)
	}
	if len(want) != len(sum) || string(want) != string(sum[:]) {
		return Attachment{}, fmt.Errorf("%w (%d bytes downloaded)", ErrHashMismatch, len(body))
	}

	return Attachment{Bytes: body, MIME: out.MIME,
		SHA256: fmt.Sprintf("%x", sum), PageShape: out.Shape,
		Waited: time.Since(start)}, nil
}

func downloadScript(msgID string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			let msg = null;
			for (const m of MC.getModelsArray()) {
				try { if (m.id && m.id.id === ` + strconv.Quote(msgID) + `) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }
			if (!msg.directPath || !msg.mediaKey || !msg.filehash) {
				park({ stage, ok: false, why: 'NOT_MEDIA' }); return;
			}
			// REFUSE BEFORE DOWNLOADING when the message says how big it is.
			// Fetching four megabytes in order to reject them wastes the
			// sender's bandwidth as well as ours.
			if (msg.size && Number(msg.size) > ` + strconv.Itoa(MaxBytes) + `) {
				park({ stage, ok: false, why: 'TOO_LARGE', size: Number(msg.size) }); return;
			}

			stage = 'download';
			const DM = window.require('` + string(spa.ModuleDownloadManager) + `').downloadManager;
			// downloadQpl IS REQUIRED, and omitting it threw
			// "Cannot read properties of undefined (reading 'addAnnotations')"
			// — the corrected rule from H57 reading exactly right: the field
			// name says WHERE the missing object lives, and addAnnotations
			// lives on a QPL. Every call site in the app passes one; none of
			// them is optional about it.
			const Q = window.require('` + string(spa.ModuleStartMediaDownloadQpl) + `');
			const qpl = Q.startMediaDownloadQpl({ entryPoint: 'wa-headless' });
			const got = await DM.downloadAndMaybeDecrypt({
				signal: new AbortController().signal,
				downloadQpl: qpl,
				directPath: msg.directPath,
				encFilehash: msg.encFilehash,
				filehash: msg.filehash,
				mediaKey: msg.mediaKey,
				mediaKeyTimestamp: msg.mediaKeyTimestamp,
				type: msg.type
			});

			stage = 'encode';
			// WHAT COMES BACK WAS NEVER MEASURED HERE, so all three plausible
			// shapes are handled and the one that occurred is REPORTED.
			let bytes = null, shape = 'unknown';
			if (got instanceof ArrayBuffer) { bytes = new Uint8Array(got); shape = 'arraybuffer'; }
			else if (got && got.buffer instanceof ArrayBuffer) { bytes = new Uint8Array(got.buffer, got.byteOffset || 0, got.byteLength); shape = 'typedarray'; }
			else if (got && typeof got.arrayBuffer === 'function') { bytes = new Uint8Array(await got.arrayBuffer()); shape = 'blob'; }
			else { park({ stage, ok: false, why: 'UNKNOWN_RESULT_SHAPE' }); return; }

			if (bytes.length > ` + strconv.Itoa(MaxBytes) + `) {
				park({ stage, ok: false, why: 'TOO_LARGE', size: bytes.length }); return;
			}
			// Chunked so a large attachment does not blow the argument limit of
			// String.fromCharCode, which is a real ceiling and not a large one.
			let bin = '';
			const CHUNK = 0x8000;
			for (let i = 0; i < bytes.length; i += CHUNK) {
				bin += String.fromCharCode.apply(null, bytes.subarray(i, i + CHUNK));
			}
			park({ stage: 'done', ok: true, why: '',
				b64: btoa(bin), mime: msg.mimetype || '',
				filehash: msg.filehash, shape: shape, size: bytes.length });
		} catch (e) {
			const name = (e && e.name) ? e.name : '';
			park({ stage, ok: false,
				why: (name ? name + ': ' : '') + String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'download', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
