package bootstrap

import (
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de grupo e de newsletter — o que acontece com uma coleção de
// participantes, e não com um contato individual. Newsletter entra aqui
// porque, do ponto de vista do SDK, um canal é um JID coletivo como o de
// grupo, e não um contato.

func (mycli *MyClient) handleGroupInfo(evt *events.GroupInfo, st *eventState) {
	st.postmap["type"] = "GroupInfo"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Group info updated")
}

func (mycli *MyClient) handleJoinedGroup(evt *events.JoinedGroup, st *eventState) {
	st.postmap["type"] = "JoinedGroup"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Joined group")
}

func (mycli *MyClient) handleNewsletterJoin(evt *events.NewsletterJoin, st *eventState) {
	st.postmap["type"] = "NewsletterJoin"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter joined")
}

func (mycli *MyClient) handleNewsletterLeave(evt *events.NewsletterLeave, st *eventState) {
	st.postmap["type"] = "NewsletterLeave"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter left")
}

func (mycli *MyClient) handleNewsletterMuteChange(evt *events.NewsletterMuteChange, st *eventState) {
	st.postmap["type"] = "NewsletterMuteChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.ID.String()).Msg("Newsletter mute changed")
}

func (mycli *MyClient) handleNewsletterLiveUpdate(evt *events.NewsletterLiveUpdate, st *eventState) {
	st.postmap["type"] = "NewsletterLiveUpdate"
	st.dowebhook = 1
	log.Info().Msg("Newsletter live update")
}
