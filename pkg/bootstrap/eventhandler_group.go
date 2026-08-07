package bootstrap

import (
	"wa-api/internal/wa-noise/protocol/types/events"

	"github.com/rs/zerolog/log"
)

// Eventos de grupo e de newsletter — o que acontece com uma coleção de
// participantes, e não com um contato individual. Newsletter entra aqui
// porque, do ponto de vista do SDK, um canal é um JID coletivo como o de
// grupo, e não um contato.

func (evh *UserEventHandler) handleGroupInfo(evt *events.GroupInfo, st *eventState) {
	st.postmap["type"] = "GroupInfo"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Group info updated")
}

func (evh *UserEventHandler) handleJoinedGroup(evt *events.JoinedGroup, st *eventState) {
	st.postmap["type"] = "JoinedGroup"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Joined group")
}

func (evh *UserEventHandler) handleNewsletterJoin(evt *events.NewsletterJoin, st *eventState) {
	st.postmap["type"] = "NewsletterJoin"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter joined")
}

func (evh *UserEventHandler) handleNewsletterLeave(evt *events.NewsletterLeave, st *eventState) {
	st.postmap["type"] = "NewsletterLeave"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter left")
}

func (evh *UserEventHandler) handleNewsletterMuteChange(evt *events.NewsletterMuteChange, st *eventState) {
	st.postmap["type"] = "NewsletterMuteChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter mute changed")
}

func (evh *UserEventHandler) handleNewsletterLiveUpdate(evt *events.NewsletterLiveUpdate, st *eventState) {
	st.postmap["type"] = "NewsletterLiveUpdate"
	st.dowebhook = 1
	log.Info().Msg("Newsletter live update")
}
