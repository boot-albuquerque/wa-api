package port

import "context"

// SessionEventDispatcher publica um evento de status de sessão pelos canais
// de saída do wa-api (webhook HTTP, broadcast WS de /session/ws, modo Stdio).
//
// Existe para o SessionOrchestrator poder disparar esses efeitos sem
// importar símbolos não exportados de pkg/bootstrap (appCtx.UserInfoCache,
// sendEventWithWebHook) — a mesma inversão de dependência que SessionRegistry
// evita do lado de infra. O adapter que o implementa vive em pkg/bootstrap e
// encapsula sendEventWithWebHook sem movê-la:
// pkg/bootstrap/lifecycle_webhook.go continua onde está, com as dependências
// que já tem.
type SessionEventDispatcher interface {
	// Dispatch entrega o evento eventType de userID com payload. É
	// best-effort quanto à entrega em si (o adapter mantém a semântica
	// fire-and-forget atual); o erro devolvido cobre falha em preparar o
	// despacho, não falha de um destino individual.
	Dispatch(ctx context.Context, userID string, eventType string, payload map[string]any) error
}
