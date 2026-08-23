package bootstrap

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/db"
)

// Os três eventos de etiqueta que a biblioteca emite e que, até 2026-08-21,
// chegavam e eram deitados fora (F191).
//
// O que se recolhe aqui é LEITURA do que outro dispositivo fez: a biblioteca
// não sabe CRIAR etiquetas (LIB-01), portanto não há escrita a espelhar. Quem
// pusesse uma etiqueta no telemóvel não a via na API — e essa é exatamente a
// pergunta que o achado previa que alguém faria um dia.
//
// FromFullSync NÃO é filtrado, de propósito: a sincronização completa é
// justamente como se recupera o estado que existia antes de a sessão parear.
// Ignorá-la deixaria a tabela vazia até alguém mexer numa etiqueta.

// handleLabelEdit persiste a etiqueta criada, renomeada ou apagada.
func (evh *UserEventHandler) handleLabelEdit(evt *events.LabelEdit, st *eventState) {
	st.postmap["type"] = "LabelEdit"
	st.postmap["label_id"] = evt.LabelID
	st.postmap["from_full_sync"] = evt.FromFullSync
	st.dowebhook = 1

	rotulo := db.Label{
		LabelID:   evt.LabelID,
		UpdatedAt: evt.Timestamp,
	}
	if a := evt.Action; a != nil {
		rotulo.Name = a.GetName()
		rotulo.Color = int(a.GetColor())
		rotulo.PredefinedID = int(a.GetPredefinedID())
		rotulo.Deleted = a.GetDeleted()
		st.postmap["name"] = rotulo.Name
		st.postmap["color"] = rotulo.Color
		st.postmap["deleted"] = rotulo.Deleted
	}

	if evh.DB == nil {
		return
	}
	if err := db.NewLabelRepository(evh.DB).UpsertLabel(context.Background(), evh.UserID, rotulo); err != nil {
		log.Error().Err(err).Str("userid", evh.UserID).Str("label_id", evt.LabelID).
			Msg("failed to store label")
		return
	}
	log.Info().Str("userid", evh.UserID).Str("label_id", evt.LabelID).
		Str("name", rotulo.Name).Bool("deleted", rotulo.Deleted).Msg("Label updated")
}

// handleLabelAssociationChat persiste a marcação de uma CONVERSA.
func (evh *UserEventHandler) handleLabelAssociationChat(evt *events.LabelAssociationChat, st *eventState) {
	marcada := evt.Action.GetLabeled()

	st.postmap["type"] = "LabelAssociationChat"
	st.postmap["label_id"] = evt.LabelID
	st.postmap["chat_jid"] = evt.JID.String()
	st.postmap["labeled"] = marcada
	st.dowebhook = 1

	if evh.DB == nil {
		return
	}
	assoc := db.LabelChat{
		LabelID:   evt.LabelID,
		ChatJID:   evt.JID.String(),
		Labeled:   marcada,
		UpdatedAt: evt.Timestamp,
	}
	if err := db.NewLabelRepository(evh.DB).SetChatLabel(context.Background(), evh.UserID, assoc); err != nil {
		log.Error().Err(err).Str("userid", evh.UserID).Str("label_id", evt.LabelID).
			Str("chat", assoc.ChatJID).Msg("failed to store chat label")
		return
	}
	log.Info().Str("userid", evh.UserID).Str("label_id", evt.LabelID).
		Str("chat", assoc.ChatJID).Bool("labeled", marcada).Msg("Chat label changed")
}

// handleLabelAssociationMessage persiste a marcação de uma MENSAGEM.
func (evh *UserEventHandler) handleLabelAssociationMessage(evt *events.LabelAssociationMessage, st *eventState) {
	marcada := evt.Action.GetLabeled()

	st.postmap["type"] = "LabelAssociationMessage"
	st.postmap["label_id"] = evt.LabelID
	st.postmap["chat_jid"] = evt.JID.String()
	st.postmap["message_id"] = evt.MessageID
	st.postmap["labeled"] = marcada
	st.dowebhook = 1

	if evh.DB == nil {
		return
	}
	if err := db.NewLabelRepository(evh.DB).SetMessageLabel(context.Background(), evh.UserID,
		evt.LabelID, evt.JID.String(), evt.MessageID, marcada, evt.Timestamp); err != nil {
		log.Error().Err(err).Str("userid", evh.UserID).Str("label_id", evt.LabelID).
			Str("message_id", evt.MessageID).Msg("failed to store message label")
		return
	}
	log.Info().Str("userid", evh.UserID).Str("label_id", evt.LabelID).
		Str("message_id", evt.MessageID).Bool("labeled", marcada).Msg("Message label changed")
}

// handleAppStateChange encaminha os cinco eventos que anunciam mudanças feitas
// noutro dispositivo.
//
// Existe para tirar cinco ramos do switch principal, que é a maior função do
// repositório — ver o comentário no `case` que delega para aqui. O tipo é
// reaberto porque o switch de cima já o resolveu para o grupo, não para o
// membro.
func (evh *UserEventHandler) handleAppStateChange(rawEvt any, st *eventState) {
	switch evt := rawEvt.(type) {
	case *events.PushName:
		evh.handlePushName(evt, st)
	case *events.BusinessName:
		evh.handleBusinessName(evt, st)
	case *events.LabelEdit:
		evh.handleLabelEdit(evt, st)
	case *events.LabelAssociationChat:
		evh.handleLabelAssociationChat(evt, st)
	case *events.LabelAssociationMessage:
		evh.handleLabelAssociationMessage(evt, st)
	default:
		// Inalcançável: o switch de cima já filtrou os cinco tipos. Fica como
		// registo e não como panic — um tipo novo acrescentado lá e esquecido
		// aqui tem de aparecer no log, não derrubar o processo.
		log.Warn().Str("event_type", fmt.Sprintf("%T", rawEvt)).
			Msg("app-state change routed here without a branch")
	}
}
