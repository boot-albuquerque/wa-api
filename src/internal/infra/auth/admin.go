package auth

import (
	"crypto/sha256"
	"crypto/subtle"
)

// AdminValidator encapsula a validação de token admin via constant-time compare.
type AdminValidator struct {
	adminHash [sha256.Size]byte
}

// NewAdminValidator cria um validador a partir do token admin (plaintext).
func NewAdminValidator(adminToken string) *AdminValidator {
	return &AdminValidator{
		adminHash: sha256.Sum256([]byte(adminToken)),
	}
}

// Validate compara candidateToken com o admin token usando constant-time compare
// para evitar timing-attack leak dos bytes do admin token.
// Retorna true se o token for válido.
func (v *AdminValidator) Validate(candidateToken string) bool {
	candidateHash := sha256.Sum256([]byte(candidateToken))
	return subtle.ConstantTimeCompare(candidateHash[:], v.adminHash[:]) == 1
}
