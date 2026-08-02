package bootstrap

import (
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

// Eventos sobre contatos: bloqueio, privacidade, avatar, recado e troca de
// identidade. Todos descrevem mudança no que a sessão sabe sobre um contato,
// e não no estado da própria sessão.

func (mycli *MyClient) handlePicture(evt *events.Picture, st *eventState) {
	st.postmap["type"] = "Picture"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Picture updated")
}

func (mycli *MyClient) handleBlocklistChange(evt *events.BlocklistChange, st *eventState) {
	st.postmap["type"] = "BlocklistChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Blocklist changed")
}

func (mycli *MyClient) handleBlocklist(evt *events.Blocklist, st *eventState) {
	st.postmap["type"] = "Blocklist"
	st.dowebhook = 1
	log.Info().Msg("Blocklist received")
}

func (mycli *MyClient) handlePrivacySettings(evt *events.PrivacySettings, st *eventState) {
	st.postmap["type"] = "PrivacySettings"
	st.dowebhook = 1
	log.Info().Msg("Privacy settings updated")
}

func (mycli *MyClient) handleUserAbout(evt *events.UserAbout, st *eventState) {
	st.postmap["type"] = "UserAbout"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("User about updated")
}

func (mycli *MyClient) handleIdentityChange(evt *events.IdentityChange, st *eventState) {
	st.postmap["type"] = "IdentityChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Identity changed")
}
