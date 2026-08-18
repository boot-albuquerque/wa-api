package opengraph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
