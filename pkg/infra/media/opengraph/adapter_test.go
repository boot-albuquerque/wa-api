package opengraph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ExtractFirstURL
// ---------------------------------------------------------------------------

func TestExtractFirstURL_AchaAPrimeira(t *testing.T) {
	got := ExtractFirstURL("confere https://exemplo.com/pagina e depois http://outro.com")
	if got != "https://exemplo.com/pagina" {
		t.Errorf("got %q, want %q", got, "https://exemplo.com/pagina")
	}
}

func TestExtractFirstURL_SemURL(t *testing.T) {
	if got := ExtractFirstURL("nenhum link aqui"); got != "" {
		t.Errorf("got %q, want string vazia", got)
	}
}

// ---------------------------------------------------------------------------
// Fetcher.FetchLinkPreview
// ---------------------------------------------------------------------------

func TestFetcher_FetchLinkPreview_SemURL(t *testing.T) {
	f := NewFetcher(http.DefaultClient)
	data, found := f.FetchLinkPreview(context.Background(), "sem link nenhum")
	if found {
		t.Fatalf("found=true sem URL no texto: %+v", data)
	}
	if data.MatchedURL != "" || data.Title != "" || data.Description != "" || data.ThumbnailJPEG != nil {
		t.Errorf("data deveria ser zero value: %+v", data)
	}
}

// TestFetcher_FetchLinkPreview_ResolveOpenGraphData prova o caminho inteiro
// do fetcher (extração de URL + FetchOpenGraphData) contra um servidor real
// — não um dublê da resposta HTTP — igual à política deste repo contra
// dublê mais permissivo que a produção.
func TestFetcher_FetchLinkPreview_ResolveOpenGraphData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<meta property="og:title" content="Título Resolvido">
			<meta property="og:description" content="Descrição Resolvida">
		</head><body></body></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())
	text := "olha " + srv.URL + "/pagina isso"

	data, found := f.FetchLinkPreview(context.Background(), text)
	if !found {
		t.Fatal("found=false com URL presente no texto")
	}
	if data.MatchedURL != srv.URL+"/pagina" {
		t.Errorf("MatchedURL: got %q, want %q", data.MatchedURL, srv.URL+"/pagina")
	}
	if data.Title != "Título Resolvido" {
		t.Errorf("Title: got %q, want %q", data.Title, "Título Resolvido")
	}
	if data.Description != "Descrição Resolvida" {
		t.Errorf("Description: got %q, want %q", data.Description, "Descrição Resolvida")
	}
}

// TestFetcher_FetchLinkPreview_FalhaDeRedeNaoQuebraFound: se a URL existe no
// texto mas o fetch falha (servidor fora do ar), found continua true — só a
// metadata vem vazia. A URL casada ainda é um preview válido (MatchedText),
// igual ao contrato herdado do wuzapi original.
func TestFetcher_FetchLinkPreview_FalhaDeRedeNaoQuebraFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())
	text := "olha " + srv.URL + "/quebrado"

	data, found := f.FetchLinkPreview(context.Background(), text)
	if !found {
		t.Fatal("found=false com URL presente, mesmo que o fetch falhe")
	}
	if data.MatchedURL != srv.URL+"/quebrado" {
		t.Errorf("MatchedURL: got %q, want %q", data.MatchedURL, srv.URL+"/quebrado")
	}
	if data.Title != "" || data.Description != "" {
		t.Errorf("metadata deveria vir vazia com fetch falho: %+v", data)
	}
}

// ---------------------------------------------------------------------------
// F113 — singleflight deduplication
// ---------------------------------------------------------------------------

func TestFetcher_Singleflight_DeduplicatesConcurrentFetches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="T"></head></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())
	text := "link " + srv.URL + "/dedup aqui"

	var wg sync.WaitGroup
	const n = 5
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			data, found := f.FetchLinkPreview(context.Background(), text)
			if !found {
				t.Error("found=false")
			}
			if data.Title != "T" {
				t.Errorf("Title = %q", data.Title)
			}
		}()
	}
	wg.Wait()

	got := hits.Load()
	if got != 1 {
		t.Fatalf("singleflight should collapse %d concurrent fetches to 1 server hit, got %d", n, got)
	}
}

// ---------------------------------------------------------------------------
// F113 — TTL cache
// ---------------------------------------------------------------------------

func TestFetcher_Cache_ServesFromCacheOnSecondCall(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="Cached"></head></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())
	text := "link " + srv.URL + "/cached aqui"

	d1, _ := f.FetchLinkPreview(context.Background(), text)
	d2, _ := f.FetchLinkPreview(context.Background(), text)

	if hits.Load() != 1 {
		t.Fatalf("second call should hit cache, but server was called %d times", hits.Load())
	}
	if d1.Title != "Cached" || d2.Title != "Cached" {
		t.Fatalf("Title mismatch: d1=%q d2=%q", d1.Title, d2.Title)
	}
}

// ---------------------------------------------------------------------------
// F113 — concurrency semaphore
// ---------------------------------------------------------------------------

func TestFetcher_Semaphore_BoundsConcurrency(t *testing.T) {
	var concurrent atomic.Int32
	var maxSeen atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := concurrent.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		concurrent.Add(-1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="OK"></head></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())

	var wg sync.WaitGroup
	const n = 15
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			text := srv.URL + "/" + http.CanonicalHeaderKey("path"+string(rune('A'+i))) + " link"
			f.FetchLinkPreview(context.Background(), text)
		}()
	}
	wg.Wait()

	got := maxSeen.Load()
	if got > int32(MaxConcurrency) {
		t.Fatalf("max concurrent fetches = %d, limit is %d", got, MaxConcurrency)
	}
}

// ---------------------------------------------------------------------------
// F113 — HQ data passthrough
// ---------------------------------------------------------------------------

func TestFetcher_FetchLinkPreview_PropagatesHQImageData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/img.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes(t, 800, 600))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<meta property="og:title" content="HQ">
			<meta property="og:image" content="/img.png">
		</head></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client())
	data, found := f.FetchLinkPreview(context.Background(), "link "+srv.URL+"/page aqui")
	if !found {
		t.Fatal("found=false")
	}
	if len(data.HQImageData) == 0 {
		t.Fatal("HQImageData should be populated")
	}
	if data.HQWidth == 0 || data.HQHeight == 0 {
		t.Fatalf("HQ dimensions should be set: %dx%d", data.HQWidth, data.HQHeight)
	}
}
