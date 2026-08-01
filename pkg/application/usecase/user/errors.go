package user

import (
	"errors"
	"strings"
)

// ErrDuplicateToken indica que o token pedido já pertence a outro usuário.
// Substitui o erro cru do driver, que antes vazava nome de constraint e
// dialeto de banco para o cliente HTTP.
var ErrDuplicateToken = errors.New("user with this token already exists")

// isUniqueViolation reconhece violação de unicidade nos dois backends
// suportados sem importar driver nenhum: a camada de aplicação não pode
// depender de lib/pq nem de modernc.org/sqlite (é exatamente o que o depguard
// da F6 vai barrar). O casamento é por texto porque database/sql não expõe
// SQLSTATE de forma portável.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
