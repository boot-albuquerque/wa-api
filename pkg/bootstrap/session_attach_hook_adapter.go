package bootstrap

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

// sessionAttachHookAdapter implementa appport.SessionAttachHook. Fica em
// pkg/bootstrap (não em pkg/infra/wa-noise) porque monta *UserEventHandler e
// registra handleEvent — o handler de domínio completo, que depende de
// estado privado de bootstrap (DB, NotifyFn, mode) e não pode ser movido
// para infra sem inverter a direção de dependência bootstrap -> infra.
type sessionAttachHookAdapter struct {
	s *server
}

var _ appport.SessionAttachHook = (*sessionAttachHookAdapter)(nil)

// NewSessionAttachHook constrói o adapter que o SessionOrchestrator
// (Fase 2f) injeta como appport.SessionAttachHook.
func NewSessionAttachHook(s *server) appport.SessionAttachHook {
	return &sessionAttachHookAdapter{s: s}
}

// Attach replica exatamente a construção de lifecycle.go:218-232: resolve o
// *wa-noise.Client já registrado por SessionProvider/SessionRegistry via
// clientManager.Getwa-noiseClient, monta o UserEventHandler, registra
// handleEvent e guarda o handle em clientManager.SetUserClient.
//
// Também é dona do kill-channel (lifecycle.go:459-472): a goroutine que
// bloqueia em <-kill, faz o cleanup dos registros em clientManager e grava
// users.connected=0 migra para cá, acionada a partir de Attach. Isso mantém
// um único escritor dessa coluna no caminho de desconexão — o orchestrator
// só observa SessionEvent, nunca escreve nela.
func (h *sessionAttachHookAdapter) Attach(ctx context.Context, userID, token string) error {
	client := clientManager.GetWaNoiseClient(userID)
	if client == nil {
		return fmt.Errorf("sessionAttachHook: no wanoise client registered for userID %s", userID)
	}

	evh := &UserEventHandler{
		WAClient:       client,
		EventHandlerID: 1,
		UserID:         userID,
		Token:          token,
		DB:             h.s.DB,
		NotifyFn:       h.s.SendNotification,
		mode:           h.s.Mode,
	}
	evh.EventHandlerID = evh.WAClient.AddEventHandler(evh.handleEvent)
	clientManager.SetUserClient(userID, evh)

	kill := make(chan bool, 1)
	appCtx.KillChannel.Set(userID, kill)

	go func() {
		// eventhandler.go:67 continua sinalizando appCtx.KillChannel no
		// caminho LoggedOut, exatamente como hoje. Esta goroutine é quem
		// consome o sinal, de onde quer que ele venha (LoggedOut ou Detach).
		<-kill
		log.Info().Str("userid", userID).Msg("Received kill signal")
		client.Disconnect()
		clientManager.DeleteWaNoiseClient(userID)
		clientManager.DeleteUserClient(userID)
		clientManager.DeleteHTTPClient(userID)
		if _, err := h.s.DB.Exec(`UPDATE users SET qrcode='', connected=0 WHERE id=$1`, userID); err != nil {
			log.Error().Err(err).Msg("failed to mark user disconnected on kill")
		}
		appCtx.KillChannel.Delete(userID, kill)
	}()

	return nil
}

// Detach sinaliza o kill-channel de userID, se um estiver registrado.
// Signal é não-bloqueante e vira no-op se não houver goroutine ouvindo
// (userID já desanexado ou nunca anexado), o que torna Detach idempotente.
func (h *sessionAttachHookAdapter) Detach(userID string) {
	appCtx.KillChannel.Signal(userID)
}
