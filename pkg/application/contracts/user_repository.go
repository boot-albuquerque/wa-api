package port

import (
	"context"

	"wa-api/pkg/domain"
)

// UserRepository é a porta de persistência dos use cases de user/.
//
// Ela é trabalho NOVO desta fase, e não a exceção nomeada do MigrationRunner:
// os use cases de user/ nunca consumiram DBAccess — recebiam um *sqlx.DB
// direto no construtor e escreviam SQL no corpo do Execute. A ADR-001 não
// mencionava essa porta, o que subestimava o custo da fase em uma porta
// inteira (ver a lacuna apontada na Fase 6).
//
// A montagem de SQL — inclusive o UPDATE dinâmico de edit_user — é do
// adapter. O use case decide QUAIS campos mudar; como isso vira uma
// instrução, não é decisão dele.
type UserRepository interface {
	// CreateUser insere o usuário. Devolve created=false quando o token já
	// existe, decidido pelo índice UNIQUE de token_hash e não por uma
	// checagem prévia fora de transação.
	CreateUser(ctx context.Context, rec domain.UserRecord) (created bool, err error)

	// UserExists reporta se há usuário com o id informado.
	UserExists(ctx context.Context, id string) (bool, error)

	// UpdateUser aplica os campos presentes em upd. Devolve
	// domain.ErrDuplicateToken quando a mudança de token colide com outro
	// usuário.
	UpdateUser(ctx context.Context, id string, upd domain.UserUpdate) error

	// ListUsers devolve todos os usuários, ou apenas o de id informado
	// quando id não é vazio.
	ListUsers(ctx context.Context, id string) ([]domain.UserListEntry, error)

	// DeleteUser remove o usuário. Devolve deleted=false se não existia.
	DeleteUser(ctx context.Context, id string) (deleted bool, err error)
}

// SessionStatusReader informa o estado ao vivo da sessão WhatsApp de um
// usuário.
//
// Substitui a interface ClientManagerAdapter que list_users.go declarava
// localmente com GetWhatsmeowClient(id) interface{} — um interface{} escolhido,
// segundo o próprio comentário do arquivo, "to avoid circular deps". O valor
// devolvido nunca era usado para nada além de comparar com nil.
type SessionStatusReader interface {
	// SessionStatus devolve se a sessão está conectada e autenticada.
	SessionStatus(ctx context.Context, userID string) (connected, loggedIn bool)
}
