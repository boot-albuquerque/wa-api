// Package middleware contém middlewares HTTP cross-cutting para o disparazaap-wa-api.
// Chains compostas via alice (github.com/justinas/alice) e aplicadas em registerCustomRoutes().
//
// Stubs atuais:
//   - hmac.go: verificação de assinatura HMAC
//   - idempotency.go: chave de idempotência
//   - retry.go: políticas de retry
//
// TODO: Implementar cada middleware quando o roadmap demandar.
package middleware
