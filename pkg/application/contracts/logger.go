package port

import "context"

// Logger abstrai logging estruturado para os usecases.
// A implementação concreta usa zerolog (compatível com o upstream).
//
// O ctx é obrigatório (não é um par de métodos "…Ctx" adicional: este repo
// não mantém shims de compatibilidade). Ele carrega o logger de requisição
// injetado pela cadeia hlog em pkg/bootstrap/router.go, e é o que faz o
// req_id do registro de fronteira aparecer também nos logs de use case da
// mesma requisição. Sem ctx, correlação é impossível.
type Logger interface {
	Info(ctx context.Context, msg string, keyvals ...any)
	Warn(ctx context.Context, msg string, keyvals ...any)
	Error(ctx context.Context, msg string, keyvals ...any)
}
