package middleware

import (
	"net/http"
)

// IdempotencyKey extrai e valida o header X-Idempotency-Key.
// Requests com a mesma chave dentro da janela de idempotência retornam
// a resposta original em vez de re-executar a operação.
//
// TODO: Implementar armazenamento de chaves de idempotência
// quando o roadmap demandar. O backend de storage (memória/Redis/Postgres)
// será injetado via constructor.
func IdempotencyKey( /* store IdempotencyStore */ ) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Extrair header X-Idempotency-Key
			// TODO: Verificar se chave existe no store
			// TODO: Se existe: retornar resposta cacheada
			// TODO: Se não existe: executar handler, cachear resposta
			next.ServeHTTP(w, r)
		})
	}
}
