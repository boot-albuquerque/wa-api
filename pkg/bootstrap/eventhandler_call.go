package bootstrap

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de chamada. Os cinco apenas nomeiam o evento para o webhook e o
// registram no log — nenhum deles toca banco, sessão ou cache.

func (mycli *MyClient) handleCallOffer(evt *events.CallOffer, st *eventState) {
	st.postmap["type"] = "CallOffer"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer")
}

func (mycli *MyClient) handleCallAccept(evt *events.CallAccept, st *eventState) {
	st.postmap["type"] = "CallAccept"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call accept")
}

func (mycli *MyClient) handleCallTerminate(evt *events.CallTerminate, st *eventState) {
	st.postmap["type"] = "CallTerminate"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call terminate")
}

func (mycli *MyClient) handleCallOfferNotice(evt *events.CallOfferNotice, st *eventState) {
	st.postmap["type"] = "CallOfferNotice"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call offer notice")
}

func (mycli *MyClient) handleCallRelayLatency(evt *events.CallRelayLatency, st *eventState) {
	st.postmap["type"] = "CallRelayLatency"
	st.dowebhook = 1
	log.Info().Str("event", fmt.Sprintf("%+v", evt)).Msg("Got call relay latency")
}
