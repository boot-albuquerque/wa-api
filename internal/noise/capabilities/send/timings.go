package send

import (
	"time"

	"github.com/rs/zerolog"
)

// DebugTimings e' o MessageDebugTimings historico da raiz, que continua
// expondo o nome antigo por apelido de tipo — ver o doc de Response.
//
// O apelido importa mais aqui do que nos outros dois tipos: MessageDebugTimings
// tem um METODO (MarshalZerologObject) e satisfaz zerolog.LogObjectMarshaler.
// Com apelido, o metodo continua sendo o mesmo metodo do mesmo tipo, e o
// `evt.Object(...)` de quem loga um SendResponse produz exatamente o mesmo
// JSON. Um tipo novo na raiz precisaria reimplementar o metodo, e as duas
// implementacoes poderiam divergir.
type DebugTimings struct {
	LIDFetch time.Duration
	Queue    time.Duration

	Marshal         time.Duration
	GetParticipants time.Duration
	GetDevices      time.Duration
	GroupEncrypt    time.Duration
	PeerEncrypt     time.Duration

	Send  time.Duration
	Resp  time.Duration
	Retry time.Duration
}

func (mdt DebugTimings) MarshalZerologObject(evt *zerolog.Event) {
	if mdt.LIDFetch != 0 {
		evt.Dur("lid_fetch", mdt.LIDFetch)
	}
	evt.Dur("queue", mdt.Queue)
	evt.Dur("marshal", mdt.Marshal)
	if mdt.GetParticipants != 0 {
		evt.Dur("get_participants", mdt.GetParticipants)
	}
	evt.Dur("get_devices", mdt.GetDevices)
	if mdt.GroupEncrypt != 0 {
		evt.Dur("group_encrypt", mdt.GroupEncrypt)
	}
	evt.Dur("peer_encrypt", mdt.PeerEncrypt)
	evt.Dur("send", mdt.Send)
	evt.Dur("resp", mdt.Resp)
	if mdt.Retry != 0 {
		evt.Dur("retry", mdt.Retry)
	}
}
