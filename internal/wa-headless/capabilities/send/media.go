package send

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Media sending, and EVERY CALL SHAPE HERE WAS READ, NOT GUESSED.
//
// The text path cost four measured corrections at exactly this layer (H34), so
// the signatures were taken from the build's own source before a line was
// written. Two of them would have been wrong by the obvious guess:
//
//	sendToChat({chat, earlyUpload, options})   ONE object, not (chat, opts)
//	WAWebMediaOpaqueData                        exists; WAWebOpaqueData is NULL here
//
// The rest, measured the same way:
//
//	prepRawMedia(file, opts)   opts branches on isPtt / asDocument / asGif /
//	                           isAudio / asSticker / asStickerPack, else UNKNOWN
//	createFromData(data, type) returns a Promise, and KEEPS the Blob it was
//	                           given when the type matches — which is why a File
//	                           survives and carries its filename through

// MaxMediaBytes bounds what this capability will attempt.
//
// Measured on this build: the page builds Blobs of 8 MB without complaint and
// round-trips 3 MB through base64 (a 4 MB string). The limit here is set below
// that on purpose — it is the size this module has EVIDENCE for, not the size
// nothing has failed at yet.
const MaxMediaBytes = 4 << 20

var (
	// ErrMediaEmpty is a send with no bytes. It is refused before the page is
	// touched, because an empty file is a caller mistake and not a transfer to
	// attempt.
	ErrMediaEmpty = fmt.Errorf("send: the media has no bytes")
	// ErrMediaTooLarge is a payload beyond what was measured.
	ErrMediaTooLarge = fmt.Errorf("send: the media is larger than this capability has been measured with")
	// ErrMediaNoMimeType is a send without a declared type. The page uses it to
	// decide what KIND of message this is, so guessing it here would be
	// choosing the recipient's experience by accident.
	ErrMediaNoMimeType = fmt.Errorf("send: the media has no mime type")
	// ErrMediaStickerConflict is asking for a sticker and a document at once.
	ErrMediaStickerConflict = fmt.Errorf("send: a sticker cannot also be a document")
	// ErrMediaStickerCaption is a caption on a sticker, which nobody would see.
	ErrMediaStickerCaption = fmt.Errorf("send: a sticker carries no caption")
)

// Media is one attachment to send.
type Media struct {
	// Filename travels with documents and is what the recipient sees. It is
	// carried by building a File rather than a Blob — createFromData keeps the
	// object it is handed when the type matches, so the name survives.
	Filename string
	// MimeType decides how the page classifies the message. Required.
	MimeType string
	Data     []byte
	// Caption is optional and ignored by kinds that cannot carry one.
	Caption string
	// AsDocument forces the document path even for something the page would
	// otherwise render inline. It is the difference between a photo in the
	// conversation and a file to download.
	AsDocument bool
	// AsSticker sends the bytes as a sticker.
	//
	// IT IS NOT THE STICKER ACTION. WAWebSendStickerAction.sendStickerToChat
	// exists, and the argument instrument measured what it wants:
	// (chat, {mediaData}) — a sticker MODEL that is already in the account's
	// collection. It re-sends a sticker somebody already has; it cannot carry
	// bytes. The path for bytes is this one, through prepRawMedia's asSticker
	// branch, which was already measured and documented above.
	//
	// WhatsApp expects WebP. Nothing here converts: a caller handing this PNG
	// bytes gets whatever the page decides, and guessing an encoder into the
	// middle of a send is not this package's job.
	AsSticker bool
}

// String redacts. Bytes and filenames are content.
func (m Media) String() string {
	return fmt.Sprintf("send.Media(name=%t mime=%s bytes=%d caption=%t asDocument=%t asSticker=%t)",
		m.Filename != "", m.MimeType, len(m.Data), m.Caption != "", m.AsDocument, m.AsSticker)
}

// validate refuses what should never reach the page.
func (m Media) validate() error {
	if len(m.Data) == 0 {
		return ErrMediaEmpty
	}
	if len(m.Data) > MaxMediaBytes {
		return fmt.Errorf("%w: %d bytes, limit %d", ErrMediaTooLarge, len(m.Data), MaxMediaBytes)
	}
	if strings.TrimSpace(m.MimeType) == "" {
		return ErrMediaNoMimeType
	}
	// A STICKER IS NOT A DOCUMENT, and the page branches on one flag at a time.
	// Sending both would let whichever the page checks first decide silently.
	if m.AsSticker && m.AsDocument {
		return ErrMediaStickerConflict
	}
	// A STICKER CARRIES NO CAPTION. Accepting one and dropping it would leave
	// the caller believing words were sent that nobody will ever see.
	if m.AsSticker && m.Caption != "" {
		return ErrMediaStickerCaption
	}
	return nil
}

const mediaStateKey = "__waHeadlessMediaResult"

// SendMedia sends an attachment and returns only after proving it was sent.
//
// Same contract as Text and for the same reason: the page accepting a call is
// not the account having sent anything (invariant 14). The verification is
// narrowed to media, because a text send in flight would otherwise satisfy it.
func SendMedia(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	toJID string, m Media, label string) (Result, error) {

	// REFUSE BEFORE TOUCHING THE PAGE. A caller's mistake must not become an
	// upload attempt, and the ordering is asserted in the tests.
	if err := m.validate(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(toJID) == "" {
		return Result{}, ErrNoChat
	}

	sentAt := time.Now().Add(-clockSkewAllowance)

	var kicked string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return eval(ctx, mediaScript(toJID, m), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
	}

	var out struct {
		Stage string `json:"stage"`
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		JID   string `json:"jid"`
	}
	// Media takes longer than text: the bytes have to be encrypted and
	// uploaded before the message exists at all. The budget is the verify
	// budget plus room for that upload, rather than the same number reused
	// because it was already there.
	deadline := time.Now().Add(verifyBudget + mediaUploadAllowance)
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return eval(ctx, mediaResultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("send: unexpected media answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled the upload within %s",
				ErrDispatch, verifyBudget+mediaUploadAllowance)
		}
		time.Sleep(verifyTick)
	}
	switch {
	case out.Stage == "resolve" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrDispatch, out.Stage, out.Why)
	}
	if out.JID == "" {
		return Result{}, fmt.Errorf("%w: the page reported success without a resolved identity", ErrDispatch)
	}
	return verify(ctx, runner, eval, out.JID, sentAt, label, kindMedia)
}

// mediaUploadAllowance is the extra time an upload needs over a text send.
// Var, not const, so tests can compress the clock.
var mediaUploadAllowance = 60 * time.Second

func mediaScript(toJID string, m Media) string {
	name := m.Filename
	if strings.TrimSpace(name) == "" {
		name = "file"
	}
	return `JSON.stringify((() => {
		window[` + strconv.Quote(mediaStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(mediaStateKey) + `] = v; };
		const resolveChat = ` + resolveChatExpr + `;
		(async () => {
		let stage = 'resolve';
		try {
			const r = await resolveChat(` + strconv.Quote(toJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			stage = 'decode';
			const b64 = ` + strconv.Quote(base64.StdEncoding.EncodeToString(m.Data)) + `;
			const bin = atob(b64);
			const bytes = new Uint8Array(bin.length);
			for (let i = 0; i < bin.length; i++) { bytes[i] = bin.charCodeAt(i); }
			// A File, not a Blob: createFromData keeps the object it is handed
			// when the type matches, so this is what carries the filename to
			// the recipient. A Blob would arrive nameless.
			const file = new File([bytes], ` + strconv.Quote(name) + `, { type: ` + strconv.Quote(m.MimeType) + ` });

			stage = 'opaque';
			// WAWebMediaOpaqueData, measured present. WAWebOpaqueData — the
			// name the reference would use — resolves to NULL on this build.
			const OD = window.require('` + string(spa.ModuleMediaOpaqueData) + `');
			const opaque = await OD.createFromData(file, ` + strconv.Quote(m.MimeType) + `);
			if (!opaque) { park({ stage, ok: false, why: 'OPAQUE_NULL' }); return; }

			stage = 'prep';
			const P = window.require('` + string(spa.ModulePrepRawMedia) + `');
			const prep = P.prepRawMedia(opaque, { asDocument: ` + strconv.FormatBool(m.AsDocument) + `,
				asSticker: ` + strconv.FormatBool(m.AsSticker) + ` });
			if (!prep) { park({ stage, ok: false, why: 'PREP_NULL' }); return; }
			// The upload and encryption happen here, and this is the slow part.
			await prep.waitForPrep();

			stage = 'dispatch';
			// ONE OBJECT, not (chat, options). Measured from the source:
			// function (t) { var e = t.chat, n = t.earlyUpload, r = t.options; ... }
			await prep.sendToChat({
				chat: r.chat,
				options: { caption: ` + strconv.Quote(m.Caption) + ` }
			});
			park({ stage: 'done', ok: true, why: '', jid: r.jid });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const mediaResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + mediaStateKey + `"` + `];
	if (!s) { return { stage: 'dispatch', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
