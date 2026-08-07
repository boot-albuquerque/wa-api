package wanoise

import "time"

// As constantes do fluxo de pareamento em si (QR data format, codigos de erro,
// parametros do link code) foram para internal/wa-noise/pairing/constants.go na
// Fase F/G lote 4. O que sobrou aqui e' o canal de QR, que continua na raiz.

// Politica de emissao de QR codes por GetQRChannel.
const (
	// qrChannelBuffer e a capacidade do canal devolvido ao chamador.
	qrChannelBuffer = 8
	// qrCodeTimeout e a validade de cada QR code apos o primeiro.
	qrCodeTimeout = 20 * time.Second
	// qrCodeFirstTimeout e a validade do primeiro QR code, identificado por
	// ainda haver qrCodeFirstBatchSize codigos na fila.
	qrCodeFirstTimeout   = 60 * time.Second
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
