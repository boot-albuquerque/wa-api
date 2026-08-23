package opengraph

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"golang.org/x/sync/singleflight"
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

const (
	CacheTTL       = 5 * time.Minute
	MaxConcurrency = 5
)

type cacheEntry struct {
	data    domain.LinkPreviewData
	found   bool
	created time.Time
}

// Fetcher implementa appport.LinkPreviewFetcher usando FetchOpenGraphData.
// httpClient DEVE ser SSRF-safe (bootstrap.NewSafeHTTPClient()) — este
// pacote busca páginas e imagens em URLs vindas do corpo de mensagens de
// usuários, então um client sem a validação de IP privado/loopback vira
// uma primitiva de SSRF no primeiro deploy.
//
// Protections ported from the original wuzapi (media_utils.go):
//   - singleflight.Group deduplicates concurrent fetches for the same URL
//   - TTL cache (5 min) avoids re-fetching recently resolved URLs
//   - global semaphore (MaxConcurrency) bounds outbound fetch concurrency
type Fetcher struct {
	HTTPClient *http.Client

	group singleflight.Group
	sem   chan struct{}

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewFetcher cria um Fetcher com o http.Client dado.
func NewFetcher(httpClient *http.Client) *Fetcher {
	return &Fetcher{
		HTTPClient: httpClient,
		sem:        make(chan struct{}, MaxConcurrency),
		cache:      make(map[string]cacheEntry),
	}
}

var _ appport.LinkPreviewFetcher = (*Fetcher)(nil)

// FetchLinkPreview implementa appport.LinkPreviewFetcher: extrai a primeira
// URL de text e, se achar, resolve a metadata de Open Graph dela.
func (f *Fetcher) FetchLinkPreview(ctx context.Context, text string) (domain.LinkPreviewData, bool) {
	url := ExtractFirstURL(text)
	if url == "" {
		return domain.LinkPreviewData{}, false
	}

	if entry, ok := f.cacheGet(url); ok {
		return entry.data, entry.found
	}

	type fetchResult struct {
		data  domain.LinkPreviewData
		found bool
	}

	v, _, _ := f.group.Do(url, func() (interface{}, error) {
		if entry, ok := f.cacheGet(url); ok {
			return fetchResult{data: entry.data, found: entry.found}, nil
		}

		select {
		case f.sem <- struct{}{}:
			defer func() { <-f.sem }()
		case <-ctx.Done():
			return fetchResult{}, nil
		}

		result := FetchOpenGraphData(ctx, f.HTTPClient, url)
		data := domain.LinkPreviewData{
			MatchedURL:    url,
			Title:         result.Title,
			Description:   result.Description,
			ThumbnailJPEG: result.ImageData,
			HQImageData:   result.HQImageData,
			HQWidth:       result.HQWidth,
			HQHeight:      result.HQHeight,
		}

		fr := fetchResult{data: data, found: true}
		f.cachePut(url, fr.data, fr.found)
		return fr, nil
	})

	fr := v.(fetchResult)
	return fr.data, fr.found
}

func (f *Fetcher) cacheGet(url string) (cacheEntry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.cache[url]
	if !ok || time.Since(entry.created) > CacheTTL {
		return cacheEntry{}, false
	}
	return entry, true
}

func (f *Fetcher) cachePut(url string, data domain.LinkPreviewData, found bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache[url] = cacheEntry{data: data, found: found, created: time.Now()}
}
