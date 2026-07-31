// Package auth contém middlewares e utilitários de autenticação/autorização
// extraídos de handlers.go como parte da migração para Clean Architecture.
//
// Estrutura:
//   - admin.go: validação de token admin via constant-time compare
//   - cache.go: cache de tokens com TTL configurável
//   - authenticators.go: lógica de lookup de usuário no DB
//
// Uso típico (em package main):
//
//	adminValidator := auth.NewAdminValidator(*adminToken)
//	s.router.Use(func(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        if !adminValidator.Validate(r.Header.Get("Authorization")) {
//	            s.Respond(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
//	            return
//	        }
//	        next.ServeHTTP(w, r)
//	    })
//	})
package auth
