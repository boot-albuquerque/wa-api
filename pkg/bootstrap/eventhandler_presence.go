package bootstrap

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos de presença: quem está online e quem está digitando.

func (mycli *MyClient) handlePresence(evt *events.Presence, st *eventState) {
	st.postmap["type"] = "Presence"
	st.dowebhook = 1
	st.postmap["from"] = evt.From.String()
	if evt.Unavailable {
		st.postmap["state"] = "offline"
		if evt.LastSeen.IsZero() {
			log.Info().Str("from", evt.From.String()).Msg("User is now offline")
		} else {
			st.postmap["last_seen"] = evt.LastSeen.Unix()
			log.Info().Str("from", evt.From.String()).Str("lastSeen", fmt.Sprintf("%v", evt.LastSeen)).Msg("User is now offline")
		}
	} else {
		st.postmap["state"] = "online"
		log.Info().Str("from", evt.From.String()).Msg("User is now online")
	}
}

func (mycli *MyClient) handleChatPresence(evt *events.ChatPresence, st *eventState) {
	st.postmap["type"] = "ChatPresence"
	st.dowebhook = 1
	log.Info().Str("state", string(evt.State)).Str("media", string(evt.Media)).Str("chat", evt.MessageSource.Chat.String()).Str("sender", evt.MessageSource.Sender.String()).Msg("Chat Presence received")
}
