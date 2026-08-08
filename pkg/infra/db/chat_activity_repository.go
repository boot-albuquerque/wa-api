package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// ChatActivityRepository implementa appport.ChatActivityReader sobre
// *sqlx.DB. Fino de propósito: delega inteiramente para
// GetLastActivityByUser (message_history.go), que já concentra o SQL —
// existe como tipo próprio só para satisfazer a porta na injeção de
// dependência (mesmo padrão de UserRepository).
type ChatActivityRepository struct {
	db *sqlx.DB
}

// NewChatActivityRepository cria o repositório.
func NewChatActivityRepository(db *sqlx.DB) *ChatActivityRepository {
	return &ChatActivityRepository{db: db}
}

// GetLastActivityByUser implementa appport.ChatActivityReader.
func (r *ChatActivityRepository) GetLastActivityByUser(_ context.Context, userID string) (map[string]time.Time, error) {
	return GetLastActivityByUser(r.db, userID)
}

// GetChatPushNames devolve, por chat, o pushName mais recente que já chegou
// naquela conversa. Ver GetChatPushNamesByUser.
func (r *ChatActivityRepository) GetChatPushNames(_ context.Context, userID string) (map[string]string, error) {
	return GetChatPushNamesByUser(r.db, userID)
}
