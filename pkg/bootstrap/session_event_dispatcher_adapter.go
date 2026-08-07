package bootstrap

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

// sessionEventDispatcherAdapter implementa appport.SessionEventDispatcher
// encapsulando sendEventWithWebHook (lifecycle_webhook.go) sem movê-la: os
// efeitos (webhook por usuário, webhook global, broadcast WS, modo Stdio,
// HMAC) continuam exatamente onde estão hoje, com as dependências privadas
// de pkg/bootstrap (appCtx.UserInfoCache, clientManager) que já têm.
type sessionEventDispatcherAdapter struct{}

var _ appport.SessionEventDispatcher = sessionEventDispatcherAdapter{}

// NewSessionEventDispatcher constrói o adapter de pkg/bootstrap que o
// SessionOrchestrator (Fase 2f) injeta como appport.SessionEventDispatcher.
func NewSessionEventDispatcher() appport.SessionEventDispatcher {
	return sessionEventDispatcherAdapter{}
}

// Dispatch resolve userID -> *UserEventHandler via clientManager.GetUserClient
// (o mesmo registro que lifecycle.go:232 preenche com SetUserClient) e chama
// sendEventWithWebHook com o payload fornecido. eventType é gravado em
// payload["type"] porque sendEventWithWebHook lê o tipo do próprio postmap,
// não de um parâmetro separado.
func (sessionEventDispatcherAdapter) Dispatch(_ context.Context, userID string, eventType string, payload map[string]any) error {
	handle := clientManager.GetUserClient(userID)
	if handle == nil {
		log.Warn().Str("userID", userID).Str("eventType", eventType).Msg("SessionEventDispatcher: no UserEventHandler registered for userID, dropping event")
		return nil
	}

	evh, ok := handle.(*UserEventHandler)
	if !ok {
		log.Error().Str("userID", userID).Str("eventType", eventType).Msg("SessionEventDispatcher: registered UserEventHandler is not *bootstrap.UserEventHandler")
		return nil
	}

	postmap := make(map[string]interface{}, len(payload)+1)
	for k, v := range payload {
		postmap[k] = v
	}
	postmap["type"] = eventType

	sendEventWithWebHook(evh, postmap, "")
	return nil
}
