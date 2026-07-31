package middleware

import (
	"net/http"
)

// RetryAware é um middleware que adiciona headers informativos sobre
// políticas de retry sugeridas para o consumidor (disparazaap wa-worker).
//
// TODO: Implementar lógica de retry server-side quando necessário.
// Por enquanto apenas documenta o contrato para o consumidor.
// O consumidor (WuzApiHttpClient) já implementa retry client-side (1s + 3×3s).
func RetryAware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Adicionar header Retry-After em caso de 429/503
			// w.Header().Set("Retry-After", "3")
			next.ServeHTTP(w, r)
		})
	}
}
