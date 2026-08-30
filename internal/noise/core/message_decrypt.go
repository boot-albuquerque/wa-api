package core

import (
	"context"

	"wa-api/internal/noise/capabilities/message"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// O caminho de decifragem vive em internal/noise/message/ desde a Fase F/G
// lote 9. As fachadas abaixo existem porque client.go chama
// handleEncryptedMessage direto, porque internals.go (gerado) cita os nomes
// minusculos, e porque retry_transport.go e send_adapter.go chamam
// migrateSessionStore/clearUntrustedIdentity.

// pbSerializer continua declarado aqui porque retry_transport.go o le. E' o
// mesmo valor que message/ usa direto (store.SignalProtobufSerializer): o
// pacote extraido nao passa por esta variavel.
var pbSerializer = store.SignalProtobufSerializer

// EventAlreadyProcessed e' o MESMO valor de message.ErrEventAlreadyProcessed,
// por atribuicao e nao por `errors.New` proprio: um sentinela reconstruido aqui
// quebraria `errors.Is` para quem comparasse com o nome historico.
var EventAlreadyProcessed = message.ErrEventAlreadyProcessed

func (cli *Client) handleEncryptedMessage(ctx context.Context, node *waBinary.Node) {
	message.HandleEncrypted(ctx, cli.msgT(), node)
}

func (cli *Client) handlePlaintextMessage(ctx context.Context, info *types.MessageInfo, node *waBinary.Node) (handlerFailed bool) {
	return message.HandlePlaintext(ctx, cli.msgT(), info, node)
}

func (cli *Client) migrateSessionStore(ctx context.Context, pn, lid types.JID) {
	message.MigrateSessionStore(ctx, cli.msgT(), pn, lid)
}

func (cli *Client) decryptMessages(ctx context.Context, info *types.MessageInfo, node *waBinary.Node) {
	message.DecryptMessages(ctx, cli.msgT(), info, node)
}

func (cli *Client) clearUntrustedIdentity(ctx context.Context, target types.JID) error {
	return message.ClearUntrustedIdentity(ctx, cli.msgT(), target)
}
