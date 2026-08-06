package port

import "context"

// SessionAttachHook liga o handler de eventos de domínio a uma sessão recém
// criada. É simétrico a SessionEventDispatcher: existe para preservar a
// direção de dependência do outro lado.
//
// O handler de domínio (myEventHandler, método de *bootstrap.MyClient) trata
// mensagem, presença, grupo e histórico e carrega estado de bootstrap
// (DB, NotifyFn, mode). Registrá-lo a partir de pkg/infra/whatsmeow forçaria
// infra → bootstrap; por isso o adapter que implementa este port fica em
// pkg/bootstrap, onde bootstrap → infra já é direção legal.
//
// Ordem de uso obrigatória: o orchestrator chama Attach antes de
// Pair/Connect — o handler precisa estar registrado antes que qualquer
// evento possa chegar.
type SessionAttachHook interface {
	// Attach monta o MyClient de userID, registra o handler de eventos de
	// domínio na sessão e passa a acompanhar o kill-channel dela, que
	// permanece propriedade de pkg/bootstrap.
	Attach(ctx context.Context, userID, token string) error

	// Detach remove o handler e encerra o acompanhamento do kill-channel.
	// É o único escritor de users.connected no caminho de desconexão —
	// o orchestrator observa SessionEvent, mas nunca escreve essa coluna.
	// Idempotente.
	Detach(userID string)
}
