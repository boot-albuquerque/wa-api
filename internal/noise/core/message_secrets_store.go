package core

import (
	"context"

	"wa-api/internal/noise/capabilities/message"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waHistorySync"
	"wa-api/internal/noise/protocol/types"
)

// A gravacao de segredos de mensagem e de mapeamentos LID/PN vive em
// internal/wa-noise/message/ desde a Fase F/G lote 9. As fachadas abaixo
// existem porque internals.go (gerado) cita os cinco nomes minusculos.

func (cli *Client) storeMessageSecret(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) {
	message.StoreSecret(ctx, cli.msgT(), info, msg)
}

func (cli *Client) storeHistoricalMessageSecrets(ctx context.Context, conversations []*waHistorySync.Conversation) {
	message.StoreHistoricalSecrets(ctx, cli.msgT(), conversations)
}

func (cli *Client) storeLIDSyncMessage(ctx context.Context, msg []byte) {
	message.StoreLIDSyncMessage(ctx, cli.msgT(), msg)
}

func (cli *Client) storeGlobalSettings(ctx context.Context, settings *waHistorySync.GlobalSettings) {
	message.StoreGlobalSettings(ctx, cli.msgT(), settings)
}

func (cli *Client) storeHistoricalPNLIDMappings(ctx context.Context, mappings []*waHistorySync.PhoneNumberToLIDMapping) {
	message.StoreHistoricalPNLIDMappings(ctx, cli.msgT(), mappings)
}
