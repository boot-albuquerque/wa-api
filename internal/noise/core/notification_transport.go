package core

import (
	"wa-api/internal/noise/capabilities/notification"
	sdklog "wa-api/internal/noise/observability/log"
)

// notifTransport adapta *Client a notification.Transport. Existe para que o
// pacote internal/wa-noise/notification possa operar sobre uma interface
// estreita sem importar o pacote raiz (o que fecharia um ciclo) e sem que
// *Client precise ganhar metodos exportados novos so' para satisfazer a
// interface.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 5".
type notifTransport struct {
	cli *Client
}

var _ notification.Transport = notifTransport{}

// notifT devolve o adaptador de notificacao deste cliente.
func (cli *Client) notifT() notification.Transport {
	return notifTransport{cli}
}

func (t notifTransport) Log() sdklog.Logger {
	return t.cli.Log
}

// DispatchEvent descarta o handlerFailed do dispatchEvent da raiz de proposito:
// nenhum dos pontos de despacho absorvidos pelo subpacote o consultava antes da
// extracao.
func (t notifTransport) DispatchEvent(evt any) {
	t.cli.dispatchEvent(evt)
}
