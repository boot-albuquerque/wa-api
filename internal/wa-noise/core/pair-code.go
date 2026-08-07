package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/pairing"
)

// PairClientType is the type of client to use with PairCode.
// The type is automatically filled based on store.DeviceProps.PlatformType (which is what QR login uses).
//
// Apelido de tipo, e nao tipo novo: internals.go (gerado, fora do escopo deste
// lote) cita PairClientType na assinatura de GetQRClientType/MakeQRData, e
// chamadores externos comparam com as constantes abaixo.
type PairClientType = pairing.ClientType

// As constantes continuam com o nome historico e o MESMO valor do subpacote.
const (
	PairClientUnknown        = pairing.ClientUnknown
	PairClientChrome         = pairing.ClientChrome
	PairClientEdge           = pairing.ClientEdge
	PairClientFirefox        = pairing.ClientFirefox
	PairClientIE             = pairing.ClientIE
	PairClientOpera          = pairing.ClientOpera
	PairClientSafari         = pairing.ClientSafari
	PairClientElectron       = pairing.ClientElectron
	PairClientUWP            = pairing.ClientUWP
	PairClientOtherWebClient = pairing.ClientOtherWebClient
	PairClientMacOS          = pairing.ClientMacOS
	PairClientAndroid        = pairing.ClientAndroid
)

// PairPhone generates a pairing code that can be used to link to a phone without scanning a QR code.
//
// You must connect the client normally before calling this (which means you'll also receive a QR code
// event, but that can be ignored when doing code pairing). You should also wait for `*events.QR` before
// calling this to ensure the connection is fully established. If using [Client.GetQRChannel], wait for
// the first item in the channel. Alternatively, sleeping for a second after calling Connect will probably work too.
//
// The exact expiry of pairing codes is unknown, but QR codes are always generated and the login websocket is closed
// after the QR codes run out, which means there's a 160-second time limit. It is recommended to generate the pairing
// code immediately after connecting to the websocket to have the maximum time.
//
// The clientType parameter must be one of the PairClient* constants, but which one doesn't matter.
// The client display name must be formatted as `Browser (OS)`, and only common browsers/OSes are allowed
// (the server will validate it and return 400 if it's wrong).
//
// See https://faq.whatsapp.com/1324084875126592 for more info
func (cli *Client) PairPhone(ctx context.Context, phone string, showPushNotification bool, clientType PairClientType, clientDisplayName string) (string, error) {
	if cli == nil {
		return "", ErrClientIsNil
	}
	return pairing.PairPhone(ctx, cli.pairT(), phone, showPushNotification, clientType, clientDisplayName)
}

func (cli *Client) tryHandleCodePairNotification(ctx context.Context, parentNode *waBinary.Node) {
	if cli == nil {
		return
	}
	pairing.TryHandleCodeNotification(ctx, cli.pairT(), parentNode)
}

func (cli *Client) handleCodePairNotification(ctx context.Context, parentNode *waBinary.Node) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return pairing.HandleCodeNotification(ctx, cli.pairT(), parentNode)
}
