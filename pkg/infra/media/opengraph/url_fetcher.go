package opengraph

import (
	"context"
	"net/http"

	appport "wa-api/pkg/application/contracts"
)

// URLFetcher implementa appport.MediaFetcher em cima de FetchURLBytes — a
// mesma função SSRF-safe e com limite por bytes que FetchOpenGraphData já
// usa para buscar páginas e imagens de link preview (CAP-01.1). CAP-02
// reaproveita em vez de duplicar: buscar bytes de uma URL com limite e
// proteção contra SSRF é a mesma operação nos dois casos, só o chamador
// muda. httpClient DEVE ser SSRF-safe (bootstrap.NewSafeHTTPClient()) —
// mesma exigência de Fetcher.
type URLFetcher struct {
	HTTPClient *http.Client
}

// NewURLFetcher cria um URLFetcher com o http.Client dado.
func NewURLFetcher(httpClient *http.Client) *URLFetcher {
	return &URLFetcher{HTTPClient: httpClient}
}

var _ appport.MediaFetcher = (*URLFetcher)(nil)

// FetchBytes implementa appport.MediaFetcher.
func (f *URLFetcher) FetchBytes(ctx context.Context, resourceURL string, limit int64) ([]byte, string, error) {
	return FetchURLBytes(ctx, f.HTTPClient, resourceURL, limit)
}
