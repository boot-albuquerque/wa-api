package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashToken deriva a representação armazenável de um token de usuário.
//
// Vive em domain porque três camadas independentes precisam produzir
// exatamente o mesmo valor — a migração (pkg/infra/db), a autenticação
// (pkg/presentation/http/middleware) e os use cases de usuário — e domain é a
// única que todas podem importar sem criar ciclo.
//
// SHA-256 sem salt é deliberado: o valor precisa ser determinístico para servir
// de chave de lookup e de coluna UNIQUE. Tokens são segredos de alta entropia
// gerados pelo operador, não senhas escolhidas por humanos, então o custo
// linear de um KDF não compraria resistência a dicionário aqui.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
