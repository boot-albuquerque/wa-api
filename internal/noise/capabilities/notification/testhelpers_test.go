package notification

import (
	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/protocol/types"
)

// fakeTransport e' o duble de Transport. Guarda os eventos despachados na
// ordem em que sairam — a ordem e' asserida por varios testes (um evento por
// filho de <picture>, no maximo um por <update> de mex).
type fakeTransport struct {
	events []any
}

func (f *fakeTransport) Log() sdklog.Logger { return sdklog.Noop }

func (f *fakeTransport) DispatchEvent(evt any) { f.events = append(f.events, evt) }

var (
	testPeerJID = types.NewJID("5511999999999", types.DefaultUserServer)
	testOwnJID  = types.NewJID("5511888888888", types.DefaultUserServer)
)
