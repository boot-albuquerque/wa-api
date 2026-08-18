package opengraph

import (
	"context"
	"net/http"
	"regexp"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// urlRegex reconhece a primeira URL http(s) num texto livre. Mesma
// expressão usada pelo helpers.go original do wuzapi (extractFirstURL) e
// pela cópia legada em pkg/infra/media/media_utils.go — mantida aqui,
// dentro do pacote extraído na Fase 12b, como a única fonte viva depois de
// CAP-01.1 (a de media_utils.go não é chamada por ninguém).
var urlRegex = regexp.MustCompile(`https?://[^\s"']*[^\"'\s\.,!?()[\]{}]`)

// ExtractFirstURL devolve a primeira URL http(s) encontrada em text, ou
// string vazia se não houver nenhuma.
func ExtractFirstURL(text string) string {
	return urlRegex.FindString(text)
}

// Fetcher implementa appport.LinkPreviewFetcher usando FetchOpenGraphData.
// httpClient DEVE ser SSRF-safe (bootstrap.NewSafeHTTPClient()) — este
// pacote busca páginas e imagens em URLs vindas do corpo de mensagens de
// usuários, então um client sem a validação de IP privado/loopback vira
// uma primitiva de SSRF no primeiro deploy.
type Fetcher struct {
	HTTPClient *http.Client
}

// NewFetcher cria um Fetcher com o http.Client dado.
func NewFetcher(httpClient *http.Client) *Fetcher {
	return &Fetcher{HTTPClient: httpClient}
}

var _ appport.LinkPreviewFetcher = (*Fetcher)(nil)

// FetchLinkPreview implementa appport.LinkPreviewFetcher: extrai a primeira
// URL de text e, se achar, resolve a metadata de Open Graph dela.
func (f *Fetcher) FetchLinkPreview(ctx context.Context, text string) (domain.LinkPreviewData, bool) {
	url := ExtractFirstURL(text)
	if url == "" {
		return domain.LinkPreviewData{}, false
	}

	result := FetchOpenGraphData(ctx, f.HTTPClient, url)
	return domain.LinkPreviewData{
		MatchedURL:    url,
		Title:         result.Title,
		Description:   result.Description,
		ThumbnailJPEG: result.ImageData,
	}, true
}
