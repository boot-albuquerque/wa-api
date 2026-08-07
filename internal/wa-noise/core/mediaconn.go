package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/media"
)

// A implementacao vive em internal/wa-noise/media/conn.go; aqui ficam so' os
// apelidos de tipo e os metodos finos de *Client (ADR-0004, Fase F/G lote 1).

// MediaConnHost represents a single host to download media from.
type MediaConnHost = media.ConnHost

// MediaConn contains a list of WhatsApp servers from which attachments can be downloaded from.
type MediaConn = media.Conn

func (cli *Client) refreshMediaConn(ctx context.Context, force bool) (*MediaConn, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.RefreshConn(ctx, cli.mediaT(), force)
}

func (cli *Client) queryMediaConn(ctx context.Context) (*MediaConn, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.QueryConn(ctx, cli.mediaT())
}
