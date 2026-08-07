// Package socket implements a subset of the Noise protocol framework on top of websockets as used by WhatsApp.
//
// There shouldn't be any need to manually interact with this package.
// The Client struct in the top-level wa-noise package handles everything.
package socket

import (
	"errors"

	"github.com/coder/websocket"

	"wa-api/internal/wa-noise/protocol/binary/token"
)

const (
	// Origin is the Origin header for all WhatsApp websocket connections
	Origin = "https://web.whatsapp.com"
	// URL is the websocket URL for the new multidevice protocol
	URL = "wss://web.whatsapp.com/ws/chat"
)

// originHeaderName is the HTTP header carrying [Origin] on the websocket
// handshake request.
const originHeaderName = "Origin"

const (
	NoiseStartPattern = "Noise_XX_25519_AESGCM_SHA256\x00\x00\x00\x00"

	WAMagicValue = 6
)

var WAConnHeader = []byte{'W', 'A', WAMagicValue, token.DictVersion}

const (
	FrameMaxSize    = 1 << 24
	FrameLengthSize = 3
)

// statusForceClose is the sentinel passed to [FrameSocket.Close] to mean "don't
// send a websocket close frame, drop the connection now". It is not a real
// websocket status code — the websocket spec has no code 0 — and it doubles as
// the marker that tells [FrameSocket.Close] the disconnect was remote/unclean
// rather than a local, graceful shutdown.
const statusForceClose websocket.StatusCode = 0

// Parameters of the Noise_XX_25519_AESGCM_SHA256 handshake used by WhatsApp.
const (
	// noiseHashSize is the size in bytes of both the SHA-256 handshake hash and
	// of each key derived from it via HKDF. A handshake pattern exactly this
	// long is used verbatim as the initial hash instead of being hashed.
	noiseHashSize = 32
	// gcmIVSize is the AES-GCM nonce size in bytes (96 bits, the value both the
	// Noise spec and crypto/cipher's GCM default assume).
	gcmIVSize = 12
	// gcmIVCounterOffset is where the big-endian uint32 frame counter is written
	// inside the nonce: the counter occupies the last 4 of the gcmIVSize bytes,
	// leaving the leading bytes zero.
	gcmIVCounterOffset = gcmIVSize - 4
)

var (
	ErrFrameTooLarge     = errors.New("frame too large")
	ErrSocketClosed      = errors.New("frame socket is closed")
	ErrSocketAlreadyOpen = errors.New("frame socket is already open")
	ErrDialFailed        = errors.New("failed to dial whatsapp web websocket")
)

type ErrWithStatusCode struct {
	error
	StatusCode int
}
