// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em internal/infrastructure/.
package port

import (
	"context"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
)

// DBAccess abstrai o acesso ao banco de dados.
// Implementação concreta em internal/infrastructure/db.
type DBAccess interface {
	// User operations
	CreateUser(ctx context.Context, user domain.User) error
	GetUserByToken(ctx context.Context, token string) (*domain.User, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	UpdateUser(ctx context.Context, user domain.User) error
	DeleteUser(ctx context.Context, id string) error

	// Session operations
	GetSessionByUserID(ctx context.Context, userID string) (*domain.Session, error)
	UpdateSession(ctx context.Context, session domain.Session) error

	// History
	SaveHistory(ctx context.Context, msg domain.HistoryMessage) error

	// Raw DB access for migrations
	DB() *sqlx.DB
}
