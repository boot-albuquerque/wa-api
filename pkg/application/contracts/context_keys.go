// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em pkg/infra/.
package port

// CtxKey é o tipo para chaves de contexto tipadas, evitando magic strings.
type CtxKey string

// UserInfoKey é a chave de contexto injetada pelo middleware authalice do upstream.
// O valor associado implementa a interface com método Get(key string) string.
const UserInfoKey CtxKey = "userinfo"
