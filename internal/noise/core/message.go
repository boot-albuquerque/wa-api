package core

import (
	"context"

	"wa-api/internal/noise/capabilities/message"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// A logica de recepcao/decifragem de mensagem vive em
// internal/wa-noise/message/ desde a Fase F/G lote 9. Os metodos abaixo sao
// fachadas sem logica: existem porque internals.go (GERADO por
// internals_generate.go, fora do escopo do lote) cita os nomes minusculos
// historicos em DangerousInternalClient, e porque client.go chama
// handleEncryptedMessage direto.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 9".

func (cli *Client) handleProtocolMessage(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) bool {
	return message.HandleProtocolMessage(ctx, cli.msgT(), info, msg)
}

func (cli *Client) processProtocolParts(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) bool {
	return message.ProcessProtocolParts(ctx, cli.msgT(), info, msg)
}

func (cli *Client) handleDecryptedMessage(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message, retryCount int) (handlerFailed bool) {
	return message.HandleDecrypted(ctx, cli.msgT(), info, msg, retryCount)
}

// SendProtocolMessageReceipt sends a receipt for a protocol message back to the phone.
//
// NAO ganhou a guarda `cli == nil -> ErrClientIsNil` que as fachadas exportadas
// dos lotes 1-7 receberam. E' deliberado: este lote se comprometeu com
// conservadorismo maximo no caminho de recepcao, e a guarda seria uma mudanca
// de comportamento (hoje um receptor nil entra em panico em sendNode) que a
// extracao nao exige. Registrado em PATCHES.md como divida consciente.
func (cli *Client) SendProtocolMessageReceipt(ctx context.Context, id types.MessageID, msgType types.ReceiptType) error {
	return message.SendProtocolReceipt(ctx, cli.msgT(), id, msgType)
}
