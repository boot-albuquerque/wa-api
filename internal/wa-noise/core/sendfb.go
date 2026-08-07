package whatsmeow

import (
	"context"
	"errors"

	armadillo "wa-api/internal/wa-noise/protocol/proto"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/capabilities/send"
	"wa-api/internal/wa-noise/protocol/types"
)

// As cinco constantes continuam sendo API publica da raiz com os mesmos valores
// e o mesmo tipo (constante sem tipo). As definicoes moram em
// internal/wa-noise/send desde a Fase F/G lote 8 — e FBMessageApplicationVersion
// mora em internal/wa-noise/retry desde o lote 5, para que o caminho de retry e
// o de envio normal nao possam divergir.
const FBMessageVersion = send.FBMessageVersion
const FBMessageApplicationVersion = send.FBApplicationVersion
const IGMessageApplicationVersion = send.IGApplicationVersion
const FBConsumerMessageVersion = send.ConsumerVersion
const FBArmadilloMessageVersion = send.ArmadilloVersion

// SendFBMessage sends the given v3 message to the given JID.
func (cli *Client) SendFBMessage(
	ctx context.Context,
	to types.JID,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
	extra ...SendRequestExtra,
) (resp SendResponse, err error) {
	if cli == nil {
		err = ErrClientIsNil
		return
	}
	var req SendRequestExtra
	if len(extra) > 1 {
		err = errors.New("only one extra parameter may be provided to SendMessage")
		return
	} else if len(extra) == 1 {
		req = extra[0]
	}
	return send.FBMessage(ctx, cli.sendT(), to, message, metadata, req)
}
