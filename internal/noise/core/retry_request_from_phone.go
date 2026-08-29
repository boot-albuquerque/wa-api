package core

import (
	"context"
	"time"

	"wa-api/internal/noise/capabilities/retry"
	"wa-api/internal/noise/protocol/types"
)

// RequestFromPhoneDelay specifies how long to wait for the sender to resend the message before requesting from your phone.
// This is only used if Client.AutomaticMessageRerequestFromPhone is true.
//
// Continua sendo uma VARIAVEL da raiz, e nao foi movida para o subpacote, por
// dois motivos: e' API publica ajustavel em tempo de execucao, e variaveis nao
// podem ser reexportadas por apelido em Go. O subpacote le' o valor atual a
// cada chamada, pelo metodo RerequestDelay do Transport.
var RequestFromPhoneDelay = 5 * time.Second

// Fachadas do reenvio pelo telefone. A logica e o estado vivem em
// internal/wa-noise/retry (Fase F/G, lote 5).

// retryMessageRef traduz o types.MessageInfo da raiz para a fatia dele que o
// subpacote usa.
func retryMessageRef(info *types.MessageInfo) retry.MessageRef {
	return retry.MessageRef{
		Chat:     info.Chat,
		Sender:   info.Sender,
		ID:       info.ID,
		Type:     info.Type,
		IsFromMe: info.IsFromMe,
	}
}

func (cli *Client) cancelDelayedRequestFromPhone(msgID types.MessageID) {
	if cli == nil {
		return
	}
	retry.CancelDelayedFromPhone(cli.retryT(), msgID)
}

func (cli *Client) delayedRequestMessageFromPhone(info *types.MessageInfo) {
	if cli == nil {
		return
	}
	retry.DelayedRequestFromPhone(cli.retryT(), retryMessageRef(info))
}

func (cli *Client) immediateRequestMessageFromPhone(ctx context.Context, info *types.MessageInfo) {
	if cli == nil {
		return
	}
	retry.ImmediateRequestFromPhone(ctx, cli.retryT(), retryMessageRef(info))
}

func (cli *Client) clearDelayedMessageRequests() {
	if cli == nil {
		return
	}
	retry.ClearDelayedRequests(cli.retryT())
}
