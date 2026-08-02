package domain

import "errors"

// ErrDuplicateToken é devolvido quando o token informado já pertence a outro
// usuário. Mora no domínio porque tanto o use case quanto o adapter de
// persistência precisam falar dele.
var ErrDuplicateToken = errors.New("user with this token already exists")

// ErrNoFieldsToUpdate é devolvido quando um UserUpdate não traz campo algum.
var ErrNoFieldsToUpdate = errors.New("no fields to update")

// UserRecord é a linha de usuário a ser criada. Os campos correspondem ao que
// AddUserUseCase já montava; o SQL que os grava é do adapter.
type UserRecord struct {
	ID              string
	Name            string
	Token           string
	Webhook         string
	Expiration      int
	Events          string
	ProxyURL        string
	WebhookUseProxy bool
	S3              S3Config
	HmacKey         []byte
	History         int
}

// UserUpdate descreve uma alteração parcial de usuário. Campo nil é campo
// que não muda — a distinção entre "não informado" e "informado como vazio"
// é o que o UPDATE dinâmico precisa saber, e é decisão do use case.
type UserUpdate struct {
	Name            *string
	Token           *string
	Webhook         *string
	Expiration      *int
	Events          *string
	History         *int
	ProxyURL        *string
	WebhookUseProxy *bool
	S3              *S3Config
}

// UserListEntry é uma linha da listagem de usuários, já lida do banco.
//
// Os campos anuláveis viram valor mais um booleano de validade em vez de
// sql.NullString/sql.NullInt64: database/sql é detalhe de persistência, e o
// domínio não deve conhecê-lo.
type UserListEntry struct {
	ID              string
	Name            string
	Webhook         string
	JID             string
	QRCode          string
	Expiration      int64
	ProxyURL        string
	HasProxyURL     bool
	WebhookUseProxy bool
	Events          string
	S3              S3Config
}
