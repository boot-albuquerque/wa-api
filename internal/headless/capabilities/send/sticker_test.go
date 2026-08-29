package send

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestAStickerIsNotADocumentAndCarriesNoCaption. Both refusals are about the
// page branching on ONE flag: sending two would let whichever it checks first
// decide silently, and a caption on a sticker is words the caller believes were
// sent that nobody will ever see.
func TestAStickerIsNotADocumentAndCarriesNoCaption(t *testing.T) {
	body := []byte("RIFF....WEBPfake")
	if err := (Media{MimeType: "image/webp", Data: body, AsSticker: true, AsDocument: true}).validate(); !errors.Is(err, ErrMediaStickerConflict) {
		t.Fatalf("got %v, want ErrMediaStickerConflict", err)
	}
	if err := (Media{MimeType: "image/webp", Data: body, AsSticker: true, Caption: "olá"}).validate(); !errors.Is(err, ErrMediaStickerCaption) {
		t.Fatalf("got %v, want ErrMediaStickerCaption", err)
	}
	// A plain sticker is fine, and so is a captioned non-sticker.
	if err := (Media{MimeType: "image/webp", Data: body, AsSticker: true}).validate(); err != nil {
		t.Fatalf("a plain sticker was refused: %v", err)
	}
	if err := (Media{MimeType: "image/png", Data: body, Caption: "olá"}).validate(); err != nil {
		t.Fatalf("a captioned image was refused: %v", err)
	}
}

// TestTheStickerFlagReachesPrepRawMedia. asSticker is a branch in prepRawMedia,
// measured and documented on the module constant; a flag that never arrives is
// a flag that does nothing.
func TestTheStickerFlagReachesPrepRawMedia(t *testing.T) {
	on := mediaScript("1@c.us", Media{MimeType: "image/webp", Data: []byte("x"), AsSticker: true})
	if !strings.Contains(on, "asSticker: true") {
		t.Fatal("asSticker=true does not reach prepRawMedia")
	}
	off := mediaScript("1@c.us", Media{MimeType: "image/png", Data: []byte("x")})
	if !strings.Contains(off, "asSticker: false") {
		t.Fatal("asSticker=false does not reach prepRawMedia")
	}
	// AND IT IS NOT THE STICKER ACTION. sendStickerToChat wants a sticker MODEL
	// the account already has; it cannot carry bytes, and reaching for it is the
	// obvious wrong turn here.
	if strings.Contains(on, "sendStickerToChat") {
		t.Fatal("the byte path calls the action that re-sends an existing sticker")
	}
}

// TestTheRenderingSaysWhetherItIsASticker, without saying what it depicts.
func TestTheRenderingSaysWhetherItIsASticker(t *testing.T) {
	s := Media{Filename: "x.webp", MimeType: "image/webp", Data: []byte("abcd"), AsSticker: true}.String()
	if !strings.Contains(s, "asSticker=true") {
		t.Fatalf("the rendering does not say it is a sticker: %s", s)
	}
	if strings.Contains(s, "x.webp") || strings.Contains(s, "abcd") {
		t.Fatalf("the rendering carries the filename or the bytes: %s", s)
	}
	if !strings.Contains(s, "bytes=4") {
		t.Fatalf("the size is missing: %s", s)
	}
}

// TestNoConversionIsAttempted. WhatsApp expects WebP and this package does not
// encode: guessing an encoder into the middle of a send would make the failure
// mode "your sticker looks wrong" instead of "you sent the wrong bytes".
func TestNoConversionIsAttempted(t *testing.T) {
	script := mediaScript("1@c.us", Media{MimeType: "image/png", Data: []byte("x"), AsSticker: true})
	for _, ghost := range []string{"canvas", "toDataURL", "createImageBitmap", "OffscreenCanvas"} {
		if strings.Contains(script, ghost) {
			t.Fatalf("the script tries to convert the image (%s)", ghost)
		}
	}
	// The mime the caller gave is the mime that travels.
	if !strings.Contains(script, strconv.Quote("image/png")) {
		t.Fatal("the caller's mime type does not reach the page")
	}
}
