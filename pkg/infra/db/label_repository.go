package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// As tabelas, como constantes: os nomes aparecem no log de cada falha, e um
// literal divergente faria a busca do operador não encontrar nada (ADR-0004).
const (
	tabelaLabels        = "wa_labels"
	tabelaLabelChats    = "wa_label_chats"
	tabelaLabelMessages = "wa_label_messages"
)

// Label é uma etiqueta do WhatsApp, como o utilizador a vê no telemóvel.
type Label struct {
	LabelID      string    `db:"label_id" json:"label_id"`
	Name         string    `db:"name" json:"name"`
	Color        int       `db:"color" json:"color"`
	PredefinedID int       `db:"predefined_id" json:"predefined_id"`
	Deleted      bool      `db:"deleted" json:"deleted"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// LabelChat é a associação entre uma etiqueta e uma conversa.
type LabelChat struct {
	LabelID   string    `db:"label_id" json:"label_id"`
	ChatJID   string    `db:"chat_jid" json:"chat_jid"`
	Labeled   bool      `db:"labeled" json:"labeled"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// LabelRepository persiste o estado de etiquetas que os eventos da biblioteca
// anunciam (F191).
//
// NÃO cria etiquetas: a biblioteca não sabe fazê-lo (LIB-01), e esta camada
// só recolhe o que outro dispositivo já fez. É leitura do que aconteceu, não
// escrita do que queremos que aconteça.
type LabelRepository struct{ db *sqlx.DB }

// NewLabelRepository cria o repositório.
func NewLabelRepository(database *sqlx.DB) *LabelRepository {
	return &LabelRepository{db: database}
}

// UpsertLabel grava a etiqueta, sobrepondo o que lá estava.
//
// É UPSERT e não INSERT porque o evento chega a cada edição E em cada
// sincronização completa: o mesmo label_id volta muitas vezes, e o último a
// chegar é o que vale.
func (r *LabelRepository) UpsertLabel(ctx context.Context, userID string, l Label) error {
	q := r.db.Rebind(`
		INSERT INTO wa_labels (user_id, label_id, name, color, predefined_id, deleted, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, label_id) DO UPDATE SET
			name=excluded.name, color=excluded.color,
			predefined_id=excluded.predefined_id, deleted=excluded.deleted,
			updated_at=excluded.updated_at`)
	if _, err := r.db.ExecContext(ctx, q, userID, l.LabelID, l.Name, l.Color,
		l.PredefinedID, l.Deleted, l.UpdatedAt); err != nil {
		log.Error().Err(err).Str("table", tabelaLabels).
			Str("user_id", userID).Str("label_id", l.LabelID).Msg("failed to upsert label")
		return err
	}
	return nil
}

// SetChatLabel grava a associação etiqueta↔conversa.
//
// `labeled=false` é GRAVADO, não apagado: o evento de desetiquetar traz essa
// informação, e apagar a linha perderia o instante em que aconteceu — que é o
// que distingue "nunca teve" de "tinha e tiraram".
func (r *LabelRepository) SetChatLabel(ctx context.Context, userID string, c LabelChat) error {
	q := r.db.Rebind(`
		INSERT INTO wa_label_chats (user_id, label_id, chat_jid, labeled, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, label_id, chat_jid) DO UPDATE SET
			labeled=excluded.labeled, updated_at=excluded.updated_at`)
	if _, err := r.db.ExecContext(ctx, q, userID, c.LabelID, c.ChatJID, c.Labeled, c.UpdatedAt); err != nil {
		log.Error().Err(err).Str("table", tabelaLabelChats).
			Str("user_id", userID).Str("label_id", c.LabelID).Str("chat_jid", c.ChatJID).
			Msg("failed to store chat label")
		return err
	}
	return nil
}

// SetMessageLabel grava a associação etiqueta↔mensagem.
func (r *LabelRepository) SetMessageLabel(ctx context.Context, userID, labelID, chatJID, messageID string, labeled bool, at time.Time) error {
	q := r.db.Rebind(`
		INSERT INTO wa_label_messages (user_id, label_id, chat_jid, message_id, labeled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, label_id, chat_jid, message_id) DO UPDATE SET
			labeled=excluded.labeled, updated_at=excluded.updated_at`)
	if _, err := r.db.ExecContext(ctx, q, userID, labelID, chatJID, messageID, labeled, at); err != nil {
		log.Error().Err(err).Str("table", tabelaLabelMessages).
			Str("user_id", userID).Str("label_id", labelID).Str("message_id", messageID).
			Msg("failed to store message label")
		return err
	}
	return nil
}

// ListLabels devolve as etiquetas NÃO apagadas do utilizador.
//
// As apagadas ficam na tabela — o registo de que existiram tem valor para quem
// reconstrói histórico — mas não saem na listagem, que responde "o que existe
// hoje".
func (r *LabelRepository) ListLabels(ctx context.Context, userID string) ([]Label, error) {
	var out []Label
	q := r.db.Rebind(`SELECT label_id, name, color, predefined_id, deleted, updated_at
		FROM wa_labels WHERE user_id=? AND deleted=? ORDER BY label_id`)
	if err := r.db.SelectContext(ctx, &out, q, userID, false); err != nil {
		log.Error().Err(err).Str("table", tabelaLabels).Str("user_id", userID).
			Msg("failed to list labels")
		return nil, err
	}
	return out, nil
}

// ListChatsForLabel devolve as conversas ATUALMENTE marcadas com a etiqueta.
func (r *LabelRepository) ListChatsForLabel(ctx context.Context, userID, labelID string) ([]LabelChat, error) {
	var out []LabelChat
	q := r.db.Rebind(`SELECT label_id, chat_jid, labeled, updated_at
		FROM wa_label_chats WHERE user_id=? AND label_id=? AND labeled=? ORDER BY chat_jid`)
	if err := r.db.SelectContext(ctx, &out, q, userID, labelID, true); err != nil {
		log.Error().Err(err).Str("table", tabelaLabelChats).
			Str("user_id", userID).Str("label_id", labelID).Msg("failed to list label chats")
		return nil, err
	}
	return out, nil
}
