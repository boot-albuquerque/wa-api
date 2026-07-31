package middleware

import (
	"net/http"
)

// HMACVerifier retorna um middleware que verifica a assinatura HMAC
// no header X-Signature de cada request.
//
// TODO: Implementar verificação HMAC quando o roadmap demandar.
// O formato da assinatura: HMAC-SHA256(requestBody, chave).
// O middleware injeta um context value "hmac_verified" com o resultado.
func HMACVerifier(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Verificar header X-Signature contra HMAC-SHA256(body, key)
			// ctx := context.WithValue(r.Context(), ctxKey("hmac_verified"), true)
			// next.ServeHTTP(w, r.WithContext(ctx))
			next.ServeHTTP(w, r)
		})
	}
}
