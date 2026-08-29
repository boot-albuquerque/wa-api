package bootstrap

import (
	"wa-api/internal/noise/protocol/types/events"

	"github.com/rs/zerolog/log"
)

// Eventos sobre contatos: bloqueio, privacidade, avatar, recado e troca de
// identidade. Todos descrevem mudança no que a sessão sabe sobre um contato,
// e não no estado da própria sessão.

func (evh *UserEventHandler) handlePicture(evt *events.Picture, st *eventState) {
	st.postmap["type"] = "Picture"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Picture updated")
}

// handlePushName despacha a mudança de nome de um CONTACTO (F73).
//
// Não confundir com PushNameSetting, que é o nome do PRÓPRIO utilizador e já
// era assinável. O SDK persiste antes de emitir, portanto não há dado a
// gravar aqui — o que faltava era a NOTIFICAÇÃO.
//
// O nome ANTIGO vai no payload junto com o novo: quem mantém um cache local
// precisa de saber o que substituir, e sem ele teria de adivinhar qual das
// suas entradas mudou.
func (evh *UserEventHandler) handlePushName(evt *events.PushName, st *eventState) {
	st.postmap["type"] = "PushName"
	st.postmap["jid"] = evt.JID.String()
	st.postmap["old_push_name"] = evt.OldPushName
	st.postmap["new_push_name"] = evt.NewPushName
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).
		Str("old", evt.OldPushName).Str("new", evt.NewPushName).
		Msg("Contact push name changed")
}

// handleBusinessName despacha a mudança do nome verificado de uma conta
// comercial (F73). Mesma forma do handlePushName.
func (evh *UserEventHandler) handleBusinessName(evt *events.BusinessName, st *eventState) {
	st.postmap["type"] = "BusinessName"
	st.postmap["jid"] = evt.JID.String()
	st.postmap["old_business_name"] = evt.OldBusinessName
	st.postmap["new_business_name"] = evt.NewBusinessName
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).
		Str("old", evt.OldBusinessName).Str("new", evt.NewBusinessName).
		Msg("Contact business name changed")
}

func (evh *UserEventHandler) handleBlocklistChange(evt *events.BlocklistChange, st *eventState) {
	st.postmap["type"] = "BlocklistChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Blocklist changed")
}

func (evh *UserEventHandler) handleBlocklist(evt *events.Blocklist, st *eventState) {
	st.postmap["type"] = "Blocklist"
	st.dowebhook = 1
	log.Info().Msg("Blocklist received")
}

func (evh *UserEventHandler) handlePrivacySettings(evt *events.PrivacySettings, st *eventState) {
	st.postmap["type"] = "PrivacySettings"
	st.dowebhook = 1
	log.Info().Msg("Privacy settings updated")
}

func (evh *UserEventHandler) handleUserAbout(evt *events.UserAbout, st *eventState) {
	st.postmap["type"] = "UserAbout"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("User about updated")
}

func (evh *UserEventHandler) handleIdentityChange(evt *events.IdentityChange, st *eventState) {
	st.postmap["type"] = "IdentityChange"
	st.dowebhook = 1
	log.Info().Str("jid", evt.JID.String()).Msg("Identity changed")
}
