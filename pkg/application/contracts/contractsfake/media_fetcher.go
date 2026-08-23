package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// DefaultFetchedImageBytes é o corpo que MediaFetcher devolve sem
// FetchBytesFunc configurada — bytes não vazios e não decodificáveis como
// imagem de propósito, para que um teste que esqueça de configurar o fetch
// mas dependa do conteúdo falhe de forma óbvia em vez de silenciosa.
var DefaultFetchedImageBytes = []byte("contractsfake-default-media-bytes")

// MediaFetcherCall é uma chamada a FetchBytes.
type MediaFetcherCall struct {
	Ctx         context.Context
	ResourceURL string
	Limit       int64
}

// MediaFetcher é o fake de port.MediaFetcher.
type MediaFetcher struct {
	FetchBytesFunc  func(ctx context.Context, resourceURL string, limit int64) ([]byte, string, error)
	FetchBytesCalls []MediaFetcherCall
}

var _ port.MediaFetcher = (*MediaFetcher)(nil)

// FetchBytes implementa port.MediaFetcher.
func (f *MediaFetcher) FetchBytes(ctx context.Context, resourceURL string, limit int64) ([]byte, string, error) {
	f.FetchBytesCalls = append(f.FetchBytesCalls, MediaFetcherCall{Ctx: ctx, ResourceURL: resourceURL, Limit: limit})
	if f.FetchBytesFunc != nil {
		return f.FetchBytesFunc(ctx, resourceURL, limit)
	}
	return DefaultFetchedImageBytes, "application/octet-stream", nil
}
