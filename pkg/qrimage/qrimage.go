// Package qrimage renders a WhatsApp pairing code into the PNG data URI that
// `GET /session/pair/qr` is documented to answer.
//
// # Why this exists as one package instead of one line per engine
//
// The route's contract is written down, and it is an IMAGE:
//
//	api/openapi/schemas/sessao.yaml:79 — qr_code:
//	  "Imagem do QR code em data URI (`data:image/png;base64,…`), lida da
//	   coluna `users.qrcode`."
//
// Until 2026-08-29 only ONE engine honoured it. noise did, because
// pkg/application/session/orchestrator.go encoded the PNG before writing
// users.qrcode. headless (HOUSEKEEP H145) returned the RAW pairing string
// from the same port, and nothing in the type system noticed: both are
// `string`, both flow through appport.PairingQRReader into
// domain.GetQRResult.QRCode, and both serialise to the same `qr_code` JSON
// field.
//
// That silent divergence is what F373 measured in the field: the dev panel,
// having been told the two engines answered the same shape, drew the string
// it got as QR PAYLOAD — so noise produced a picture-perfect QR code
// encoding the 1858 characters "data:image/png;base64,iVBORw0KG…", which a
// phone decodes successfully and WhatsApp then rejects as invalid.
//
// So the encode lives in ONE place that both engines call, for the same
// reason buildQRPayload exists in the orchestrator (F68): two encoders in two
// packages is exactly how the shapes drifted apart the first time.
package qrimage

import (
	"encoding/base64"
	"fmt"

	"github.com/skip2/go-qrcode"
)

const (
	// ImageSize is the side, in pixels, of the rendered PNG.
	//
	// 256 is the value pkg/application/session/orchestrator.go has always
	// used for noise, and the measured payload it produces is 1375 bytes
	// of PNG / 1858 characters of data URI (F373, live measurement against
	// GET /session/pair/qr). Keeping it means headless answers the same
	// size as noise, and no existing consumer sees its images change.
	ImageSize = 256

	// DataURIPrefix is the data URI header a client drops straight into an
	// <img src>. It is also the byte-for-byte marker a client can test to
	// tell a rendered image from a raw pairing string — see IsDataURI.
	DataURIPrefix = "data:image/png;base64,"

	// correction is the error-correction level. Medium is what noise has
	// always used; a pairing string is a couple of hundred bytes, three
	// orders of magnitude under Medium's ~2331-byte ceiling, so the level is
	// chosen for scan robustness and not for capacity.
	correction = qrcode.Medium
)

// Encode renders code as a PNG data URI.
//
// An empty code is NOT an error and returns an empty string: "no code right
// now" is a normal state of this route (the code rotates, and the gap between
// two codes is what a poller polls through — see the schema's own "vazio é
// informação, não defeito"). Encoding "" would otherwise hand callers a
// perfectly valid QR code for the empty string, which is the same class of
// bug F373 was about: a picture that decodes to something nobody asked for.
func Encode(code string) (string, error) {
	if code == "" {
		return "", nil
	}
	png, err := qrcode.Encode(code, correction, ImageSize)
	if err != nil {
		return "", fmt.Errorf("encode pairing QR image: %w", err)
	}
	return DataURIPrefix + base64.StdEncoding.EncodeToString(png), nil
}

// IsDataURI reports whether s is already a rendered image rather than a raw
// pairing string. Exposed so tests can assert the boundary in the same terms
// the contract states it.
func IsDataURI(s string) bool {
	return len(s) >= len(DataURIPrefix) && s[:len(DataURIPrefix)] == DataURIPrefix
}

// EnsureDataURI returns code as the rendered image the route promises,
// whatever shape the engine handed over.
//
// Both branches are MEASURED reality, not defensive padding:
//
//   - already an image — noise. Its adapter reads users.qrcode, and the
//     orchestrator's QR listener writes the encoded PNG into that column
//     (pkg/application/session/orchestrator.go, onPairingQR). Re-encoding it
//     would render a QR code whose payload is the 1858 characters
//     "data:image/png;base64,iVBORw0KG…" — a code that scans perfectly and
//     that WhatsApp rejects. That is F373, verbatim, and it is why this
//     function checks instead of encoding unconditionally.
//   - raw string — headless. Its adapter reads the live pairing code out
//     of the page (pkg/infra/headless/pairing.QRReader) and has no image
//     to hand over.
//
// Idempotent by construction: EnsureDataURI(EnsureDataURI(x)) == EnsureDataURI(x).
func EnsureDataURI(code string) (string, error) {
	if IsDataURI(code) {
		return code, nil
	}
	return Encode(code)
}
