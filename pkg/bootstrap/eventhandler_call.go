package bootstrap

import (
	"fmt"

	"wa-api/internal/noise/protocol/types/events"

	"github.com/rs/zerolog/log"
)

// Eventos de chamada. Os cinco apenas nomeiam o evento para o webhook e o
// registram no log — nenhum deles toca banco, sessão ou cache.

func (evh *UserEventHandler) handleCallOffer(evt *events.CallOffer, st *eventState) {
	st.postmap["type"] = "CallOffer"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer")
}

func (evh *UserEventHandler) handleCallAccept(evt *events.CallAccept, st *eventState) {
	st.postmap["type"] = "CallAccept"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call accept")
}

func (evh *UserEventHandler) handleCallTerminate(evt *events.CallTerminate, st *eventState) {
	st.postmap["type"] = "CallTerminate"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call terminate")
}

func (evh *UserEventHandler) handleCallOfferNotice(evt *events.CallOfferNotice, st *eventState) {
	st.postmap["type"] = "CallOfferNotice"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer notice")
}

func (evh *UserEventHandler) handleCallRelayLatency(evt *events.CallRelayLatency, st *eventState) {
	st.postmap["type"] = "CallRelayLatency"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call relay latency")
}
