package pairing

import (
	"encoding/base64"
	"fmt"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/proto/waCompanionReg"
	"wa-api/internal/noise/protocol/proto/waWa6"
)

// ClientType is the type of client to use with PairCode.
// The type is automatically filled based on store.DeviceProps.PlatformType (which is what QR login uses).
//
// Exposto na raiz como PairClientType, por apelido de tipo: internals.go
// (gerado) cita o nome antigo numa assinatura.
type ClientType string

// Os valores nao mudaram; so' o prefixo `PairClient` virou o nome do pacote.
const (
	ClientUnknown        ClientType = "0"
	ClientChrome         ClientType = "1"
	ClientEdge           ClientType = "2"
	ClientFirefox        ClientType = "3"
	ClientIE             ClientType = "4"
	ClientOpera          ClientType = "5"
	ClientSafari         ClientType = "6"
	ClientElectron       ClientType = "7"
	ClientUWP            ClientType = "8"
	ClientOtherWebClient ClientType = "9"
	ClientMacOS          ClientType = "c"
	ClientAndroid        ClientType = "e"
)

// DetectClientType deduz o tipo de cliente do QR a partir do que o chamador
// configurou e, na falta disso, das DeviceProps globais do store.
//
// Era Client.getQRClientType. `configured` e' o Client.QRClientType.
func DetectClientType(configured ClientType) ClientType {
	if configured != "" {
		return configured
	}
	switch store.DeviceProps.GetPlatformType() {
	case waCompanionReg.DeviceProps_CHROME:
		return ClientChrome
	case waCompanionReg.DeviceProps_FIREFOX:
		return ClientFirefox
	case waCompanionReg.DeviceProps_EDGE:
		return ClientEdge
	case waCompanionReg.DeviceProps_IE:
		return ClientIE
	case waCompanionReg.DeviceProps_OPERA:
		return ClientOpera
	case waCompanionReg.DeviceProps_SAFARI:
		return ClientSafari
	case waCompanionReg.DeviceProps_UWP:
		return ClientUWP
	case waCompanionReg.DeviceProps_ANDROID_PHONE:
		return ClientAndroid
	}
	switch store.BaseClientPayload.UserAgent.GetPlatform() {
	case waWa6.ClientPayload_UserAgent_WEB:
		return ClientOtherWebClient
	case waWa6.ClientPayload_UserAgent_MACOS:
		return ClientMacOS
	default:
		return ClientUnknown
	}
}

// MakeQRData monta o payload do QR code lido pelo celular. Era
// Client.makeQRData.
func MakeQRData(t Transport, ref []byte, clientType ClientType) string {
	noise := base64.StdEncoding.EncodeToString(t.Store().NoiseKey.Pub[:])
	identity := base64.StdEncoding.EncodeToString(t.Store().IdentityKey.Pub[:])
	adv := base64.StdEncoding.EncodeToString(t.Store().AdvSecretKey)
	return fmt.Sprintf(qrDataFormat, ref, noise, identity, adv, clientType)
}
