package bootstrap

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"

	appsession "wa-api/pkg/application/session"
	"wa-api/pkg/infra/storage"
	wa "wa-api/pkg/infra/whatsmeow"
)

// newSessionOrchestrator liga os quatro ports de sessão (Fases 2a-2e) ao
// Orchestrator concreto (Fase 2f): SessionProviderAdapter sobre o
// sqlstore.Container compartilhado, ClientManager como SessionRegistry e os
// dois adapters de pkg/bootstrap para dispatcher e attach hook. É o que
// substitui (*server).startClient, removido nesta fase.
func newSessionOrchestrator(s *server) *appsession.Orchestrator {
	var clientLog waLog.Logger
	if *waDebug != "" {
		clientLog = waLog.Stdout("Client", *waDebug, *colorOutput)
	}

	// DeviceProps é global do SDK e precisa estar definido antes de qualquer
	// cliente ser criado — antes vivia no topo de startClient.
	store.DeviceProps.PlatformType = wa.GetPlatformTypeEnum(*platformType)
	store.DeviceProps.Os = osName

	provider := wa.NewSessionProviderWithLogger(container, deviceJIDLookup(s), clientLog)

	return appsession.NewOrchestrator(
		provider,
		clientManager,
		NewSessionEventDispatcher(),
		NewSessionAttachHook(s),
		s.DB,
		appsession.WithDefaultWebhookUseProxy(appCtx.GlobalWebhookUseProxy),
		appsession.WithS3Provisioner(func(userID string) {
			storage.GetS3Manager().EnsureClientFromDB(userID)
		}),
	)
}

// deviceJIDLookup resolve users.jid, que é como o provider decide entre
// reaproveitar o device persistido e criar um novo. Substitui o parâmetro
// textjid que connectOnStartup e o ConnectHandler passavam para startClient.
func deviceJIDLookup(s *server) wa.DeviceJIDLookup {
	return func(ctx context.Context, userID string) (string, error) {
		var jid string
		if err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(jid, '') FROM users WHERE id=$1", userID).Scan(&jid); err != nil {
			return "", err
		}
		return jid, nil
	}
}

// startSession sobe a sessão de userID. É a função injetada no ConnectHandler
// e em Deps; bloqueia enquanto o pareamento estiver ativo, então os chamadores
// a disparam em goroutine — a mesma forma que startClient tinha.
//
// O contexto é Background de propósito: a vida da sessão não é a vida da
// requisição HTTP que a iniciou. Amarrá-la ao contexto do request mataria o
// fluxo de QR assim que /session/connect respondesse.
func (s *server) startSession(userID, token string) {
	if err := s.SessionOrchestrator.Start(context.Background(), userID, token); err != nil {
		log.Error().Err(err).Str("userid", userID).Msg("failed to start session")
	}
}
