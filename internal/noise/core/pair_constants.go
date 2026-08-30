package core

import "time"

// As constantes do fluxo de pareamento em si (QR data format, codigos de erro,
// parametros do link code) foram para internal/noise/pairing/constants.go na
// Fase F/G lote 4. O que sobrou aqui e' o canal de QR, que continua na raiz.

// Politica de emissao de QR codes por GetQRChannel.
const (
	// qrChannelBuffer e a capacidade do canal devolvido ao chamador.
	qrChannelBuffer = 8
	// qrCodeTimeout e a validade de cada QR code apos o primeiro.
	qrCodeTimeout = 20 * time.Second
	// qrCodeFirstTimeout is the validity of the first QR code, identified by
	// qrCodeFirstBatchSize codes still being queued. It intentionally equals
	// qrCodeTimeout: security parity with the officially measured behavior
	// (HOUSEKEEP.md F69 item 2). Upstream whatsmeow still uses 60s here,
	// undocumented; we diverge on purpose because a pairing QR is a
	// credential, and 3x the exposure window on the first code has no
	// offsetting benefit — the channel just rotates to the next code sooner,
	// same as the official client already does.
	qrCodeFirstTimeout   = qrCodeTimeout
	qrCodeFirstBatchSize = 6
)

// Nomes dos eventos emitidos no QRChannelItem.Event.
const (
	QRChannelEventCode  = "code"
	QRChannelEventError = "error"

	qrChannelEventSuccess                   = "success"
	qrChannelEventTimeout                   = "timeout"
	qrChannelEventUnexpectedState           = "err-unexpected-state"
	qrChannelEventClientOutdated            = "err-client-outdated"
	qrChannelEventScannedWithoutMultidevice = "err-scanned-without-multidevice"
)
