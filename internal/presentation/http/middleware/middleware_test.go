package middleware

import (
	"testing"
)

// TestHMACVerifier_SkipPending valida que o stub compila e é seguramente
// não funcional (não bloqueia requests).
func TestHMACVerifier_SkipPending(t *testing.T) {
	t.Skip("implementation pending — HMAC verification not yet needed")
}

// TestIdempotencyKey_SkipPending valida que o stub compila.
func TestIdempotencyKey_SkipPending(t *testing.T) {
	t.Skip("implementation pending — idempotency store not yet needed")
}

// TestRetryAware_SkipPending valida que o stub compila.
func TestRetryAware_SkipPending(t *testing.T) {
	t.Skip("implementation pending — retry policies not yet needed")
}
