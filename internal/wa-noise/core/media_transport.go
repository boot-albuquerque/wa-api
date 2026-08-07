package whatsmeow

import (
	"context"
	"net/http"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/media"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// mediaTransport adapta *Client a media.Transport. Existe para que o pacote
// internal/wa-noise/media possa operar sobre uma interface estreita sem
// importar o pacote raiz (o que fecharia um ciclo) e sem que *Client precise
// ganhar metodos exportados novos so' para satisfazer a interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 1".
type mediaTransport struct {
	cli *Client
}

var _ media.Transport = mediaTransport{}

// mediaT devolve o adaptador de midia deste cliente.
func (cli *Client) mediaT() media.Transport {
	return mediaTransport{cli}
}

func (t mediaTransport) HTTPClient() *http.Client {
	return t.cli.mediaHTTP
}

func (t mediaTransport) Log() waLog.Logger {
	return t.cli.Log
}

func (t mediaTransport) IsMessenger() bool {
	return t.cli.MessengerConfig != nil
}

func (t mediaTransport) MessengerUserAgent() string {
	if t.cli.MessengerConfig == nil {
		return ""
	}
	return t.cli.MessengerConfig.UserAgent
}

func (t mediaTransport) ReturnDownloadWarnings() bool {
	return ReturnDownloadWarnings
}

func (t mediaTransport) MediaConnCache() *media.ConnCache {
	return &t.cli.mediaConn
}

func (t mediaTransport) SendMediaConnIQ(ctx context.Context) (*waBinary.Node, error) {
	return t.cli.sendIQ(ctx, infoQuery{
		Namespace: "w:m",
		Type:      "set",
		To:        types.ServerJID,
		Content:   []waBinary.Node{{Tag: "media_conn"}},
	})
}
